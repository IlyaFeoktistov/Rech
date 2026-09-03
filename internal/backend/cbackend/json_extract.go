// "разобрать JSON <выражение> как <Тип>" (internal/ir.ParseExpr) - см.
// internal/semantic/parse.go, package doc для проверки JSON-совместимости
// целевого типа.
//
// C-представление разобранного текста - "rech_jv", размеченное объединение
// (kind + один активный вариант), построенное ручным рекурсивным парсером
// (jsonParserCode) - никакая сторонняя библиотека не подключается, тем же
// принципом, что и остальной рантайм cbackend (rech_panic/socketHelpers/
// httpHelpers и т. д. - всё написано с нуля). Память - через GC_MALLOC
// (Boehm GC уже безусловно слинкован, см. package doc generateSource,
// usesGC) - без единого free, как и весь остальной cbackend.
//
// Для каждого типа из prog.ParseTargets (и рекурсивно для любого
// достижимого из него типа поля сущности/элемента Список<T>)
// jsonRuntimeCode генерирует ОДНУ функцию извлечения
// "rech_json_extract_<mangled>(rech_jv v) -> <CType>", проверяющую вид
// JSON-значения и либо возвращающую C-значение целевого типа, либо
// вызывающую rech_struct_throw - несовпадение (отсутствующее поле/не тот вид
// JSON-значения) выбрасывает перехватываемое "ОшибкаРазбора" через уже
// существующий канал исключений (rech_throw, exceptions.go), не
// rech_panic: JSON-текст - внешние данные, ошибка в нём - не баг
// программы. jsonExtractorGen.generated - память между рекурсивными
// вызовами ensure, чтобы тип, достижимый из нескольких целей разбора
// (или несколько раз из одной), получил ровно одну функцию.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// parseErrorEntity - см. internal/semantic/parse.go, тот же принцип общего
// идентификатора константы, что и systemExceptionEntity.
const parseErrorEntity = "ОшибкаРазбора"

// structParseThrowHelperCode - rech_struct_throw - общий для JSON- и
// XML-разбора (обе стороны просто вызывают "выбросить" уже существующим
// каналом, rech_throw/exceptions.go) - эмитится один раз, когда нужен хотя
// бы один из двух форматов (len(ParseTargets) > 0 || len(XMLParseTargets)
// > 0, см. ir_generator.go), а не отдельно в jsonRuntimeCode/xmlRuntimeCode
// - иначе программа, использующая оба формата, получила бы два одинаковых
// определения этой функции.
func structParseThrowHelperCode() string {
	return fmt.Sprintf("static void rech_struct_throw(const char *msg) {\n    rech_throw(%s(msg), \"%s\");\n}\n\n",
		mangleCtor(parseErrorEntity), escapeCString(parseErrorEntity))
}

// divZeroThrowHelperCode - rech_divzero_throw - тот же принцип, что и
// rech_struct_throw выше, для целочисленного деления на ноль
// (internal/backend/cbackend/builtins_shared.go, rech_int_div/rech_uint_div):
// выбрасывает перехватываемое "ОшибкаДеленияНаНоль" через rech_throw, не
// rech_panic. Определение эмитится ПОСЛЕ exceptionRuntimeCode (нужен
// rech_throw уже объявленным) - см. divZeroExceptionEntity в
// internal/semantic/exception.go и прототип в protos (ir_generator.go), без
// которого rech_int_div/rech_uint_div (эмитятся раньше по файлу) не смогли
// бы её вызвать.
func divZeroThrowHelperCode() string {
	return fmt.Sprintf("static void rech_divzero_throw(const char *msg) {\n    rech_throw(%s(msg), \"%s\");\n}\n\n",
		mangleCtor(divByZeroExceptionEntity), escapeCString(divByZeroExceptionEntity))
}

func mangleJSONExtractor(name string) string {
	return "rech_json_extract_" + transliterate(name)
}

// jsonPrimitiveExtractors - фиксированные имена функций извлечения для
// пяти JSON-совместимых примитивов (internal/semantic/parse.go,
// jsonCompatible) - отдельные константы, не mangleJSONExtractor(имя типа),
// потому что примитивы не сущности и generic-параметром cbackend не
// участвуют в транслитерации специально под это.
var jsonPrimitiveExtractors = map[ir.TypeKind]string{
	ir.KindInt:     "rech_json_extract_int",
	ir.KindFloat32: "rech_json_extract_float32",
	ir.KindFloat:   "rech_json_extract_float64",
	ir.KindString:  "rech_json_extract_string",
	ir.KindBool:    "rech_json_extract_bool",
}

type jsonExtractorGen struct {
	ctx       *irGenCtx
	generics  map[string]ir.GenericInstantiation
	generated map[string]bool
	code      strings.Builder
}

// jsonRuntimeCode возвращает C-текст парсера JSON и функций извлечения для
// targets (prog.ParseTargets) и всего, что из них достижимо - вызывается,
// только когда targets непуст (см. ctx.usesJSON, ir_generator.go).
func jsonRuntimeCode(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error) {
	generics := make(map[string]ir.GenericInstantiation, len(prog.Generics))
	for _, g := range prog.Generics {
		generics[g.Name] = g
	}
	g := &jsonExtractorGen{ctx: ctx, generics: generics, generated: map[string]bool{}}
	for _, t := range targets {
		if err := g.ensure(t); err != nil {
			return "", err
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "#define RECH_JSON_PARSE_ERR \"%s\"\n", escapeCString(rtmsg.JSONParseError))
	sb.WriteString(jsonParserCode)
	sb.WriteString(g.code.String())
	return sb.String(), nil
}

func jsonTypeKey(t ir.Type) string {
	if t.Kind == ir.KindEntity {
		return "E:" + t.Entity
	}
	return fmt.Sprintf("K:%d", t.Kind)
}

// ensure генерирует функцию извлечения для t, если она ещё не
// сгенерирована (память по jsonTypeKey) - рекурсивно обеспечивает функции
// для всего, что t использует (элемент Список<T>, поля сущности) ПЕРЕД
// собственным телом t, чтобы вызываемая функция была уже объявлена (в C99
// нет неявного объявления функций).
func (g *jsonExtractorGen) ensure(t ir.Type) error {
	key := jsonTypeKey(t)
	if g.generated[key] {
		return nil
	}
	g.generated[key] = true

	if name, ok := jsonPrimitiveExtractors[t.Kind]; ok {
		g.code.WriteString(jsonPrimitiveExtractorCode(name, t))
		return nil
	}
	if t.Kind != ir.KindEntity {
		return fmt.Errorf("генерация C из IR: \"разобрать JSON\" в неподдержанный тип %s", t)
	}
	if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
		if err := g.ensure(gi.ElemType); err != nil {
			return err
		}
		g.code.WriteString(g.jsonListExtractorCode(gi))
		return nil
	}
	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: \"разобрать JSON\" в неизвестный тип %q", t.Entity)
	}
	for _, f := range ent.Fields {
		if err := g.ensure(f.Type); err != nil {
			return err
		}
	}
	g.code.WriteString(g.jsonEntityExtractorCode(ent))
	return nil
}

func jsonPrimitiveExtractorCode(name string, t ir.Type) string {
	mismatch := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.JSONValueMismatch))
	switch t.Kind {
	case ir.KindInt:
		return fmt.Sprintf(`static long long %s(rech_jv v) {
    if (v.kind != RECH_JV_NUM) { %s }
    long long i = (long long)v.num;
    if ((double)i != v.num) { %s }
    return i;
}

`, name, mismatch, mismatch)
	case ir.KindFloat32:
		return fmt.Sprintf(`static float %s(rech_jv v) {
    if (v.kind != RECH_JV_NUM) { %s }
    return (float)v.num;
}

`, name, mismatch)
	case ir.KindFloat:
		return fmt.Sprintf(`static double %s(rech_jv v) {
    if (v.kind != RECH_JV_NUM) { %s }
    return v.num;
}

`, name, mismatch)
	case ir.KindString:
		return fmt.Sprintf(`static const char *%s(rech_jv v) {
    if (v.kind != RECH_JV_STR) { %s }
    return v.str;
}

`, name, mismatch)
	default: // ir.KindBool
		return fmt.Sprintf(`static bool %s(rech_jv v) {
    if (v.kind != RECH_JV_BOOL) { %s }
    return v.b;
}

`, name, mismatch)
	}
}

func (g *jsonExtractorGen) jsonListExtractorCode(gi ir.GenericInstantiation) string {
	listC := mangleType(gi.Name)
	ctor := mangleCtor(gi.Name)
	addFn := mangleMethod(gi.Name, "добавить")
	elemExtractor := jsonExtractorNameFor(gi.ElemType)
	mismatch := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.JSONValueMismatch))
	return fmt.Sprintf(`static %[1]s *%[2]s(rech_jv v) {
    if (v.kind != RECH_JV_ARR) { %[3]s }
    %[1]s *out = %[6]s();
    for (long long i = 0; i < v.count; i++) {
        %[4]s(out, %[5]s(v.items[i]));
    }
    return out;
}

`, listC, mangleJSONExtractor(gi.Name), mismatch, addFn, elemExtractor, ctor)
}

func (g *jsonExtractorGen) jsonEntityExtractorCode(ent *ir.Entity) string {
	retC := fromIRType(ir.EntityType(ent.Name)).cType(g.ctx)
	mismatch := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.JSONValueMismatch))

	var decls strings.Builder
	args := make([]string, len(ent.Fields))
	for i, f := range ent.Fields {
		fieldC := fromIRType(f.Type).cType(g.ctx)
		extractor := jsonExtractorNameFor(f.Type)
		fvVar := fmt.Sprintf("fv%d", i)
		foundVar := fmt.Sprintf("found%d", i)
		key := f.JSONKey()
		missing := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.JSONFieldMissing(key)))
		fmt.Fprintf(&decls, "    bool %s;\n", foundVar)
		fmt.Fprintf(&decls, "    rech_jv %s = rech_json_obj_get(v, \"%s\", &%s);\n", fvVar, escapeCString(key), foundVar)
		fmt.Fprintf(&decls, "    if (!%s) { %s }\n", foundVar, missing)
		fmt.Fprintf(&decls, "    %s f%d = %s(%s);\n", fieldC, i, extractor, fvVar)
		args[i] = fmt.Sprintf("f%d", i)
	}

	var construct string
	if g.ctx.entityIsRef(ent.Name) {
		// rech_new_X() у сущности-цели разбора всегда без аргументов - её
		// собственный "конструктор(...)" запрещён отдельной проверкой (см.
		// jsonCompatible/xmlCompatible, "у сущности есть собственный
		// конструктор") именно ради того, чтобы разбор мог заполнять поля
		// напрямую, а не гадать о смысле параметров пользовательского
		// конструктора - GC_MALLOC уже занулил объект, дальше просто
		// присваиваем разобранные значения полям одно за другим, тем же
		// принципом, что и у сущности БЕЗ явного конструктора вообще
		// (docs/SPEC.md, "Явные конструкторы").
		var b strings.Builder
		fmt.Fprintf(&b, "%s p = %s();\n", retC, mangleCtor(ent.Name))
		for i, f := range ent.Fields {
			fmt.Fprintf(&b, "    p->%s = %s;\n", mangleField(f.Name), args[i])
		}
		b.WriteString("    return p;\n")
		construct = b.String()
	} else {
		parts := make([]string, len(ent.Fields))
		for i, f := range ent.Fields {
			parts[i] = fmt.Sprintf(".%s = %s", mangleField(f.Name), args[i])
		}
		initList := strings.Join(parts, ", ")
		if initList == "" {
			initList = "0"
		}
		construct = fmt.Sprintf("return (%s){%s};\n", mangleType(ent.Name), initList)
	}

	return fmt.Sprintf(`static %[1]s %[2]s(rech_jv v) {
    if (v.kind != RECH_JV_OBJ) { %[3]s }
%[4]s    %[5]s}

`, retC, mangleJSONExtractor(ent.Name), mismatch, decls.String(), construct)
}

func jsonExtractorNameFor(t ir.Type) string {
	if name, ok := jsonPrimitiveExtractors[t.Kind]; ok {
		return name
	}
	return mangleJSONExtractor(t.Entity)
}

// generateIRParseExpr - "разобрать JSON <выражение> как <Тип>": разбирает
// исходную строку в rech_jv (rech_json_parse) и сразу передаёт результат
// экстрактору целевого типа - сама функция извлечения уже гарантированно
// сгенерирована (jsonRuntimeCode проходит prog.ParseTargets целиком до
// генерации тел функций, см. generateSource).
func generateIRParseExpr(ctx *irGenCtx, e *ir.ParseExpr) (string, valueType, error) {
	ctx.usesExceptions = true
	srcCode, _, err := generateIRExpr(ctx, e.Source)
	if err != nil {
		return "", typeUnknown, err
	}
	if e.Format == "XML" {
		code := fmt.Sprintf("%s(rech_xml_parse(%s))", xmlExtractorNameFor(e.To), srcCode)
		return code, fromIRType(e.To), nil
	}
	code := fmt.Sprintf("%s(rech_json_parse(%s))", jsonExtractorNameFor(e.To), srcCode)
	return code, fromIRType(e.To), nil
}

// jsonParserCode - разметка значения (rech_jv) и ручной рекурсивный
// JSON-парсер: пробел/строка (с escape-последовательностями, включая \uXXXX
// и суррогатные пары UTF-16 -> UTF-8) /число (strtod, локаль-независимо -
// рантайм нигде не вызывает setlocale)/true/false/null/массив/объект.
// rech_json_obj_get - линейный поиск ключа (объекты в этом срезе - обычно
// малы, схема известна заранее - без отдельной хеш-таблицы). Любая
// синтаксическая ошибка вызывает RECH_JSON_PARSE_ERR - единое сообщение
// без позиции (см. rtmsg.JSONParseError, package doc там же).
const jsonParserCode = `typedef enum {
    RECH_JV_NULL, RECH_JV_BOOL, RECH_JV_NUM, RECH_JV_STR, RECH_JV_ARR, RECH_JV_OBJ
} rech_jv_kind;

typedef struct rech_jv {
    rech_jv_kind kind;
    bool b;
    double num;
    char *str;
    struct rech_jv *items;
    char **keys;
    long long count;
} rech_jv;

typedef struct { const char *cur; } rech_jvp;

static rech_jv rech_json_parse_value(rech_jvp *p);
static void rech_struct_throw(const char *msg);

static void rech_jv_skip_ws(rech_jvp *p) {
    while (*p->cur == ' ' || *p->cur == '\t' || *p->cur == '\n' || *p->cur == '\r') {
        p->cur++;
    }
}

typedef struct { char *buf; long long len; long long cap; } rech_jv_sb;

static void rech_jv_sb_init(rech_jv_sb *sb) {
    sb->cap = 16;
    sb->len = 0;
    sb->buf = GC_MALLOC((size_t)sb->cap);
}

static void rech_jv_sb_putc(rech_jv_sb *sb, char c) {
    if (sb->len + 4 >= sb->cap) {
        long long ncap = sb->cap * 2;
        char *nb = GC_MALLOC((size_t)ncap);
        memcpy(nb, sb->buf, (size_t)sb->len);
        sb->buf = nb;
        sb->cap = ncap;
    }
    sb->buf[sb->len++] = c;
}

static char *rech_jv_sb_finish(rech_jv_sb *sb) {
    sb->buf[sb->len] = '\0';
    return sb->buf;
}

static void rech_jv_sb_utf8(rech_jv_sb *sb, unsigned int cp) {
    if (cp <= 0x7F) {
        rech_jv_sb_putc(sb, (char)cp);
    } else if (cp <= 0x7FF) {
        rech_jv_sb_putc(sb, (char)(0xC0 | (cp >> 6)));
        rech_jv_sb_putc(sb, (char)(0x80 | (cp & 0x3F)));
    } else if (cp <= 0xFFFF) {
        rech_jv_sb_putc(sb, (char)(0xE0 | (cp >> 12)));
        rech_jv_sb_putc(sb, (char)(0x80 | ((cp >> 6) & 0x3F)));
        rech_jv_sb_putc(sb, (char)(0x80 | (cp & 0x3F)));
    } else {
        rech_jv_sb_putc(sb, (char)(0xF0 | (cp >> 18)));
        rech_jv_sb_putc(sb, (char)(0x80 | ((cp >> 12) & 0x3F)));
        rech_jv_sb_putc(sb, (char)(0x80 | ((cp >> 6) & 0x3F)));
        rech_jv_sb_putc(sb, (char)(0x80 | (cp & 0x3F)));
    }
}

static unsigned int rech_jv_hex_digit(char c) {
    if (c >= '0' && c <= '9') return (unsigned int)(c - '0');
    if (c >= 'a' && c <= 'f') return (unsigned int)(c - 'a' + 10);
    if (c >= 'A' && c <= 'F') return (unsigned int)(c - 'A' + 10);
    rech_struct_throw(RECH_JSON_PARSE_ERR);
    return 0;
}

static unsigned int rech_jv_hex4(rech_jvp *p) {
    unsigned int v = 0;
    for (int i = 0; i < 4; i++) {
        v = v * 16 + rech_jv_hex_digit(*p->cur);
        p->cur++;
    }
    return v;
}

static char *rech_json_parse_string_raw(rech_jvp *p) {
    p->cur++;
    rech_jv_sb sb;
    rech_jv_sb_init(&sb);
    for (;;) {
        unsigned char c = (unsigned char)*p->cur;
        if (c == '\0') rech_struct_throw(RECH_JSON_PARSE_ERR);
        if (c == '"') { p->cur++; break; }
        if (c == '\\') {
            p->cur++;
            char e = *p->cur;
            switch (e) {
                case '"': rech_jv_sb_putc(&sb, '"'); p->cur++; break;
                case '\\': rech_jv_sb_putc(&sb, '\\'); p->cur++; break;
                case '/': rech_jv_sb_putc(&sb, '/'); p->cur++; break;
                case 'b': rech_jv_sb_putc(&sb, '\b'); p->cur++; break;
                case 'f': rech_jv_sb_putc(&sb, '\f'); p->cur++; break;
                case 'n': rech_jv_sb_putc(&sb, '\n'); p->cur++; break;
                case 'r': rech_jv_sb_putc(&sb, '\r'); p->cur++; break;
                case 't': rech_jv_sb_putc(&sb, '\t'); p->cur++; break;
                case 'u': {
                    p->cur++;
                    unsigned int cp = rech_jv_hex4(p);
                    if (cp >= 0xD800 && cp <= 0xDBFF && p->cur[0] == '\\' && p->cur[1] == 'u') {
                        p->cur += 2;
                        unsigned int lo = rech_jv_hex4(p);
                        if (lo < 0xDC00 || lo > 0xDFFF) rech_struct_throw(RECH_JSON_PARSE_ERR);
                        cp = 0x10000u + ((cp - 0xD800u) << 10) + (lo - 0xDC00u);
                    }
                    rech_jv_sb_utf8(&sb, cp);
                    break;
                }
                default:
                    rech_struct_throw(RECH_JSON_PARSE_ERR);
            }
            continue;
        }
        if (c < 0x20) rech_struct_throw(RECH_JSON_PARSE_ERR);
        rech_jv_sb_putc(&sb, (char)c);
        p->cur++;
    }
    return rech_jv_sb_finish(&sb);
}

static rech_jv rech_json_parse_array(rech_jvp *p) {
    p->cur++;
    rech_jv_skip_ws(p);
    rech_jv v = {0};
    v.kind = RECH_JV_ARR;
    if (*p->cur == ']') { p->cur++; return v; }
    long long cap = 4;
    v.items = GC_MALLOC(sizeof(rech_jv) * (size_t)cap);
    for (;;) {
        rech_jv item = rech_json_parse_value(p);
        if (v.count >= cap) {
            cap *= 2;
            rech_jv *ni = GC_MALLOC(sizeof(rech_jv) * (size_t)cap);
            memcpy(ni, v.items, sizeof(rech_jv) * (size_t)v.count);
            v.items = ni;
        }
        v.items[v.count++] = item;
        rech_jv_skip_ws(p);
        if (*p->cur == ',') { p->cur++; rech_jv_skip_ws(p); continue; }
        if (*p->cur == ']') { p->cur++; break; }
        rech_struct_throw(RECH_JSON_PARSE_ERR);
    }
    return v;
}

static rech_jv rech_json_parse_object(rech_jvp *p) {
    p->cur++;
    rech_jv_skip_ws(p);
    rech_jv v = {0};
    v.kind = RECH_JV_OBJ;
    if (*p->cur == '}') { p->cur++; return v; }
    long long cap = 4;
    v.items = GC_MALLOC(sizeof(rech_jv) * (size_t)cap);
    v.keys = GC_MALLOC(sizeof(char *) * (size_t)cap);
    for (;;) {
        rech_jv_skip_ws(p);
        if (*p->cur != '"') rech_struct_throw(RECH_JSON_PARSE_ERR);
        char *key = rech_json_parse_string_raw(p);
        rech_jv_skip_ws(p);
        if (*p->cur != ':') rech_struct_throw(RECH_JSON_PARSE_ERR);
        p->cur++;
        rech_jv val = rech_json_parse_value(p);
        if (v.count >= cap) {
            cap *= 2;
            rech_jv *ni = GC_MALLOC(sizeof(rech_jv) * (size_t)cap);
            memcpy(ni, v.items, sizeof(rech_jv) * (size_t)v.count);
            v.items = ni;
            char **nk = GC_MALLOC(sizeof(char *) * (size_t)cap);
            memcpy(nk, v.keys, sizeof(char *) * (size_t)v.count);
            v.keys = nk;
        }
        v.keys[v.count] = key;
        v.items[v.count] = val;
        v.count++;
        rech_jv_skip_ws(p);
        if (*p->cur == ',') { p->cur++; continue; }
        if (*p->cur == '}') { p->cur++; break; }
        rech_struct_throw(RECH_JSON_PARSE_ERR);
    }
    return v;
}

static rech_jv rech_json_parse_value(rech_jvp *p) {
    rech_jv_skip_ws(p);
    char c = *p->cur;
    if (c == '"') {
        rech_jv v = {0};
        v.kind = RECH_JV_STR;
        v.str = rech_json_parse_string_raw(p);
        return v;
    }
    if (c == '{') return rech_json_parse_object(p);
    if (c == '[') return rech_json_parse_array(p);
    if (c == 't') {
        if (strncmp(p->cur, "true", 4) != 0) rech_struct_throw(RECH_JSON_PARSE_ERR);
        p->cur += 4;
        rech_jv v = {0};
        v.kind = RECH_JV_BOOL;
        v.b = true;
        return v;
    }
    if (c == 'f') {
        if (strncmp(p->cur, "false", 5) != 0) rech_struct_throw(RECH_JSON_PARSE_ERR);
        p->cur += 5;
        rech_jv v = {0};
        v.kind = RECH_JV_BOOL;
        v.b = false;
        return v;
    }
    if (c == 'n') {
        if (strncmp(p->cur, "null", 4) != 0) rech_struct_throw(RECH_JSON_PARSE_ERR);
        p->cur += 4;
        rech_jv v = {0};
        v.kind = RECH_JV_NULL;
        return v;
    }
    if (c == '-' || (c >= '0' && c <= '9')) {
        char *end;
        double n = strtod(p->cur, &end);
        if (end == p->cur) rech_struct_throw(RECH_JSON_PARSE_ERR);
        p->cur = end;
        rech_jv v = {0};
        v.kind = RECH_JV_NUM;
        v.num = n;
        return v;
    }
    rech_struct_throw(RECH_JSON_PARSE_ERR);
    rech_jv v = {0};
    return v;
}

static rech_jv rech_json_parse(const char *text) {
    rech_jvp p;
    p.cur = text;
    rech_jv_skip_ws(&p);
    rech_jv v = rech_json_parse_value(&p);
    rech_jv_skip_ws(&p);
    if (*p.cur != '\0') rech_struct_throw(RECH_JSON_PARSE_ERR);
    return v;
}

static rech_jv rech_json_obj_get(rech_jv obj, const char *key, bool *found) {
    for (long long i = 0; i < obj.count; i++) {
        if (strcmp(obj.keys[i], key) == 0) {
            *found = true;
            return obj.items[i];
        }
    }
    *found = false;
    rech_jv v = {0};
    return v;
}

`
