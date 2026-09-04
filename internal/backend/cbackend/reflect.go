// Рефлексия ("тип <Имя>", "(Значение) x", "значение как <Тип>", методы
// "Значение"/"Тип") - см. docs/SPEC.md и internal/semantic/reflect.go
// (package doc). Без встроенной RTTI в целевом C компилятор сам строит
// таблицы метаданных: "rech_any_t" - боксированное значение (тег типа +
// union), "rech_type_meta_t" - дескриптор типа (имя + таблицы полей/
// методов + указатель на конструктор-обёртку). Каждый примитив получает
// статический дескриптор безусловно (когда UsesReflection); каждая
// сущность/структура из prog.ReflectTargets - рекурсивно, тем же
// "ensure"-принципом, что jsonExtractorGen (json_extract.go): дескриптор
// генерируется один раз, до него - дескрипторы всего, что достижимо через
// поля/типы параметров-методов/тип результата (иначе тип_поля/
// тип_параметра_метода/тип_результата_метода указывали бы на
// несуществующий дескриптор).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/codegen"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// reflectTypedefs - C-типы рантайма рефлексии, эмитятся один раз, до
// genericsCode (Список<Значение> ссылается на rech_any_t как на C-тип
// элемента - см. package doc generateSource, тот же класс проблемы, что и
// у typedefs/genericsCode). rech_pusto_singleton - "Пусто" как реальный
// Тип (не NULL) у тип_результата_метода метода без результата - см.
// reflectPustoName.
const reflectTypedefs = `typedef struct rech_type_meta rech_type_meta_t;

typedef struct {
    const rech_type_meta_t *type;
    union {
        long long i;
        double f64;
        float f32;
        bool b;
        const char *s;
        void *ptr;
    } v;
} rech_any_t;

typedef rech_any_t (*rech_field_getter_t)(void *self);
typedef void (*rech_field_setter_t)(void *self, rech_any_t v);
typedef struct {
    const char *name;
    const rech_type_meta_t *type;
    rech_field_getter_t get;
    rech_field_setter_t set;
    const char *tag;
} rech_field_meta_t;

typedef rech_any_t (*rech_method_invoker_t)(void *self, rech_any_t *args, int argc);
typedef struct {
    const char *name;
    int param_count;
    const rech_type_meta_t *const *param_types;
    const rech_type_meta_t *return_type;
    rech_method_invoker_t invoke;
    const char *tag;
} rech_method_meta_t;

typedef rech_any_t (*rech_ctor_invoker_t)(rech_any_t *args, int argc);

struct rech_type_meta {
    const char *name;
    const rech_field_meta_t *fields;
    int field_count;
    const rech_method_meta_t *methods;
    int method_count;
    rech_ctor_invoker_t ctor;
    int ctor_param_count;
    const rech_type_meta_t *base;
    const rech_type_meta_t *const *interfaces;
    int interface_count;
    const char *tag;
};

`

// reflectUnboxCheckHelper - "значение как Тип" и распаковка каждого
// аргумента "вызвать"/"установить_поле"/"создать" проходят через эту
// проверку - несовпадение фактического типа боксированного значения с
// ожидаемым - паника (сравнение указателей на статический дескриптор, не
// строк - каждый тип получает РОВНО один дескриптор на всю программу).
var reflectUnboxCheckHelper = fmt.Sprintf(`static rech_any_t rech_unbox_check(rech_any_t v, const rech_type_meta_t *expected) {
    if (v.type != expected) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%s", expected->name, v.type->name);
        rech_panic(msg);
    }
    return v;
}
`, escapeCString(rtmsg.AsTypeMismatchTemplate))

// reflectUnboxCheckIfaceHelper - "значение как ИмяИнтерфейса"
// (docs/ROADMAP.md - находка "значение как ИмяИнтерфейса не распаковывает
// боксированную реализацию") - в отличие от rech_unbox_check выше (точное
// совпадение с ОДНИМ ожидаемым дескриптором), проверяет членство v.type
// среди impls (rech_iface_impls_<Интерфейс>, см. interfaceImplementorsArrayCode) -
// боксированное значение несёт дескриптор КОНКРЕТНОЙ сущности, не самого
// интерфейса, поэтому единственный корректный вопрос - "реализует ли ЭТА
// сущность требуемый интерфейс", не "совпадает ли с ним дословно".
var reflectUnboxCheckIfaceHelper = fmt.Sprintf(`static rech_any_t rech_unbox_check_iface(rech_any_t v, const rech_type_meta_t *const *impls, int impl_count, const char *iface_name) {
    for (int i = 0; i < impl_count; i++) {
        if (v.type == impls[i]) {
            return v;
        }
    }
    char msg[256];
    snprintf(msg, sizeof(msg), "%s", iface_name, v.type->name);
    rech_panic(msg);
    return v;
}
`, escapeCString(rtmsg.AsInterfaceMismatchTemplate))

// reflectLookupHelpers - поиск поля/метода по имени (линейный - число
// полей/методов сущности единицы, не нужен хеш) и по индексу (с проверкой
// границ) - общие для всех типов, используются кодогенерацией методов
// "Тип"/"Значение" (internal/backend/cbackend/ir_generator.go).
var reflectLookupHelpers = fmt.Sprintf(`static const rech_field_meta_t *rech_field_by_name(const rech_type_meta_t *t, const char *name) {
    for (int i = 0; i < t->field_count; i++) {
        if (strcmp(t->fields[i].name, name) == 0) {
            return &t->fields[i];
        }
    }
    return NULL;
}

static const rech_method_meta_t *rech_method_by_name(const rech_type_meta_t *t, const char *name) {
    for (int i = 0; i < t->method_count; i++) {
        if (strcmp(t->methods[i].name, name) == 0) {
            return &t->methods[i];
        }
    }
    return NULL;
}

static const rech_field_meta_t *rech_field_at(const rech_type_meta_t *t, long long i) {
    if (i < 0 || i >= (long long)t->field_count) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%s", "поле", t->name);
        rech_panic(msg);
    }
    return &t->fields[i];
}

static const rech_method_meta_t *rech_method_at(const rech_type_meta_t *t, long long i) {
    if (i < 0 || i >= (long long)t->method_count) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%s", "метод", t->name);
        rech_panic(msg);
    }
    return &t->methods[i];
}

static const rech_type_meta_t *rech_param_at(const rech_method_meta_t *m, long long i) {
    if (i < 0 || i >= (long long)m->param_count) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%s", "параметр", m->name);
        rech_panic(msg);
    }
    return m->param_types[i];
}

static const rech_type_meta_t *rech_iface_at(const rech_type_meta_t *t, long long i) {
    if (i < 0 || i >= (long long)t->interface_count) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%s", "интерфейс", t->name);
        rech_panic(msg);
    }
    return t->interfaces[i];
}

static bool rech_type_is_subclass(const rech_type_meta_t *base, const rech_type_meta_t *target) {
    for (; base != NULL; base = base->base) {
        if (base == target) {
            return true;
        }
    }
    return false;
}

static bool rech_type_implements(const rech_type_meta_t *t, const rech_type_meta_t *iface) {
    for (int i = 0; i < t->interface_count; i++) {
        if (t->interfaces[i] == iface) {
            return true;
        }
    }
    return false;
}

static const rech_type_meta_t *rech_type_base_or_pusto(const rech_type_meta_t *t) {
    return t->base ? t->base : &%s;
}

static rech_any_t rech_reflect_get_field(rech_any_t self, const char *name) {
    const rech_field_meta_t *f = rech_field_by_name(self.type, name);
    if (!f) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%s", name, self.type->name);
        rech_panic(msg);
    }
    return f->get(self.v.ptr);
}

static void rech_reflect_set_field(rech_any_t self, const char *name, rech_any_t v) {
    const rech_field_meta_t *f = rech_field_by_name(self.type, name);
    if (!f) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%s", name, self.type->name);
        rech_panic(msg);
    }
    f->set(self.v.ptr, v);
}
`, escapeCString(rtmsg.ReflectIndexOutOfRangeTemplate), escapeCString(rtmsg.ReflectIndexOutOfRangeTemplate),
	escapeCString(rtmsg.ReflectIndexOutOfRangeTemplate), escapeCString(rtmsg.ReflectIndexOutOfRangeTemplate),
	reflectMetaName(reflectPustoName),
	escapeCString(rtmsg.ReflectFieldNotFoundTemplate), escapeCString(rtmsg.ReflectFieldNotFoundTemplate))

// reflectTagLookupHelper - "тег_поля(индекс, ключ)": rech_field_meta_t.tag
// несёт сырой текст тега поля ("json:name, sql:user_name", "" - тега нет) -
// rech_tag_lookup разбирает его на лету тем же способом, что и
// internal/structtag.Parse (пары "ключ:значение" через запятую, пробелы
// вокруг ключа/значения/запятой не значимы) и возвращает копию найденного
// значения (GC_MALLOC - тот же принцип управления памятью, что и весь
// остальной рантайм cbackend) либо "" - тега нет или искомого ключа в нём
// нет.
const reflectTagLookupHelper = `static const char *rech_tag_lookup(const char *tag, const char *key) {
    if (!tag) return "";
    size_t keylen = strlen(key);
    const char *p = tag;
    while (*p) {
        while (*p == ' ' || *p == ',') p++;
        if (!*p) break;
        const char *kstart = p;
        while (*p && *p != ':' && *p != ',') p++;
        const char *kend = p;
        while (kend > kstart && kend[-1] == ' ') kend--;
        if (*p != ':') {
            if (*p == ',') p++;
            continue;
        }
        p++;
        while (*p == ' ') p++;
        const char *vstart = p;
        while (*p && *p != ',') p++;
        const char *vend = p;
        while (vend > vstart && vend[-1] == ' ') vend--;
        if ((size_t)(kend - kstart) == keylen && strncmp(kstart, key, keylen) == 0) {
            size_t vlen = (size_t)(vend - vstart);
            char *out = GC_MALLOC(vlen + 1);
            memcpy(out, vstart, vlen);
            out[vlen] = '\0';
            return out;
        }
        if (*p == ',') p++;
    }
    return "";
}
`

// reflectAnyListType/reflectAnyListCType - "Список<Значение>", тип
// аргументов "вызвать"/"создать" - та же строка, которую строит
// internal/semantic.resolveGenericTypeName для реального "новый
// Список<Значение>()" в коде пользователя (см. internal/semantic/
// reflect.go, listOfAnyType) - вызвать/создать типизированы так, что
// компилируются только если пользователь такой список уже где-то
// сконструировал, поэтому reflectDispatchHelpers эмитится (см.
// hasReflectAnyList в ir_generator.go) РОВНО тогда, когда этот C-typedef
// уже существует (prog.Generics).
const reflectAnyListType = "Список<Значение>"

func reflectAnyListCType() string { return mangleType(reflectAnyListType) }

// reflectDispatchHelpersCode - "значение.вызвать(имя, аргументы)"/
// "тип.создать(аргументы)" - находят метод/конструктор по имени/через
// указатель дескриптора, проверяют число аргументов (паника при
// несовпадении - ту же проверку на этапе компиляции сделать нельзя, имя
// метода известно только в рантайме), вызывают через уже найденный
// указатель (invoke/ctor).
func reflectDispatchHelpersCode() string {
	return fmt.Sprintf(`static rech_any_t rech_reflect_invoke(rech_any_t self, const char *name, %[1]s *args) {
    const rech_method_meta_t *m = rech_method_by_name(self.type, name);
    if (!m) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[2]s", name, self.type->name);
        rech_panic(msg);
    }
    if (m->param_count != (int)args->len) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[3]s", name, m->param_count, (int)args->len);
        rech_panic(msg);
    }
    return m->invoke(self.v.ptr, args->data, (int)args->len);
}

static rech_any_t rech_reflect_create(const rech_type_meta_t *t, %[1]s *args) {
    if (!t->ctor) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[2]s", "создать", t->name);
        rech_panic(msg);
    }
    if (t->ctor_param_count != (int)args->len) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[3]s", "создать", t->ctor_param_count, (int)args->len);
        rech_panic(msg);
    }
    return t->ctor(args->data, (int)args->len);
}
`, reflectAnyListCType(), escapeCString(rtmsg.ReflectMethodNotFoundTemplate), escapeCString(rtmsg.ReflectArgCountMismatchTemplate))
}

// reflectPustoName - имя синглтон-дескриптора "Пусто" (тип результата
// метода без объявленного результата, см. typeInfoMethodSigs.
// "тип_результата_метода" в internal/semantic/reflect.go) - настоящий
// дескриптор (нулевые поля/методы), не NULL, чтобы .имя()/остальные
// методы "Тип" работали на нём без отдельной проверки на NULL.
const reflectPustoName = "Пусто"

// reflectPrimitiveKinds - примитивы, получающие статический дескриптор
// безусловно, когда prog.UsesReflection - Символ/Байт входят (у обоих
// естественный числовой код, боксируются как .v.i, тем же принципом, что
// и у Целое). Файл/Сокет/БД (непрозрачные дескрипторы) - боксируются как
// указатель в ".v.i" тем же общим "default:"-путём reflectBoxExprCode/
// reflectUnboxExprCode, что и остальные примитивы здесь - НАЙДЕННЫЙ БАГ
// (не по постановке задачи): docs/SPEC.md, раздел 19, обещает боксирование
// "значения ЛЮБОГО типа" безусловно, semantic и internal/interp этому
// следуют (ни один не отвергает "(Значение) x" для Файл/Сокет/БД), но
// cbackend раньше не включал их сюда - "(Значение) бд" компилировался в
// ссылку на несуществующий "rech_type_meta_bd" (проявилось при попытке
// хранить "БД" в DI-контейнере через рефлексию, examples/проекты/
// http_сервер/ди/контейнер.rech). Вызываемый/Задача сюда сознательно НЕ
// входят - у обоих не "T *"/скаляр, а собственное представление (пара
// указателей у Callable, отдельная структура-обёртка у Задачи), которое
// default-путь reflectBoxExprCode/reflectUnboxExprCode испортил бы молча
// (не тот же класс бага - решать отдельно, если/когда понадобится).
// KindNull ("Ничто" как тип самого по себе, не "?Тип" - см. package doc
// ir.KindNull) входит в этот список наравне с обычными примитивами - им
// боксируется SQL NULL в результате "запросить" (internal/backend/
// cbackend/db.go), проверяется как "результат.получить(колонка).
// тип_значения().имя() == \"Ничто\"".
var reflectPrimitiveKinds = []ir.TypeKind{
	ir.KindInt, ir.KindInt8, ir.KindInt16, ir.KindInt32, ir.KindInt64,
	ir.KindUint, ir.KindUint8, ir.KindUint16, ir.KindUint32, ir.KindUint64,
	ir.KindFloat32, ir.KindFloat, ir.KindString, ir.KindBool, ir.KindChar, ir.KindByte,
	ir.KindNull, ir.KindFile, ir.KindSocket, ir.KindDB,
}

// reflectMetaName - имя C-переменной статического дескриптора для
// Rech-имени типа name (примитив, сущность или reflectPustoName).
func reflectMetaName(name string) string {
	return "rech_type_meta_" + transliterate(name)
}

// reflectMetaVarFor - то же самое, что reflectMetaName, но по ir.Type -
// для сущности/структуры имя дескриптора берётся из t.Entity, для
// примитива - из t.String() (то же имя, что видит пользователь).
func reflectMetaVarFor(t ir.Type) string {
	if t.Kind == ir.KindEntity {
		return reflectMetaName(t.Entity)
	}
	return reflectMetaName(t.String())
}

// primitiveReflectDescriptors - статические дескрипторы примитивов
// (включая KindNull, "Ничто") и синглтона "Пусто" - нулевые поля/методы/
// конструктор (примитив нельзя рефлексивно "создать" - reflectCreateCall
// это проверяет в рантайме).
func primitiveReflectDescriptors() string {
	var sb strings.Builder
	for _, k := range reflectPrimitiveKinds {
		name := ir.Type{Kind: k}.String()
		fmt.Fprintf(&sb, "static const rech_type_meta_t %s = {\"%s\", NULL, 0, NULL, 0, NULL, 0, NULL, NULL, 0, \"\"};\n", reflectMetaName(name), escapeCString(name))
	}
	fmt.Fprintf(&sb, "static const rech_type_meta_t %s = {\"%s\", NULL, 0, NULL, 0, NULL, 0, NULL, NULL, 0, \"\"};\n", reflectMetaName(reflectPustoName), escapeCString(reflectPustoName))
	sb.WriteString("\n")
	return sb.String()
}

// nullableReflectDescriptors - статические дескрипторы "?Тип" (bare, как у
// primitiveReflectDescriptors выше - нулевые поля/методы/конструктор) для
// КАЖДОГО "?Тип", реально зарегистрированного в prog.Nullables. НАЙДЕННЫЙ
// БАГ (не по постановке задачи, тот же класс, что и у "БД" в
// reflectPrimitiveKinds выше): reflectMetaVarFor для метода с nullable-
// возвратом ("статус(id: Строка): ?Задача") уже строит ИМЯ переменной
// дескриптора корректно (t.String() у KindNullable == t.Entity == "?Задача",
// см. internal/ir/type.go), но САМУ переменную никто не эмитировал -
// метод-таблица рефлексии сущности (entityMethodTableCCode) ссылалась на
// несуществующий "rech_type_meta_n_zadacha", падая "undeclared identifier"
// при боксировании ЛЮБОЙ сущности с хотя бы одним таким методом (найдено
// на "ХранилищеЗадач.статус" при переводе DI-контейнера http_сервера на
// рефлексивное хранилище, examples/проекты/http_сервер/исходники/
// контейнер.rech). prog.Nullables - уже готовый список ВСЕХ "?Тип",
// использованных где-либо в программе (см. package doc internal/semantic,
// registerNullableTypeName) - тот же источник, из которого genericsCode
// строит сами C-обёртки "?Тип", просто эмитируется отдельно и раньше (bare-
// дескриптор не ссылается на typedef обёртки, в отличие от entity/generic-
// коллекции descriptors ниже).
func nullableReflectDescriptors(prog *ir.Program) string {
	var sb strings.Builder
	for _, nb := range prog.Nullables {
		fmt.Fprintf(&sb, "static const rech_type_meta_t %s = {\"%s\", NULL, 0, NULL, 0, NULL, 0, NULL, NULL, 0, \"\"};\n", reflectMetaName(nb.Name), escapeCString(nb.Name))
	}
	if len(prog.Nullables) > 0 {
		sb.WriteString("\n")
	}
	return sb.String()
}

// reflectBoxExprCode строит C-выражение типа rech_any_t, боксирующее
// значение типа t, хранящееся в cExpr. Целое8/16/32/64, Натуральное/
// 8/16/32/64, Целое, Символ, Байт - битовый паттерн в .v.i как есть (тег
// типа несёт знаковость/ширину для корректной распаковки обратно, тот же
// принцип, что и у internal/interp.wrapFixedWidth), не нуждаются в
// отдельной ветке каждый.
func reflectBoxExprCode(ctx *irGenCtx, t ir.Type, cExpr string) string {
	meta := reflectMetaVarFor(t)
	switch t.Kind {
	case ir.KindFloat32:
		return fmt.Sprintf("(rech_any_t){.type = &%s, .v.f32 = (float)(%s)}", meta, cExpr)
	case ir.KindFloat:
		return fmt.Sprintf("(rech_any_t){.type = &%s, .v.f64 = (double)(%s)}", meta, cExpr)
	case ir.KindBool:
		return fmt.Sprintf("(rech_any_t){.type = &%s, .v.b = (bool)(%s)}", meta, cExpr)
	case ir.KindString:
		return fmt.Sprintf("(rech_any_t){.type = &%s, .v.s = (const char *)(%s)}", meta, cExpr)
	case ir.KindEntity:
		if ctx.entityIsRef(t.Entity) {
			return fmt.Sprintf("(rech_any_t){.type = &%s, .v.ptr = (void *)(%s)}", meta, cExpr)
		}
		return fmt.Sprintf("%s(%s)", reflectBoxStructFuncName(t.Entity), cExpr)
	case ir.KindNullable:
		// НАЙДЕННЫЙ БАГ: раньше падал в default ниже - "?Тип" (например,
		// "?Задача" у "ХранилищеЗадач.статус") ЭТО СТРУКТУРА (has/value,
		// nullable.go), не скаляр, приводить её к "(long long)" - ошибка
		// компиляции самого сгенерированного C ("aggregate value used
		// where an integer was expected"), найдено при боксировании
		// ЛЮБОЙ сущности с хотя бы одним методом/полем nullable-типа
		// (examples/проекты/http_сервер, DI-контейнер на рефлексии). cExpr
		// может быть вызовом с побочным эффектом (сам метод) - вычислить
		// РОВНО один раз обязана функция-хелпер (см.
		// reflectNullableBoxHelperName), не инлайн-тернарник здесь же.
		return fmt.Sprintf("%s(%s)", reflectNullableBoxHelperName(t.Entity), cExpr)
	default:
		return fmt.Sprintf("(rech_any_t){.type = &%s, .v.i = (long long)(%s)}", meta, cExpr)
	}
}

// reflectNullableBoxHelperName - имя C-функции-хелпера (см.
// nullableReflectBoxHelpers), боксирующей ОДНО конкретное "?Тип" (name -
// каноническое имя с "?", тот же принцип, что у reflectMetaName).
func reflectNullableBoxHelperName(name string) string {
	return "rech_reflect_box_" + transliterate(name)
}

// nullableReflectBoxHelpers - одна функция-хелпер на каждый "?Тип" из
// prog.Nullables, боксирующая его значение в rech_any_t (см. её package doc
// в reflectBoxExprCode выше про сам баг): "Ничто" (v.has == 0) - хелпер
// значения "Ничто" (тот же принцип, что и у SQL NULL в internal/backend/
// cbackend/db.go - "результат.получить(колонка).тип_значения().имя() ==
// \"Ничто\""), иначе - боксирование ВНУТРЕННЕГО типа рекурсивно через
// reflectBoxExprCode (сама вложенность конечна - "?Тип" не может обернуть
// сам себя, "??Тип" запрещено на этапе семантики, docs/SPEC.md).
//
// Круговая зависимость с reflectRuntimeCode ниже разрешена в два прохода:
// **прототипы** (nullableReflectBoxHelperProtos) эмитятся ДО
// reflectRuntimeCode (методы сущностей, чьи method-таблицы зовут эти
// хелперы по имени - обязаны видеть хотя бы прототип раньше места вызова),
// а **сами тела** (эта функция) - ПОСЛЕ (тело хелпера для "?Задача" само
// ссылается на дескриптор сущности "Задача" - &rech_type_meta_zadacha,
// который получает опережающее объявление только внутри reflectRuntimeCode,
// см. reflectGen.ensure). Прототипу для этого не нужно ничего, кроме уже
// видимого typedef самой обёртки "?Тип" (nullablesCode/
// nullablesAfterTypedefs, ir_generator.go); телу - ещё и
// primitiveReflectDescriptors/nullableReflectDescriptors (дескриптор
// "Ничто") и opережающее объявление сущности-содержимого из
// reflectRuntimeCode.
func nullableReflectBoxHelperProtos(prog *ir.Program) string {
	var sb strings.Builder
	for _, nb := range prog.Nullables {
		fmt.Fprintf(&sb, "static rech_any_t %s(%s v);\n", reflectNullableBoxHelperName(nb.Name), mangleType(nb.Name))
	}
	if len(prog.Nullables) > 0 {
		sb.WriteString("\n")
	}
	return sb.String()
}

func nullableReflectBoxHelpers(ctx *irGenCtx, prog *ir.Program) string {
	var sb strings.Builder
	nichto := reflectMetaName(ir.TypeNullValue.String())
	for _, nb := range prog.Nullables {
		cType := mangleType(nb.Name)
		fmt.Fprintf(&sb, "static rech_any_t %s(%s v) {\n", reflectNullableBoxHelperName(nb.Name), cType)
		fmt.Fprintf(&sb, "    if (!v.has) {\n        return (rech_any_t){.type = &%s};\n    }\n", nichto)
		fmt.Fprintf(&sb, "    return %s;\n}\n\n", reflectBoxExprCode(ctx, nb.Inner, "v.value"))
	}
	return sb.String()
}

// reflectUnboxExprCode - обратное к reflectBoxExprCode: читает значение
// типа t из уже ПРОВЕРЕННОГО anyExpr (rech_any_t, тип уже подтверждён
// вызывающей стороной - см. reflectCheckedUnboxExprCode). Структура -
// копия ИЗ памяти, на которую указывает .v.ptr (та память - GC-копия,
// сделанная при боксировании, см. reflectBoxStructFuncName) - разыменование
// даёт независимую копию, тем же принципом, что и обычное присваивание
// структуры в Rech.
func reflectUnboxExprCode(ctx *irGenCtx, t ir.Type, anyExpr string) string {
	switch t.Kind {
	case ir.KindFloat32:
		return fmt.Sprintf("((float)(%s).v.f32)", anyExpr)
	case ir.KindFloat:
		return fmt.Sprintf("((double)(%s).v.f64)", anyExpr)
	case ir.KindBool:
		return fmt.Sprintf("((bool)(%s).v.b)", anyExpr)
	case ir.KindString:
		return fmt.Sprintf("((const char *)(%s).v.s)", anyExpr)
	case ir.KindEntity:
		cType := mangleType(t.Entity)
		if ctx.entityIsRef(t.Entity) {
			return fmt.Sprintf("((%s *)(%s).v.ptr)", cType, anyExpr)
		}
		return fmt.Sprintf("(*(%s *)(%s).v.ptr)", cType, anyExpr)
	case ir.KindInterface:
		// Представление интерфейсного значения - всегда "void *" (см.
		// package doc ctx.receiverIsPointer, vtable.go) - боксированный
		// указатель (v.v.ptr) уже указывает на КОНКРЕТНУЮ сущность,
		// реализующую интерфейс (rech_unbox_check_iface выше уже это
		// проверил - см. reflectCheckedUnboxExprCode), она сама несёт
		// скрытый заголовок vtable первым полем - тот же указатель без
		// изменений УЖЕ пригоден для обычного вызова метода через
		// интерфейс, никакого дополнительного оборачивания не нужно.
		return fmt.Sprintf("((void *)(%s).v.ptr)", anyExpr)
	default:
		return fmt.Sprintf("((%s)(%s).v.i)", fromIRType(t).cType(ctx), anyExpr)
	}
}

// reflectCheckedUnboxExprCode - reflectUnboxExprCode с рантайм-проверкой
// фактического типа anyExpr перед распаковкой (rech_unbox_check, паника
// при несовпадении) - используется абсолютно везде, где Значение
// распаковывается в конкретный тип ("как", аргументы "вызвать"/
// "установить_поле"/"создать") - ни одна из этих операций не должна
// молча читать чужой union-вариант.
func reflectCheckedUnboxExprCode(ctx *irGenCtx, t ir.Type, anyExpr string) string {
	if t.Kind == ir.KindInterface {
		implCount := len(ctx.interfaceImplementors[t.Entity])
		checked := fmt.Sprintf("rech_unbox_check_iface(%s, %s, %d, \"%s\")",
			anyExpr, interfaceImplementorsArrayName(t.Entity), implCount, escapeCString(t.Entity))
		return reflectUnboxExprCode(ctx, t, checked)
	}
	checked := fmt.Sprintf("rech_unbox_check(%s, &%s)", anyExpr, reflectMetaVarFor(t))
	return reflectUnboxExprCode(ctx, t, checked)
}

// generateIRExprList генерирует C-код каждого выражения exprs по порядку -
// общий маленький хелпер для generateIRAnyMethodCall/
// generateIRTypeInfoMethodCall (аргументы методов "Значение"/"Тип"), у
// которых, в отличие от generateIRMethodCallExpr для обычных методов
// сущности, нет единого "this_"-получателя, требующего отдельной ветки.
func generateIRExprList(ctx *irGenCtx, exprs []ir.Expr) ([]string, error) {
	codes := make([]string, len(exprs))
	for i, e := range exprs {
		code, _, err := generateIRExpr(ctx, e)
		if err != nil {
			return nil, err
		}
		codes[i] = code
	}
	return codes, nil
}

// generateIRAsExpr - "<значение> как <Тип>" (ir.AsExpr) - разбоксирование
// с рантайм-проверкой (reflectCheckedUnboxExprCode).
func generateIRAsExpr(ctx *irGenCtx, e *ir.AsExpr) (string, valueType, error) {
	code, _, err := generateIRExpr(ctx, e.Value)
	if err != nil {
		return "", typeUnknown, err
	}
	ctx.usesPanic = true
	return reflectCheckedUnboxExprCode(ctx, e.To, code), fromIRType(e.To), nil
}

// generateIRAnyMethodCall/generateIRTypeInfoMethodCall - методы
// "Значение"/"Тип" (docs/SPEC.md) - вызываются из generateIRMethodCallExpr
// ДО общей ветки сущности/интерфейса (Значение/Тип - не KindEntity, см.
// package doc internal/ir/type.go, KindAny/KindTypeInfo), не переиспользуют
// mangleMethod/receiverIsPointer - у обоих собственное, не по имени
// сущности, представление.
func generateIRAnyMethodCall(ctx *irGenCtx, e *ir.MethodCallExpr) (string, valueType, error) {
	recvCode, _, err := generateIRExpr(ctx, e.Receiver)
	if err != nil {
		return "", typeUnknown, err
	}
	argCodes, err := generateIRExprList(ctx, e.Args)
	if err != nil {
		return "", typeUnknown, err
	}
	ctx.usesPanic = true
	switch e.Method {
	case "тип_значения":
		return fmt.Sprintf("(%s).type", recvCode), typeTypeInfo, nil
	case "получить_поле":
		return fmt.Sprintf("rech_reflect_get_field(%s, %s)", recvCode, argCodes[0]), typeAny, nil
	case "установить_поле":
		return fmt.Sprintf("rech_reflect_set_field(%s, %s, %s)", recvCode, argCodes[0], argCodes[1]), typeUnknown, nil
	case "вызвать":
		return fmt.Sprintf("rech_reflect_invoke(%s, %s, %s)", recvCode, argCodes[0], argCodes[1]), typeAny, nil
	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: у \"Значение\" нет метода %q", e.Method)
	}
}

func generateIRTypeInfoMethodCall(ctx *irGenCtx, e *ir.MethodCallExpr) (string, valueType, error) {
	recvCode, _, err := generateIRExpr(ctx, e.Receiver)
	if err != nil {
		return "", typeUnknown, err
	}
	argCodes, err := generateIRExprList(ctx, e.Args)
	if err != nil {
		return "", typeUnknown, err
	}
	ctx.usesPanic = true
	switch e.Method {
	case "имя":
		return fmt.Sprintf("(%s)->name", recvCode), typeString, nil
	case "число_полей":
		return fmt.Sprintf("(%s)->field_count", recvCode), typeInt, nil
	case "имя_поля":
		return fmt.Sprintf("rech_field_at(%s, %s)->name", recvCode, argCodes[0]), typeString, nil
	case "тип_поля":
		return fmt.Sprintf("rech_field_at(%s, %s)->type", recvCode, argCodes[0]), typeTypeInfo, nil
	case "тег_поля":
		return fmt.Sprintf("rech_tag_lookup(rech_field_at(%s, %s)->tag, %s)", recvCode, argCodes[0], argCodes[1]), typeString, nil
	case "тег_поля_текст":
		return fmt.Sprintf("rech_field_at(%s, %s)->tag", recvCode, argCodes[0]), typeString, nil
	case "число_методов":
		return fmt.Sprintf("(%s)->method_count", recvCode), typeInt, nil
	case "имя_метода":
		return fmt.Sprintf("rech_method_at(%s, %s)->name", recvCode, argCodes[0]), typeString, nil
	case "число_параметров_метода":
		return fmt.Sprintf("rech_method_at(%s, %s)->param_count", recvCode, argCodes[0]), typeInt, nil
	case "тип_параметра_метода":
		return fmt.Sprintf("rech_param_at(rech_method_at(%s, %s), %s)", recvCode, argCodes[0], argCodes[1]), typeTypeInfo, nil
	case "тип_результата_метода":
		return fmt.Sprintf("rech_method_at(%s, %s)->return_type", recvCode, argCodes[0]), typeTypeInfo, nil
	case "тег_метода":
		return fmt.Sprintf("rech_tag_lookup(rech_method_at(%s, %s)->tag, %s)", recvCode, argCodes[0], argCodes[1]), typeString, nil
	case "тег_метода_текст":
		return fmt.Sprintf("rech_method_at(%s, %s)->tag", recvCode, argCodes[0]), typeString, nil
	case "создать":
		return fmt.Sprintf("rech_reflect_create(%s, %s)", recvCode, argCodes[0]), typeAny, nil
	case "тег":
		return fmt.Sprintf("rech_tag_lookup((%s)->tag, %s)", recvCode, argCodes[0]), typeString, nil
	case "тег_текст":
		return fmt.Sprintf("(%s)->tag", recvCode), typeString, nil
	case "базовый_тип":
		return fmt.Sprintf("rech_type_base_or_pusto(%s)", recvCode), typeTypeInfo, nil
	case "наследует_ли":
		return fmt.Sprintf("rech_type_is_subclass((%s)->base, %s)", recvCode, argCodes[0]), typeBool, nil
	case "число_интерфейсов":
		return fmt.Sprintf("(%s)->interface_count", recvCode), typeInt, nil
	case "получить_интерфейс":
		return fmt.Sprintf("rech_iface_at(%s, %s)", recvCode, argCodes[0]), typeTypeInfo, nil
	case "реализует_ли":
		return fmt.Sprintf("rech_type_implements(%s, %s)", recvCode, argCodes[0]), typeBool, nil
	default:
		return "", typeUnknown, fmt.Errorf("генерация C из IR: у \"Тип\" нет метода %q", e.Method)
	}
}

// reflectBoxStructFuncName/reflectGetterName/reflectSetterName/
// reflectInvokerName/reflectCtorName - имена генерируемых C-функций на
// сущность/структуру ent (или на пару ent+field/method).
func reflectBoxStructFuncName(ent string) string { return "rech_reflect_box_" + transliterate(ent) }
func reflectGetterName(ent, field string) string {
	return "rech_reflect_get_" + transliterate(ent) + "_" + transliterate(field)
}
func reflectSetterName(ent, field string) string {
	return "rech_reflect_set_" + transliterate(ent) + "_" + transliterate(field)
}
func reflectInvokerName(ent, method string) string {
	return "rech_reflect_invoke_" + transliterate(ent) + "_" + transliterate(method)
}
func reflectCtorName(ent string) string { return "rech_reflect_ctor_" + transliterate(ent) }

// effectiveReflectMethod - метод m сущности typeName вместе с owner -
// именем сущности, физически определяющей его C-функцию (mangleMethod(owner,
// m.Name)) - у неизменённого унаследованного метода это БАЗОВАЯ сущность,
// не typeName (см. package doc ir.Entity.Methods).
type effectiveReflectMethod struct {
	m     *ir.Method
	owner string
}

// effectiveReflectMethods - методы typeName, включая унаследованные - тот
// же обход цепочки BaseName, что и internal/interp.effectiveMethods
// (не разделяемый напрямую - структурно идентичный, но оперирует разными
// типами карт в разных пакетах). Абстрактный метод (m.IsAbstract) не
// попадает в результат вовсе - у него нет C-функции, которую мог бы
// обернуть methodInvokerCode (см. её package doc); для любой РЕАЛЬНО
// создаваемой сущности это не теряет ни одного метода (семантический
// анализ гарантирует, что настоящая реализация есть раньше по цепочке и
// перекрывает абстрактную запись через seen), эффект виден только если
// рефлексии подвергается сама абстрактная сущность напрямую ("тип
// АбстрактнаяСущность").
func effectiveReflectMethods(entities map[string]*ir.Entity, typeName string) []effectiveReflectMethod {
	var chain []string
	for n := typeName; n != ""; {
		ent, ok := entities[n]
		if !ok {
			break
		}
		chain = append(chain, n)
		n = ent.BaseName
	}
	seen := map[string]bool{}
	var result []effectiveReflectMethod
	for _, n := range chain {
		for _, m := range entities[n].Methods {
			if seen[m.Name] || m.IsAbstract {
				continue
			}
			seen[m.Name] = true
			result = append(result, effectiveReflectMethod{m: m, owner: n})
		}
	}
	return result
}

// reflectGen - генератор дескрипторов рефлексии для prog.ReflectTargets и
// всего, что из них рекурсивно достижимо - тот же принцип "ensure" по
// памяти, что и jsonExtractorGen (json_extract.go).
type reflectGen struct {
	ctx       *irGenCtx
	generated map[string]bool
	// generics - имя инстанциации ("Список<Целое>"/"Словарь<Строка,
	// Значение>") -> её описание (ElemType/KeyType) - НЕ то же самое, что
	// ctx.generics (тот - только map[string]bool, "существует ли вообще",
	// без ElemType/KeyType, которые нужны здесь для рекурсивного
	// ensure(ElemType)/ensure(KeyType), см. package doc ensure).
	generics map[string]ir.GenericInstantiation
	// thunksGenerated - методы-обёртки (methodInvokerCode) по ключу
	// "owner.method", ОТДЕЛЬНО от generated (который по сущности целиком) -
	// unchanged-унаследованный метод именуется по owner (базовой сущности,
	// см. package doc effectiveReflectMethod), поэтому две разные
	// сущности-потомка, каждая - собственная цель рефлексии, независимо
	// приходят к ОДНОМУ И ТОМУ ЖЕ имени обёртки - без этой памяти вторая
	// сгенерировала бы дублирующее определение C-функции.
	thunksGenerated map[string]bool
	// implementors - см. package doc ir.Program.InterfaceImplementors -
	// нужен ensureInterface (implementors[Имя] - сущности, чьи дескрипторы
	// эмитятся ДО дескриптора интерфейса, чтобы массив rech_iface_impls_*
	// мог сослаться на их адреса) и interfaceImplementorsArrayCode.
	implementors map[string][]string
	code         strings.Builder
}

// reflectRuntimeCode - точка входа: primitiveReflectDescriptors уже
// эмитятся раньше (см. generateSource) - эта функция генерирует код на
// сущности/структуры/интерфейсы и (см. ensure) на встреченные внутри них
// generic-коллекции (Список/Стопка/Очередь/Словарь).
func reflectRuntimeCode(ctx *irGenCtx, targets []ir.Type, generics []ir.GenericInstantiation, implementors map[string][]string) (string, error) {
	genericsByName := make(map[string]ir.GenericInstantiation, len(generics))
	for _, gi := range generics {
		genericsByName[gi.Name] = gi
	}
	g := &reflectGen{ctx: ctx, generated: map[string]bool{}, generics: genericsByName, thunksGenerated: map[string]bool{}, implementors: implementors}
	for _, t := range targets {
		if err := g.ensure(t); err != nil {
			return "", err
		}
	}
	return g.code.String(), nil
}

// ensure - см. package doc: генерирует дескриптор t (если t - сущность/
// структура и ещё не сгенерирован), рекурсивно обеспечивая дескрипторы
// всего, что t использует (тип поля, тип параметра/результата метода,
// тип параметра/результата конструктора) ПЕРЕД собственным дескриптором
// t - иначе таблицы t ссылались бы на ещё не объявленный &дескриптор.
//
// **Найденный и исправленный баг**: t.Kind == ir.KindEntity ЗАОДНО и у
// встроенной generic-коллекции ("Список<Целое>"/"Словарь<Строка,
// Значение>" - см. package doc ir.GenericInstantiation, у неё нет
// собственного ir.Entity-узла) - раньше это безусловно шло в
// g.ctx.entities[t.Entity], которого для generic-коллекции нет вовсе -
// "рефлексия по неизвестному типу", как только сущность-цель рефлексии
// несла поле такого типа (docs/ROADMAP.md, Шаг 41). Исправлено -
// генериками ensureGeneric ниже, отдельной, более простой веткой (у
// коллекции нет ни полей, ни методов, ни конструктора в смысле рефлексии -
// только элемент/ключ, которые сами могут быть сущностями и тоже требуют
// ensure).
func (g *reflectGen) ensure(t ir.Type) error {
	if t.Kind == ir.KindNullable {
		// НАЙДЕННЫЙ БАГ: без этой ветки ensure молча возвращала nil для
		// "?Тип" (KindNullable != KindEntity, попадала в общий отказ ниже) -
		// внутренний тип нигде не получал ensure(), значит сущность,
		// видимая рефлексии ТОЛЬКО как "?Тип" метода/поля (например,
		// "Задача" - только как результат "статус(id): ?Задача", никогда
		// не боксирована сама по себе), могла остаться БЕЗ собственного
		// дескриптора вовсе - nullableReflectBoxHelpers (ir_generator.go)
		// ссылалась бы на несуществующий rech_type_meta_zadacha при попытке
		// боксировать развёрнутое значение.
		inner, ok := g.ctx.nullableInner[t.Entity]
		if !ok {
			return nil
		}
		return g.ensure(inner)
	}
	if t.Kind == ir.KindInterface {
		if g.generated[t.Entity] {
			return nil
		}
		g.generated[t.Entity] = true
		return g.ensureInterface(t)
	}
	if t.Kind != ir.KindEntity {
		return nil
	}
	if g.generated[t.Entity] {
		return nil
	}
	g.generated[t.Entity] = true

	if gi, isGeneric := g.generics[t.Entity]; isGeneric {
		return g.ensureGeneric(t, gi)
	}

	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: рефлексия по неизвестному типу %q", t.Entity)
	}

	// Опережающее объявление без инициализатора (само по себе валидное
	// определение с внешним связыванием по умолчанию у static - "tentative
	// definition", C99 6.9.2) - ДО ЛЮБОЙ рекурсии ensure() ниже (поля/
	// методы/конструктор/base/interfaces), не только до структуры-обёртки/
	// геттеров/сеттеров/инвокеров/typeDescriptorCode этой же сущности, как
	// было раньше.
	//
	// **Найденный и исправленный баг**: раньше тентативное объявление
	// писалось ПОСЛЕ всех этих ensure()-вызовов - для настоящего взаимного
	// цикла (сущность X реализует интерфейс I, I.implementors включает ТУ
	// ЖЕ X - см. entityInterfaces/ensureInterface ниже) это давало
	// "неизвестный идентификатор": ensure(X) помечает g.generated[X]=true
	// В САМОМ НАЧАЛЕ (до тентативного объявления, которое было ещё дальше),
	// затем ensure(X) рекурсивно вызывает ensure(I) (X реализует I), тот -
	// ensure(X) СНОВА (I.implementors включает X) - g.generated[X] уже
	// true, возврат БЕЗ единой написанной строки - ensureInterface(I)
	// продолжает и пишет "rech_iface_impls_I[] = {&rech_type_meta_X}",
	// ссылаясь на X, у которого на этот момент ещё нет НИ тентативного, НИ
	// полного объявления. Тот же класс бага теоретически возможен и для
	// прямого взаимного поля между двумя сущностями (X несёт поле типа Y,
	// Y - поле типа X) - не был замечен раньше, потому что такого поля в
	// existing examples/тестах не было; этот фикс закрывает оба случая
	// сразу одним и тем же перемещением.
	fmt.Fprintf(&g.code, "static const rech_type_meta_t %s;\n", reflectMetaVarFor(ir.EntityType(ent.Name)))

	for _, f := range ent.Fields {
		if err := g.ensure(f.Type); err != nil {
			return err
		}
	}
	ems := effectiveReflectMethods(g.ctx.entities, t.Entity)
	for _, em := range ems {
		for _, p := range em.m.Params {
			if err := g.ensure(p.Type); err != nil {
				return err
			}
		}
		if em.m.HasReturn {
			if err := g.ensure(em.m.ReturnType); err != nil {
				return err
			}
		}
	}
	if ent.Ctor != nil {
		for _, p := range ent.Ctor.Params {
			if err := g.ensure(p.Type); err != nil {
				return err
			}
		}
	}
	// base/interfaces (docs/ROADMAP.md - "базовый_тип"/"наследует_ли"/"число_
	// интерфейсов"/"получить_интерфейс"/"реализует_ли", паритет с C# Type.BaseType/
	// GetInterfaces).
	if ent.BaseName != "" {
		if err := g.ensure(ir.EntityType(ent.BaseName)); err != nil {
			return err
		}
	}
	ifaceNames := g.ctx.entityInterfaces[t.Entity]
	for _, name := range ifaceNames {
		if err := g.ensure(ir.InterfaceType(name)); err != nil {
			return err
		}
	}

	if !g.ctx.entityIsRef(ent.Name) {
		g.code.WriteString(g.structBoxFuncCode(ent))
	}
	for _, f := range ent.Fields {
		g.code.WriteString(g.fieldAccessorsCode(ent, f))
	}
	for _, em := range ems {
		thunkKey := em.owner + "." + em.m.Name
		if g.thunksGenerated[thunkKey] {
			continue
		}
		g.thunksGenerated[thunkKey] = true
		g.code.WriteString(g.methodInvokerCode(em))
	}
	g.code.WriteString(g.ctorInvokerCode(ent))
	g.code.WriteString(g.typeDescriptorCode(ent, ems, ifaceNames))
	return nil
}

// ensureGeneric - см. package doc ensure: дескриптор generic-коллекции gi -
// НЕТ полей/методов/конструктора в смысле рефлексии (нечего перечислить -
// реализация не видна в Rech, ir.GenericInstantiation package doc), только
// имя - тем же тривиальным форматом, что и у примитивов
// (primitiveReflectDescriptors, "{...}", NULL/0 везде, кроме имени).
// ElemType (и KeyType у Словарь) - рекурсивно ensure(), как обычное поле:
// "Список<Пользователь>" сам не рефлектируется как сущность-с-полями, но
// "Пользователь" внутри него - вполне может встретиться отдельно (например,
// через ".получить(i)" где-то ещё в программе) и обязан иметь свой
// дескриптор к этому моменту.
func (g *reflectGen) ensureGeneric(t ir.Type, gi ir.GenericInstantiation) error {
	if err := g.ensure(gi.ElemType); err != nil {
		return err
	}
	if gi.Template == "Словарь" {
		if err := g.ensure(gi.KeyType); err != nil {
			return err
		}
	}
	fmt.Fprintf(&g.code, "static const rech_type_meta_t %s = {\"%s\", NULL, 0, NULL, 0, NULL, 0, NULL, NULL, 0, \"\"};\n",
		reflectMetaVarFor(t), escapeCString(t.Entity))
	return nil
}

// ensureInterface - дескриптор интерфейса t (docs/ROADMAP.md - находки
// "тип ИмяИнтерфейса"/"значение как ИмяИнтерфейса") - в отличие от
// ensure/ensureGeneric выше, у интерфейса НЕТ полей и НЕТ конструктора
// (ctor рефлексивно недостижим - "тип.создать" для имени, которого нет в
// g.ctx.entities, уже отдельно и корректно паникует "метод не найден" -
// см. reflectCreate/её аналог в internal/interp), а методы несут invoke =
// NULL: диспетчеризация "значение.вызвать(...)" ВСЕГДА идёт через v.type
// боксированного значения, а это ВСЕГДА дескриптор КОНКРЕТНОЙ сущности
// (боксирование интерфейсно-типизированного ВЫРАЖЕНИЯ само по себе не
// поддержано - reflectBoxExprCode ниже не различает KindInterface,
// боксируется всегда конкретный, статически известный тип), значит invoke
// у дескриптора самого интерфейса не разыменовывается в реальном пути
// исполнения ни разу.
//
// Отдельно эмитит rech_iface_impls_<Имя> - статический массив указателей
// на дескрипторы ВСЕХ сущностей программы, реально реализующих t
// (g.implementors, вычислено транзитивно один раз в internal/semantic,
// entitySatisfies) - используется reflectCheckedUnboxExprCode для
// "значение как ИмяИнтерфейса": несовпадение v.type ни с ОДНИМ элементом
// этого списка - паника (rech_unbox_check_iface), не точное сравнение с
// единственным ожидаемым дескриптором, как у rech_unbox_check.
func (g *reflectGen) ensureInterface(t ir.Type) error {
	iface, ok := g.ctx.interfaces[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: рефлексия по неизвестному интерфейсу %q", t.Entity)
	}
	// Опережающее объявление - тот же приём (и по той же причине), что и у
	// ensure (сущности) выше: методы ниже и implementors теоретически могут
	// рекурсивно сослаться на ЭТОТ ЖЕ интерфейс (поле/параметр/результат
	// метода реализующей сущности типа этого самого интерфейса).
	fmt.Fprintf(&g.code, "static const rech_type_meta_t %s;\n", reflectMetaVarFor(t))
	for _, m := range iface.Methods {
		for _, p := range m.Params {
			if err := g.ensure(p.Type); err != nil {
				return err
			}
		}
		if m.HasReturn {
			if err := g.ensure(m.ReturnType); err != nil {
				return err
			}
		}
	}
	implementors := g.implementors[t.Entity]
	for _, entName := range implementors {
		if err := g.ensure(ir.EntityType(entName)); err != nil {
			return err
		}
	}
	g.code.WriteString(g.interfaceTypeDescriptorCode(iface))
	g.code.WriteString(interfaceImplementorsArrayCode(t.Entity, implementors))
	return nil
}

// interfaceTypeDescriptorCode - тот же формат, что typeDescriptorCode у
// сущности (ниже по файлу), но без полей/конструктора - см. package doc
// ensureInterface.
func (g *reflectGen) interfaceTypeDescriptorCode(iface *ir.Interface) string {
	metaName := reflectMetaVarFor(ir.InterfaceType(iface.Name))
	var sb strings.Builder

	methodsArr := "NULL"
	if len(iface.Methods) > 0 {
		methodsArr = metaName + "_methods"
		for _, m := range iface.Methods {
			if len(m.Params) == 0 {
				continue
			}
			arrName := metaName + "_params_" + transliterate(m.Name)
			parts := make([]string, len(m.Params))
			for i, p := range m.Params {
				parts[i] = "&" + reflectMetaVarFor(p.Type)
			}
			fmt.Fprintf(&sb, "static const rech_type_meta_t *const %s[] = {%s};\n", arrName, strings.Join(parts, ", "))
		}
		fmt.Fprintf(&sb, "static const rech_method_meta_t %s[] = {\n", methodsArr)
		for _, m := range iface.Methods {
			paramsRef := "NULL"
			if len(m.Params) > 0 {
				paramsRef = metaName + "_params_" + transliterate(m.Name)
			}
			returnMeta := reflectMetaName(reflectPustoName)
			if m.HasReturn {
				returnMeta = reflectMetaVarFor(m.ReturnType)
			}
			fmt.Fprintf(&sb, "    {\"%s\", %d, %s, &%s, NULL, \"\"},\n", escapeCString(m.Name), len(m.Params), paramsRef, returnMeta)
		}
		sb.WriteString("};\n")
	}

	// base/interfaces/interface_count/tag - всегда NULL/NULL/0/NULL у
	// дескриптора САМОГО интерфейса: "базовый_тип"/"наследует_ли"/"число_
	// интерфейсов"/"получить_интерфейс"/"реализует_ли" (docs/ROADMAP.md)
	// действуют только на сущность/структуру - интерфейс не "наследует"
	// сущность и не "реализует" другой интерфейс тем же словом (у самого
	// интерфейса есть только "наследует" МЕЖДУ интерфейсами, уже полностью
	// отражённое плоским списком iface.Methods выше, отдельного смысла в
	// этих трёх полях для интерфейса нет); собственного тега у интерфейса
	// в этом срезе тоже нет (только у сущности/структуры/метода/поля - см.
	// ast.EntityDecl.Tag/MethodDecl.Tag/Field.Tag - парсер не принимает тег
	// после "интерфейс ...", осознанно уже, не забыто).
	fmt.Fprintf(&sb, "static const rech_type_meta_t %s = {\"%s\", NULL, 0, %s, %d, NULL, 0, NULL, NULL, 0, \"\"};\n\n",
		metaName, escapeCString(iface.Name), methodsArr, len(iface.Methods))
	return sb.String()
}

// interfaceImplementorsArrayName/interfaceImplementorsArrayCode - см.
// package doc ensureInterface. Пустой implementors (интерфейс без единой
// реализующей сущности в программе) - всё равно валидный C нужен: массив
// нулевой длины без инициализатора недопустим стандартным C - единственный
// NULL-элемент, count (второй аргумент rech_unbox_check_iface на месте
// вызова) при этом всегда 0, элемент физически не читается никогда.
func interfaceImplementorsArrayName(ifaceName string) string {
	return "rech_iface_impls_" + transliterate(ifaceName)
}

func interfaceImplementorsArrayCode(ifaceName string, implementors []string) string {
	if len(implementors) == 0 {
		return fmt.Sprintf("static const rech_type_meta_t *const %s[] = {NULL};\n", interfaceImplementorsArrayName(ifaceName))
	}
	parts := make([]string, len(implementors))
	for i, entName := range implementors {
		parts[i] = "&" + reflectMetaVarFor(ir.EntityType(entName))
	}
	return fmt.Sprintf("static const rech_type_meta_t *const %s[] = {%s};\n", interfaceImplementorsArrayName(ifaceName), strings.Join(parts, ", "))
}

// structBoxFuncCode - "(Значение) x" для структуры (по значению) -
// GC-копия x в отдельный блок памяти (боксированное значение живёт
// независимо от x дальше, тем же принципом, что и internal/interp -
// cloneValue при боксировании) + указатель на неё в rech_any_t.v.ptr.
func (g *reflectGen) structBoxFuncCode(ent *ir.Entity) string {
	cType := mangleType(ent.Name)
	fn := codegen.Func(reflectBoxStructFuncName(ent.Name)).
		Static().
		ReturnType("rech_any_t").
		Params(codegen.Parameter(cType, "v")).
		Statements(
			codegen.Decl(cType+" *", "p", codegen.Call("GC_MALLOC", codegen.Sizeof(codegen.Raw(cType)))),
			codegen.Assign(codegen.Deref(codegen.Var("p")), codegen.Var("v")),
			codegen.Return(codegen.Raw(fmt.Sprintf("(rech_any_t){.type = &%s, .v.ptr = (void *)p}", reflectMetaVarFor(ir.EntityType(ent.Name))))),
		)
	w := codegen.NewWriter()
	fn.Write(w)
	return w.String() + "\n"
}

// fieldAccessorsCode - геттер/сеттер поля f сущности ent - self всегда
// указатель на реальную память объекта (для ref-сущности - сам объект,
// для структуры - GC-копия из structBoxFuncCode), поэтому обе ветки
// приводят его к одному и тому же mangleType(ent.Name) *, без разницы
// ref/структура.
func (g *reflectGen) fieldAccessorsCode(ent *ir.Entity, f ir.Field) string {
	selfExpr := fmt.Sprintf("((%s *)self)->%s", mangleType(ent.Name), mangleField(f.Name))
	getter := codegen.Func(reflectGetterName(ent.Name, f.Name)).
		Static().
		ReturnType("rech_any_t").
		Params(codegen.Parameter("void *", "self")).
		Statements(codegen.Return(codegen.Raw(reflectBoxExprCode(g.ctx, f.Type, selfExpr))))
	setter := codegen.Func(reflectSetterName(ent.Name, f.Name)).
		Static().
		Params(codegen.Parameter("void *", "self"), codegen.Parameter("rech_any_t", "v")).
		Statements(codegen.Assign(codegen.Raw(selfExpr), codegen.Raw(reflectCheckedUnboxExprCode(g.ctx, f.Type, "v"))))
	w := codegen.NewWriter()
	getter.Write(w)
	w.Raw("\n")
	setter.Write(w)
	w.Raw("\n")
	return w.String()
}

// methodInvokerCode - "значение.вызвать(имя, аргументы)" - обёртка с
// type-erased сигнатурой (void*, rech_any_t*, int) -> rech_any_t над
// настоящей C-функцией метода (em.owner - см. package doc
// effectiveReflectMethod). argc не читается - число аргументов уже
// проверено вызывающей стороной (rech_method_by_name/вызывающий код
// "вызвать") перед обращением к invoke.
func (g *reflectGen) methodInvokerCode(em effectiveReflectMethod) string {
	m := em.m
	argExprs := make([]string, len(m.Params)+1)
	argExprs[0] = fmt.Sprintf("(%s *)self", mangleType(em.owner))
	for i, p := range m.Params {
		argExprs[i+1] = reflectCheckedUnboxExprCode(g.ctx, p.Type, fmt.Sprintf("args[%d]", i))
	}
	call := fmt.Sprintf("%s(%s)", mangleMethod(em.owner, m.Name), strings.Join(argExprs, ", "))
	var sb strings.Builder
	fmt.Fprintf(&sb, "static rech_any_t %s(void *self, rech_any_t *args, int argc) {\n", reflectInvokerName(em.owner, m.Name))
	if m.HasReturn {
		fmt.Fprintf(&sb, "    return %s;\n", reflectBoxExprCode(g.ctx, m.ReturnType, call))
	} else {
		fmt.Fprintf(&sb, "    %s;\n    return (rech_any_t){.type = &%s};\n", call, reflectMetaName(reflectPustoName))
	}
	sb.WriteString("}\n\n")
	return sb.String()
}

// ctorInvokerCode - "тип.создать(аргументы)" (Activator.CreateInstance) -
// ref-сущность без явного конструктора/с ним - позиционно по Fields/
// Ctor.Params через уже существующий rech_new_<Тип> (mangleCtor,
// generateIREntityCtor - он сам знает, какую из двух форм строить);
// структура - собственный compound literal (rech_new_<Тип> для структур
// не генерируется вовсе, см. package doc generateSource) + боксирование
// через structBoxFuncCode.
func (g *reflectGen) ctorInvokerCode(ent *ir.Entity) string {
	var paramTypes []ir.Type
	if ent.Ctor != nil {
		for _, p := range ent.Ctor.Params {
			paramTypes = append(paramTypes, p.Type)
		}
	} else {
		for _, f := range ent.Fields {
			paramTypes = append(paramTypes, f.Type)
		}
	}
	argExprs := make([]string, len(paramTypes))
	for i, t := range paramTypes {
		argExprs[i] = reflectCheckedUnboxExprCode(g.ctx, t, fmt.Sprintf("args[%d]", i))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "static rech_any_t %s(rech_any_t *args, int argc) {\n", reflectCtorName(ent.Name))
	if g.ctx.entityIsRef(ent.Name) {
		fmt.Fprintf(&sb, "    return (rech_any_t){.type = &%s, .v.ptr = (void *)%s(%s)};\n",
			reflectMetaVarFor(ir.EntityType(ent.Name)), mangleCtor(ent.Name), strings.Join(argExprs, ", "))
	} else {
		cType := mangleType(ent.Name)
		parts := make([]string, len(ent.Fields))
		for i, f := range ent.Fields {
			parts[i] = fmt.Sprintf(".%s = %s", mangleField(f.Name), argExprs[i])
		}
		initList := strings.Join(parts, ", ")
		if initList == "" {
			initList = "0"
		}
		fmt.Fprintf(&sb, "    %s v = (%s){%s};\n", cType, cType, initList)
		fmt.Fprintf(&sb, "    return %s(v);\n", reflectBoxStructFuncName(ent.Name))
	}
	sb.WriteString("}\n\n")
	return sb.String()
}

// typeDescriptorCode - сам дескриптор ent целиком: статические массивы
// метаданных полей/методов (и, на каждый метод с параметрами - отдельный
// статический массив указателей на дескрипторы типов параметров) плюс
// сама struct rech_type_meta. ifaceNames - g.ctx.entityInterfaces[ent.Name]
// (ir.Program.EntityInterfaces, транзитивно через entitySatisfies) -
// уже посчитан вызывающей стороной (ensure выше, там же обеспечены и их
// дескрипторы) - "базовый_тип"/"наследует_ли"/"число_интерфейсов"/
// "получить_интерфейс"/"реализует_ли" (docs/ROADMAP.md, паритет с C# Type.BaseType/
// GetInterfaces).
func (g *reflectGen) typeDescriptorCode(ent *ir.Entity, ems []effectiveReflectMethod, ifaceNames []string) string {
	metaName := reflectMetaVarFor(ir.EntityType(ent.Name))
	var sb strings.Builder

	fieldsArr := "NULL"
	if len(ent.Fields) > 0 {
		fieldsArr = metaName + "_fields"
		fmt.Fprintf(&sb, "static const rech_field_meta_t %s[] = {\n", fieldsArr)
		for _, f := range ent.Fields {
			fmt.Fprintf(&sb, "    {\"%s\", &%s, %s, %s, \"%s\"},\n", escapeCString(f.Name), reflectMetaVarFor(f.Type),
				reflectGetterName(ent.Name, f.Name), reflectSetterName(ent.Name, f.Name), escapeCString(f.Tag))
		}
		sb.WriteString("};\n")
	}

	methodsArr := "NULL"
	if len(ems) > 0 {
		methodsArr = metaName + "_methods"
		for _, em := range ems {
			if len(em.m.Params) == 0 {
				continue
			}
			arrName := metaName + "_params_" + transliterate(em.m.Name)
			parts := make([]string, len(em.m.Params))
			for i, p := range em.m.Params {
				parts[i] = "&" + reflectMetaVarFor(p.Type)
			}
			fmt.Fprintf(&sb, "static const rech_type_meta_t *const %s[] = {%s};\n", arrName, strings.Join(parts, ", "))
		}
		fmt.Fprintf(&sb, "static const rech_method_meta_t %s[] = {\n", methodsArr)
		for _, em := range ems {
			paramsRef := "NULL"
			if len(em.m.Params) > 0 {
				paramsRef = metaName + "_params_" + transliterate(em.m.Name)
			}
			returnMeta := reflectMetaName(reflectPustoName)
			if em.m.HasReturn {
				returnMeta = reflectMetaVarFor(em.m.ReturnType)
			}
			fmt.Fprintf(&sb, "    {\"%s\", %d, %s, &%s, %s, \"%s\"},\n", escapeCString(em.m.Name), len(em.m.Params), paramsRef, returnMeta, reflectInvokerName(em.owner, em.m.Name), escapeCString(em.m.Tag))
		}
		sb.WriteString("};\n")
	}

	ctorParamCount := len(ent.Fields)
	if ent.Ctor != nil {
		ctorParamCount = len(ent.Ctor.Params)
	}

	base := "NULL"
	if ent.BaseName != "" {
		base = "&" + reflectMetaVarFor(ir.EntityType(ent.BaseName))
	}
	interfacesArr := "NULL"
	if len(ifaceNames) > 0 {
		interfacesArr = metaName + "_interfaces"
		parts := make([]string, len(ifaceNames))
		for i, name := range ifaceNames {
			parts[i] = "&" + reflectMetaVarFor(ir.InterfaceType(name))
		}
		fmt.Fprintf(&sb, "static const rech_type_meta_t *const %s[] = {%s};\n", interfacesArr, strings.Join(parts, ", "))
	}

	fmt.Fprintf(&sb, "static const rech_type_meta_t %s = {\"%s\", %s, %d, %s, %d, %s, %d, %s, %s, %d, \"%s\"};\n\n",
		metaName, escapeCString(ent.Name), fieldsArr, len(ent.Fields), methodsArr, len(ems), reflectCtorName(ent.Name), ctorParamCount,
		base, interfacesArr, len(ifaceNames), escapeCString(ent.Tag))
	return sb.String()
}
