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
	escapeCString(rtmsg.ReflectIndexOutOfRangeTemplate), escapeCString(rtmsg.ReflectFieldNotFoundTemplate),
	escapeCString(rtmsg.ReflectFieldNotFoundTemplate))

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
	return fmt.Sprintf(`static rech_any_t rech_reflect_invoke(rech_any_t self, const char *name, %[1]s args) {
    const rech_method_meta_t *m = rech_method_by_name(self.type, name);
    if (!m) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[2]s", name, self.type->name);
        rech_panic(msg);
    }
    if (m->param_count != (int)args.len) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[3]s", name, m->param_count, (int)args.len);
        rech_panic(msg);
    }
    return m->invoke(self.v.ptr, args.data, (int)args.len);
}

static rech_any_t rech_reflect_create(const rech_type_meta_t *t, %[1]s args) {
    if (!t->ctor) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[2]s", "создать", t->name);
        rech_panic(msg);
    }
    if (t->ctor_param_count != (int)args.len) {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[3]s", "создать", t->ctor_param_count, (int)args.len);
        rech_panic(msg);
    }
    return t->ctor(args.data, (int)args.len);
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
// и у Целое), Файл/Сокет/Вызываемый/Задача - НЕТ (рефлексия над
// непрозрачными дескрипторами не имеет практического смысла, не
// запрошена).
// KindNull ("Ничто" как тип самого по себе, не "?Тип" - см. package doc
// ir.KindNull) входит в этот список наравне с обычными примитивами - им
// боксируется SQL NULL в результате "запросить" (internal/backend/
// cbackend/db.go), проверяется как "результат.получить(колонка).
// тип_значения().имя() == \"Ничто\"".
var reflectPrimitiveKinds = []ir.TypeKind{
	ir.KindInt, ir.KindInt8, ir.KindInt16, ir.KindInt32, ir.KindInt64,
	ir.KindUint, ir.KindUint8, ir.KindUint16, ir.KindUint32, ir.KindUint64,
	ir.KindFloat32, ir.KindFloat, ir.KindString, ir.KindBool, ir.KindChar, ir.KindByte,
	ir.KindNull,
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
		fmt.Fprintf(&sb, "static const rech_type_meta_t %s = {\"%s\", NULL, 0, NULL, 0, NULL, 0};\n", reflectMetaName(name), escapeCString(name))
	}
	fmt.Fprintf(&sb, "static const rech_type_meta_t %s = {\"%s\", NULL, 0, NULL, 0, NULL, 0};\n", reflectMetaName(reflectPustoName), escapeCString(reflectPustoName))
	sb.WriteString("\n")
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
	default:
		return fmt.Sprintf("(rech_any_t){.type = &%s, .v.i = (long long)(%s)}", meta, cExpr)
	}
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
	case "создать":
		return fmt.Sprintf("rech_reflect_create(%s, %s)", recvCode, argCodes[0]), typeAny, nil
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
	// thunksGenerated - методы-обёртки (methodInvokerCode) по ключу
	// "owner.method", ОТДЕЛЬНО от generated (который по сущности целиком) -
	// unchanged-унаследованный метод именуется по owner (базовой сущности,
	// см. package doc effectiveReflectMethod), поэтому две разные
	// сущности-потомка, каждая - собственная цель рефлексии, независимо
	// приходят к ОДНОМУ И ТОМУ ЖЕ имени обёртки - без этой памяти вторая
	// сгенерировала бы дублирующее определение C-функции.
	thunksGenerated map[string]bool
	code            strings.Builder
}

// reflectRuntimeCode - точка входа: primitiveReflectDescriptors уже
// эмитятся раньше (см. generateSource) - эта функция генерирует только
// код на сущности/структуры.
func reflectRuntimeCode(ctx *irGenCtx, targets []ir.Type) (string, error) {
	g := &reflectGen{ctx: ctx, generated: map[string]bool{}, thunksGenerated: map[string]bool{}}
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
func (g *reflectGen) ensure(t ir.Type) error {
	if t.Kind != ir.KindEntity {
		return nil
	}
	if g.generated[t.Entity] {
		return nil
	}
	g.generated[t.Entity] = true

	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: рефлексия по неизвестному типу %q", t.Entity)
	}
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

	// Опережающее объявление без инициализатора (само по себе валидное
	// определение с внешним связыванием по умолчанию у static - "tentative
	// definition", C99 6.9.2) - структура-обёртка/геттеры/сеттеры/инвокеры/
	// конструктор ниже берут адрес &rech_type_meta_<Имя> ДО того, как
	// typeDescriptorCode напишет настоящий инициализатор (который сам, в
	// свою очередь, ссылается на их адреса) - разрывает иначе неразрешимый
	// взаимный цикл вперёд.
	fmt.Fprintf(&g.code, "static const rech_type_meta_t %s;\n", reflectMetaVarFor(ir.EntityType(ent.Name)))
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
	g.code.WriteString(g.typeDescriptorCode(ent, ems))
	return nil
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
// сама struct rech_type_meta.
func (g *reflectGen) typeDescriptorCode(ent *ir.Entity, ems []effectiveReflectMethod) string {
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
			fmt.Fprintf(&sb, "    {\"%s\", %d, %s, &%s, %s},\n", escapeCString(em.m.Name), len(em.m.Params), paramsRef, returnMeta, reflectInvokerName(em.owner, em.m.Name))
		}
		sb.WriteString("};\n")
	}

	ctorParamCount := len(ent.Fields)
	if ent.Ctor != nil {
		ctorParamCount = len(ent.Ctor.Params)
	}
	fmt.Fprintf(&sb, "static const rech_type_meta_t %s = {\"%s\", %s, %d, %s, %d, %s, %d};\n\n",
		metaName, escapeCString(ent.Name), fieldsArr, len(ent.Fields), methodsArr, len(ems), reflectCtorName(ent.Name), ctorParamCount)
	return sb.String()
}
