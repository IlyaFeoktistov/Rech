// "разобрать YAML <выражение> как <Тип>" (internal/ir.ParseExpr) - см.
// internal/semantic/parse.go для проверки YAML-совместимости целевого
// типа (общая structCompatible, разделяемая с JSON/XML).
//
// Подмножество YAML - блочный стиль (отступами, БЕЗ фигурных/квадратных
// скобок JSON-flow-style): отображение ("ключ: значение" на своей строке,
// вложенные отображения - увеличенным отступом), последовательность ("-
// значение" на своей строке, тот же принцип отступов; элемент
// последовательности - либо скаляр на той же строке после "- ", либо
// вложенный блок с отступом ПОСЛЕ "- " (голое "-" в конце строки), либо
// "компактная" запись отображения "- ключ: значение" с продолжением на
// следующих строках тем же отступом, что и "ключ" - самый частый идиом
// YAML для списка записей), скаляры (голые/двойные кавычки/одинарные
// кавычки, число, true/false, null/~/пусто), комментарии "#" до конца
// строки. НЕ поддержано: flow-style {}/[], якоря/алиасы, несколько
// документов, блочные скаляры "|"/">", теги "!!".
//
// C-представление - "rech_yv" (тот же принцип, что "rech_jv" у JSON,
// json_extract.go - размеченное объединение kind + один активный
// вариант), построенное ручным рекурсивным парсером
// (yamlParserCode) - никакая сторонняя библиотека не подключается.
// Генерация экстракторов (rech_yaml_extract_<Тип>) - тот же принцип, что
// jsonExtractorGen (jsonExtract.go): по одной функции на каждый
// достижимый тип, между backend'ами общая семантика проверена
// structCompatible на этапе анализа.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

func mangleYAMLExtractor(name string) string {
	return "rech_yaml_extract_" + transliterate(name)
}

// yamlPrimitiveExtractors - тот же принцип, что jsonPrimitiveExtractors.
var yamlPrimitiveExtractors = map[ir.TypeKind]string{
	ir.KindInt:     "rech_yaml_extract_int",
	ir.KindFloat32: "rech_yaml_extract_float32",
	ir.KindFloat:   "rech_yaml_extract_float64",
	ir.KindString:  "rech_yaml_extract_string",
	ir.KindBool:    "rech_yaml_extract_bool",
	ir.KindDecimal: "rech_yaml_extract_decimal",
}

type yamlExtractorGen struct {
	ctx       *irGenCtx
	generics  map[string]ir.GenericInstantiation
	generated map[string]bool
	code      strings.Builder
}

// yamlRuntimeCode - тот же принцип, что jsonRuntimeCode (см. её package
// doc) - вызывается, только когда targets непуст (ctx.usesYAML,
// ir_generator.go).
func yamlRuntimeCode(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error) {
	generics := make(map[string]ir.GenericInstantiation, len(prog.Generics))
	for _, g := range prog.Generics {
		generics[g.Name] = g
	}
	g := &yamlExtractorGen{ctx: ctx, generics: generics, generated: map[string]bool{}}
	for _, t := range targets {
		if err := g.ensure(t); err != nil {
			return "", err
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "#define RECH_YAML_PARSE_ERR \"%s\"\n", escapeCString(rtmsg.YAMLParseError))
	sb.WriteString(yamlParserCode)
	sb.WriteString(g.code.String())
	return sb.String(), nil
}

func yamlTypeKey(t ir.Type) string {
	if t.Kind == ir.KindEntity {
		return "E:" + t.Entity
	}
	return fmt.Sprintf("K:%d", t.Kind)
}

func (g *yamlExtractorGen) ensure(t ir.Type) error {
	key := yamlTypeKey(t)
	if g.generated[key] {
		return nil
	}
	g.generated[key] = true

	if name, ok := yamlPrimitiveExtractors[t.Kind]; ok {
		g.code.WriteString(yamlPrimitiveExtractorCode(name, t))
		return nil
	}
	if t.Kind != ir.KindEntity {
		return fmt.Errorf("генерация C из IR: \"разобрать YAML\" в неподдержанный тип %s", t)
	}
	if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
		if err := g.ensure(gi.ElemType); err != nil {
			return err
		}
		g.code.WriteString(g.yamlListExtractorCode(gi))
		return nil
	}
	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: \"разобрать YAML\" в неизвестный тип %q", t.Entity)
	}
	for _, f := range ent.Fields {
		if err := g.ensure(f.Type); err != nil {
			return err
		}
	}
	g.code.WriteString(g.yamlEntityExtractorCode(ent))
	return nil
}

func yamlPrimitiveExtractorCode(name string, t ir.Type) string {
	mismatch := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.YAMLValueMismatch))
	switch t.Kind {
	case ir.KindInt:
		return fmt.Sprintf(`static long long %s(rech_yv v) {
    if (v.kind != RECH_YV_NUM) { %s }
    long long i = (long long)v.num;
    if ((double)i != v.num) { %s }
    return i;
}

`, name, mismatch, mismatch)
	case ir.KindFloat32:
		return fmt.Sprintf(`static float %s(rech_yv v) {
    if (v.kind != RECH_YV_NUM) { %s }
    return (float)v.num;
}

`, name, mismatch)
	case ir.KindFloat:
		return fmt.Sprintf(`static double %s(rech_yv v) {
    if (v.kind != RECH_YV_NUM) { %s }
    return v.num;
}

`, name, mismatch)
	case ir.KindDecimal:
		return fmt.Sprintf(`static rech_dec %s(rech_yv v) {
    rech_dec d;
    if ((v.kind != RECH_YV_NUM && v.kind != RECH_YV_STR) || !rech_dec_parse(v.str, &d)) { %s }
    return d;
}

`, name, mismatch)
	case ir.KindString:
		return fmt.Sprintf(`static const char *%s(rech_yv v) {
    if (v.kind != RECH_YV_STR) { %s }
    return v.str;
}

`, name, mismatch)
	default: // ir.KindBool
		return fmt.Sprintf(`static bool %s(rech_yv v) {
    if (v.kind != RECH_YV_BOOL) { %s }
    return v.b;
}

`, name, mismatch)
	}
}

func (g *yamlExtractorGen) yamlListExtractorCode(gi ir.GenericInstantiation) string {
	listC := mangleType(gi.Name)
	ctor := mangleCtor(gi.Name)
	addFn := mangleMethod(gi.Name, "добавить")
	elemExtractor := yamlExtractorNameFor(gi.ElemType)
	mismatch := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.YAMLValueMismatch))
	return fmt.Sprintf(`static %[1]s *%[2]s(rech_yv v) {
    if (v.kind != RECH_YV_SEQ) { %[3]s }
    %[1]s *out = %[6]s();
    for (long long i = 0; i < v.count; i++) {
        %[4]s(out, %[5]s(v.items[i]));
    }
    return out;
}

`, listC, mangleYAMLExtractor(gi.Name), mismatch, addFn, elemExtractor, ctor)
}

func (g *yamlExtractorGen) yamlEntityExtractorCode(ent *ir.Entity) string {
	retC := fromIRType(ir.EntityType(ent.Name)).cType(g.ctx)
	mismatch := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.YAMLValueMismatch))

	var decls strings.Builder
	args := make([]string, len(ent.Fields))
	for i, f := range ent.Fields {
		fieldC := fromIRType(f.Type).cType(g.ctx)
		extractor := yamlExtractorNameFor(f.Type)
		fvVar := fmt.Sprintf("fv%d", i)
		foundVar := fmt.Sprintf("found%d", i)
		key := f.JSONKey()
		missing := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.YAMLFieldMissing(key)))
		fmt.Fprintf(&decls, "    bool %s;\n", foundVar)
		fmt.Fprintf(&decls, "    rech_yv %s = rech_yaml_map_get(v, \"%s\", &%s);\n", fvVar, escapeCString(key), foundVar)
		fmt.Fprintf(&decls, "    if (!%s) { %s }\n", foundVar, missing)
		fmt.Fprintf(&decls, "    %s f%d = %s(%s);\n", fieldC, i, extractor, fvVar)
		args[i] = fmt.Sprintf("f%d", i)
	}

	var construct string
	if g.ctx.entityIsRef(ent.Name) {
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
		construct = fmt.Sprintf("return (%s){%s};\n", entityCName(g.ctx, ent.Name), initList)
	}

	return fmt.Sprintf(`static %[1]s %[2]s(rech_yv v) {
    if (v.kind != RECH_YV_MAP) { %[3]s }
%[4]s    %[5]s}

`, retC, mangleYAMLExtractor(ent.Name), mismatch, decls.String(), construct)
}

func yamlExtractorNameFor(t ir.Type) string {
	if name, ok := yamlPrimitiveExtractors[t.Kind]; ok {
		return name
	}
	return mangleYAMLExtractor(t.Entity)
}

// generateIRParseExprYAML - вызывается из generateIRParseExpr (json_extract.go)
// для e.Format == "YAML".
func generateIRParseExprYAML(e *ir.ParseExpr, srcCode string) (string, valueType) {
	code := fmt.Sprintf("%s(rech_yaml_parse(%s))", yamlExtractorNameFor(e.To), srcCode)
	return code, fromIRType(e.To)
}

// yamlParserCode - ручной блочный YAML-парсер (см. package doc выше).
// rech_yvp - курсор по тексту (та же идея, что rech_jvp у JSON), но
// парсинг управляется ОТСТУПАМИ строк, а не парными скобками - каждая
// "структурная" функция (rech_yaml_parse_map/rech_yaml_parse_seq)
// принимает "свой" отступ (колонку, на которой обязаны начинаться ВСЕ её
// непосредственные элементы) и останавливается, как только следующая
// содержательная строка окажется с ДРУГИМ отступом (меньше - конец
// текущего блока, тем же уровнем decides дальше сам вызывающий).
const yamlParserCode = `typedef enum {
    RECH_YV_NULL, RECH_YV_BOOL, RECH_YV_NUM, RECH_YV_STR, RECH_YV_SEQ, RECH_YV_MAP
} rech_yv_kind;

typedef struct rech_yv {
    rech_yv_kind kind;
    bool b;
    double num;
    char *str;
    struct rech_yv *items;
    char **keys;
    long long count;
} rech_yv;

typedef struct { const char *cur; } rech_yvp;

static rech_yv rech_yaml_parse_node(rech_yvp *p, long long min_indent);
static void rech_struct_throw(const char *msg);

static bool rech_yaml_is_blank_or_comment(const char *s) {
    while (*s == ' ') s++;
    return *s == '\0' || *s == '\n' || *s == '\r' || *s == '#';
}

static void rech_yaml_next_line(rech_yvp *p) {
    while (*p->cur != '\0' && *p->cur != '\n') p->cur++;
    if (*p->cur == '\n') p->cur++;
}

static void rech_yaml_skip_blank_lines(rech_yvp *p) {
    while (*p->cur != '\0' && rech_yaml_is_blank_or_comment(p->cur)) {
        rech_yaml_next_line(p);
    }
}

static long long rech_yaml_cur_indent(rech_yvp *p) {
    long long n = 0;
    const char *s = p->cur;
    while (*s == ' ') { s++; n++; }
    return n;
}

static void rech_yaml_skip_inline_spaces(rech_yvp *p) {
    while (*p->cur == ' ' || *p->cur == '\t') p->cur++;
}

static bool rech_yaml_at_eol_or_comment(rech_yvp *p) {
    return *p->cur == '\0' || *p->cur == '\n' || *p->cur == '\r' || *p->cur == '#';
}

typedef struct { char *buf; long long len; long long cap; } rech_yv_sb;

static void rech_yv_sb_init(rech_yv_sb *sb) {
    sb->cap = 16;
    sb->len = 0;
    sb->buf = GC_MALLOC((size_t)sb->cap);
}

static void rech_yv_sb_putc(rech_yv_sb *sb, char c) {
    if (sb->len + 2 >= sb->cap) {
        long long ncap = sb->cap * 2;
        char *nb = GC_MALLOC((size_t)ncap);
        memcpy(nb, sb->buf, (size_t)sb->len);
        sb->buf = nb;
        sb->cap = ncap;
    }
    sb->buf[sb->len++] = c;
}

static char *rech_yv_sb_finish(rech_yv_sb *sb) {
    sb->buf[sb->len] = '\0';
    if (!rech_text_valid((const unsigned char *)sb->buf, (size_t)sb->len)) {
        return rech_text_fixed((const unsigned char *)sb->buf, (size_t)sb->len);
    }
    return sb->buf;
}

// rech_yaml_parse_dquoted - двойные кавычки, escape-последовательности как
// у JSON (без \\uXXXX ради простоты - конфигу они не нужны, юникод внутри
// строки и так пишется буквально, UTF-8 исходник читается как есть).
static char *rech_yaml_parse_dquoted(rech_yvp *p) {
    p->cur++;
    rech_yv_sb sb;
    rech_yv_sb_init(&sb);
    for (;;) {
        char c = *p->cur;
        if (c == '\0' || c == '\n') rech_struct_throw(RECH_YAML_PARSE_ERR);
        if (c == '"') { p->cur++; break; }
        if (c == '\\') {
            p->cur++;
            char e = *p->cur;
            switch (e) {
                case '"': rech_yv_sb_putc(&sb, '"'); break;
                case '\\': rech_yv_sb_putc(&sb, '\\'); break;
                case 'n': rech_yv_sb_putc(&sb, '\n'); break;
                case 't': rech_yv_sb_putc(&sb, '\t'); break;
                case 'r': rech_yv_sb_putc(&sb, '\r'); break;
                default: rech_yv_sb_putc(&sb, e);
            }
            p->cur++;
            continue;
        }
        rech_yv_sb_putc(&sb, c);
        p->cur++;
    }
    return rech_yv_sb_finish(&sb);
}

// rech_yaml_parse_squoted - одинарные кавычки: единственный escape - '' -> '.
static char *rech_yaml_parse_squoted(rech_yvp *p) {
    p->cur++;
    rech_yv_sb sb;
    rech_yv_sb_init(&sb);
    for (;;) {
        char c = *p->cur;
        if (c == '\0' || c == '\n') rech_struct_throw(RECH_YAML_PARSE_ERR);
        if (c == '\'') {
            if (p->cur[1] == '\'') { rech_yv_sb_putc(&sb, '\''); p->cur += 2; continue; }
            p->cur++;
            break;
        }
        rech_yv_sb_putc(&sb, c);
        p->cur++;
    }
    return rech_yv_sb_finish(&sb);
}

// rech_yaml_classify_plain - голый (без кавычек) скаляр text -> rech_yv:
// null (пусто/"~"/"null"), true/false, число (strtod - вся строка целиком,
// иначе это не число) или строка как есть.
static rech_yv rech_yaml_classify_plain(const char *text, long long len) {
    rech_yv v = {0};
    if (len == 0 || (len == 1 && text[0] == '~')
        || (len == 4 && strncmp(text, "null", 4) == 0)
        || (len == 4 && strncmp(text, "Null", 4) == 0)
        || (len == 4 && strncmp(text, "NULL", 4) == 0)) {
        v.kind = RECH_YV_NULL;
        return v;
    }
    if ((len == 4 && strncmp(text, "true", 4) == 0) || (len == 4 && strncmp(text, "True", 4) == 0) || (len == 4 && strncmp(text, "TRUE", 4) == 0)) {
        v.kind = RECH_YV_BOOL;
        v.b = true;
        return v;
    }
    if ((len == 5 && strncmp(text, "false", 5) == 0) || (len == 5 && strncmp(text, "False", 5) == 0) || (len == 5 && strncmp(text, "FALSE", 5) == 0)) {
        v.kind = RECH_YV_BOOL;
        v.b = false;
        return v;
    }
    char *buf = GC_MALLOC((size_t)len + 1);
    memcpy(buf, text, (size_t)len);
    buf[len] = '\0';
    char *end;
    double n = strtod(buf, &end);
    if (end == buf + len) {
        v.kind = RECH_YV_NUM;
        v.num = n;
        v.str = buf; /* запись числа - для точного Десятичное */
        return v;
    }
    v.kind = RECH_YV_STR;
    v.str = buf;
    return v;
}

// rech_yaml_parse_scalar_line - разбирает скаляр, начиная с *p->cur (уже
// после отступа/"key: "/"- "), и продвигает курсор до начала следующей
// строки. Кавычки - строка целиком; голый скаляр - до конца строки, но
// СРЕЗАЯ хвостовой инлайн-комментарий (" #...") и завершающие пробелы.
static rech_yv rech_yaml_parse_scalar_line(rech_yvp *p) {
    rech_yaml_skip_inline_spaces(p);
    rech_yv v;
    if (*p->cur == '"') {
        v.kind = RECH_YV_STR;
        v.b = false; v.num = 0; v.items = NULL; v.keys = NULL; v.count = 0;
        v.str = rech_yaml_parse_dquoted(p);
    } else if (*p->cur == '\'') {
        v.kind = RECH_YV_STR;
        v.b = false; v.num = 0; v.items = NULL; v.keys = NULL; v.count = 0;
        v.str = rech_yaml_parse_squoted(p);
    } else if (p->cur[0] == '[' && p->cur[1] == ']') {
        // Единственный признанный flow-style токен в этом подмножестве -
        // пустая последовательность/отображение ("[]"/"{}") - блочный
        // стиль сам по себе не может выразить "далее НОЛЬ элементов"
        // никаким отступом (нет строк - нет и признака, что это пустой
        // список, а не Ничто) - сериализатор (yaml_serialize.go) всегда
        // пишет именно эти токены для пустых коллекций.
        v.kind = RECH_YV_SEQ;
        v.b = false; v.num = 0; v.str = NULL; v.items = NULL; v.keys = NULL; v.count = 0;
        p->cur += 2;
    } else if (p->cur[0] == '{' && p->cur[1] == '}') {
        v.kind = RECH_YV_MAP;
        v.b = false; v.num = 0; v.str = NULL; v.items = NULL; v.keys = NULL; v.count = 0;
        p->cur += 2;
    } else {
        const char *start = p->cur;
        const char *end = start;
        while (*end != '\0' && *end != '\n' && *end != '\r') {
            if (*end == '#' && end > start && *(end - 1) == ' ') break;
            end++;
        }
        while (end > start && (*(end - 1) == ' ' || *(end - 1) == '\t')) end--;
        v = rech_yaml_classify_plain(start, (long long)(end - start));
        p->cur = end;
    }
    rech_yaml_skip_inline_spaces(p);
    if (*p->cur == '#') { while (*p->cur != '\0' && *p->cur != '\n') p->cur++; }
    if (*p->cur != '\0' && *p->cur != '\n' && *p->cur != '\r') rech_struct_throw(RECH_YAML_PARSE_ERR);
    rech_yaml_next_line(p);
    return v;
}

// rech_yaml_parse_key - разбирает ключ отображения, начиная с *p->cur
// (сразу после отступа), включая двоеточие-разделитель - НЕ трогает
// перенос строки/значение после него. Голый ключ - до первого ": "/":"
// в конце строки; кавычки поддержаны тем же принципом, что и у скаляров.
static char *rech_yaml_parse_key(rech_yvp *p) {
    char *key;
    if (*p->cur == '"') {
        key = rech_yaml_parse_dquoted(p);
    } else if (*p->cur == '\'') {
        key = rech_yaml_parse_squoted(p);
    } else {
        const char *start = p->cur;
        const char *colon = NULL;
        const char *s = start;
        while (*s != '\0' && *s != '\n' && *s != '\r') {
            if (*s == ':' && (s[1] == ' ' || s[1] == '\0' || s[1] == '\n' || s[1] == '\r')) { colon = s; break; }
            s++;
        }
        if (!colon) rech_struct_throw(RECH_YAML_PARSE_ERR);
        const char *end = colon;
        while (end > start && (*(end - 1) == ' ' || *(end - 1) == '\t')) end--;
        key = GC_MALLOC((size_t)(end - start) + 1);
        memcpy(key, start, (size_t)(end - start));
        key[end - start] = '\0';
        p->cur = colon;
    }
    rech_yaml_skip_inline_spaces(p);
    if (*p->cur != ':') rech_struct_throw(RECH_YAML_PARSE_ERR);
    p->cur++;
    return key;
}

static void rech_yaml_map_push(rech_yv *v, long long *cap, char *key, rech_yv val) {
    if (v->count >= *cap) {
        long long ncap = *cap * 2;
        rech_yv *ni = GC_MALLOC(sizeof(rech_yv) * (size_t)ncap);
        memcpy(ni, v->items, sizeof(rech_yv) * (size_t)v->count);
        v->items = ni;
        char **nk = GC_MALLOC(sizeof(char *) * (size_t)ncap);
        memcpy(nk, v->keys, sizeof(char *) * (size_t)v->count);
        v->keys = nk;
        *cap = ncap;
    }
    v->keys[v->count] = key;
    v->items[v->count] = val;
    v->count++;
}

// rech_yaml_line_looks_like_key - true, если строка content выглядит
// началом отображения ("ключ: значение"/"ключ:" в конце строки), не
// самостоятельным скаляром - см. её единственный вызов в
// rech_yaml_parse_node (документ/вложенный блок целиком - голый скаляр,
// не отображение). Кавычки (голый ключ может быть в кавычках) сначала
// пропускаются целиком (с учётом экранирования), чтобы двоеточие ВНУТРИ
// строкового скаляра ("- часы: 10:30" как значение, не как ключ) не
// спутало проверку.
static bool rech_yaml_line_looks_like_key(const char *content) {
    const char *s = content;
    if (*s == '"') {
        s++;
        while (*s != '\0' && *s != '\n' && *s != '"') {
            if (*s == '\\' && s[1] != '\0') { s += 2; continue; }
            s++;
        }
        if (*s == '"') s++;
        while (*s == ' ' || *s == '\t') s++;
        return *s == ':' && (s[1] == ' ' || s[1] == '\0' || s[1] == '\n' || s[1] == '\r');
    }
    if (*s == '\'') {
        s++;
        while (*s != '\0' && *s != '\n') {
            if (*s == '\'' && s[1] == '\'') { s += 2; continue; }
            if (*s == '\'') { s++; break; }
            s++;
        }
        while (*s == ' ' || *s == '\t') s++;
        return *s == ':' && (s[1] == ' ' || s[1] == '\0' || s[1] == '\n' || s[1] == '\r');
    }
    while (*s != '\0' && *s != '\n' && *s != '\r') {
        if (*s == ':' && (s[1] == ' ' || s[1] == '\0' || s[1] == '\n' || s[1] == '\r')) {
            return true;
        }
        s++;
    }
    return false;
}

// rech_yaml_parse_map - "key: value" на каждой строке с отступом РОВНО
// key_col; значение без "value" на строке (голое "key:" в конце) -
// вложенный блок, разбираемый рекурсивно на следующих, более отступленных
// строках. first_inline - для "компактной" записи внутри
// последовательности ("- key: value") - самый первый ключ уже читается
// начиная С ТЕКУЩЕЙ позиции курсора (без отступа - "- " уже потреблено
// вызывающим), остальные - как обычно, строками с отступом key_col.
static rech_yv rech_yaml_parse_map(rech_yvp *p, long long key_col, bool first_inline) {
    rech_yv v = {0};
    v.kind = RECH_YV_MAP;
    long long cap = 4;
    v.items = GC_MALLOC(sizeof(rech_yv) * (size_t)cap);
    v.keys = GC_MALLOC(sizeof(char *) * (size_t)cap);
    bool first = true;
    for (;;) {
        if (!(first && first_inline)) {
            rech_yaml_skip_blank_lines(p);
            if (*p->cur == '\0') break;
            if (rech_yaml_cur_indent(p) != key_col) break;
            p->cur += key_col;
        }
        first = false;
        char *key = rech_yaml_parse_key(p);
        rech_yaml_skip_inline_spaces(p);
        rech_yv val;
        if (rech_yaml_at_eol_or_comment(p)) {
            if (*p->cur == '#') { while (*p->cur != '\0' && *p->cur != '\n') p->cur++; }
            rech_yaml_next_line(p);
            rech_yaml_skip_blank_lines(p);
            if (*p->cur == '\0' || rech_yaml_cur_indent(p) <= key_col) {
                val.kind = RECH_YV_NULL;
                val.b = false; val.num = 0; val.str = NULL; val.items = NULL; val.keys = NULL; val.count = 0;
            } else {
                long long child_indent = rech_yaml_cur_indent(p);
                val = rech_yaml_parse_node(p, child_indent);
            }
        } else {
            val = rech_yaml_parse_scalar_line(p);
        }
        rech_yaml_map_push(&v, &cap, key, val);
    }
    return v;
}

// rech_yaml_parse_seq - "- value"/"- " на каждой строке с отступом РОВНО
// seq_col - тем же принципом, что rech_yaml_parse_map выше, только маркер
// не "ключ:", а "- ".
static rech_yv rech_yaml_parse_seq(rech_yvp *p, long long seq_col) {
    rech_yv v = {0};
    v.kind = RECH_YV_SEQ;
    long long cap = 4;
    v.items = GC_MALLOC(sizeof(rech_yv) * (size_t)cap);
    for (;;) {
        rech_yaml_skip_blank_lines(p);
        if (*p->cur == '\0') break;
        if (rech_yaml_cur_indent(p) != seq_col) break;
        const char *at = p->cur + seq_col;
        if (!(at[0] == '-' && (at[1] == ' ' || at[1] == '\0' || at[1] == '\n' || at[1] == '\r'))) break;
        p->cur = at + 1;
        rech_yaml_skip_inline_spaces(p);
        long long item_col = seq_col + 2;
        rech_yv item;
        if (rech_yaml_at_eol_or_comment(p)) {
            if (*p->cur == '#') { while (*p->cur != '\0' && *p->cur != '\n') p->cur++; }
            rech_yaml_next_line(p);
            rech_yaml_skip_blank_lines(p);
            if (*p->cur == '\0' || rech_yaml_cur_indent(p) < item_col) {
                item.kind = RECH_YV_NULL;
                item.b = false; item.num = 0; item.str = NULL; item.items = NULL; item.keys = NULL; item.count = 0;
            } else {
                long long child_indent = rech_yaml_cur_indent(p);
                item = rech_yaml_parse_node(p, child_indent);
            }
        } else {
            // Компактная запись отображения "- ключ: значение" -
            // распознаётся тем же способом, что и обычный ключ (голое имя
            // до ": "/": конец строки"), не строкой целиком.
            const char *s = p->cur;
            const char *colon = NULL;
            while (*s != '\0' && *s != '\n' && *s != '\r') {
                if (*s == ':' && (s[1] == ' ' || s[1] == '\0' || s[1] == '\n' || s[1] == '\r')) { colon = s; break; }
                s++;
            }
            if (colon != NULL && *p->cur != '"' && *p->cur != '\'') {
                item = rech_yaml_parse_map(p, item_col, true);
            } else {
                item = rech_yaml_parse_scalar_line(p);
            }
        }
        if (v.count >= cap) {
            cap *= 2;
            rech_yv *ni = GC_MALLOC(sizeof(rech_yv) * (size_t)cap);
            memcpy(ni, v.items, sizeof(rech_yv) * (size_t)v.count);
            v.items = ni;
        }
        v.items[v.count++] = item;
    }
    return v;
}

static rech_yv rech_yaml_parse_node(rech_yvp *p, long long min_indent) {
    rech_yaml_skip_blank_lines(p);
    rech_yv empty = {0};
    empty.kind = RECH_YV_NULL;
    if (*p->cur == '\0') return empty;
    long long indent = rech_yaml_cur_indent(p);
    if (indent < min_indent) return empty;
    const char *content = p->cur + indent;
    if (content[0] == '[' && content[1] == ']') {
        p->cur = content + 2;
        rech_yaml_skip_inline_spaces(p);
        if (*p->cur == '#') { while (*p->cur != '\0' && *p->cur != '\n') p->cur++; }
        rech_yaml_next_line(p);
        rech_yv v = {0};
        v.kind = RECH_YV_SEQ;
        return v;
    }
    if (content[0] == '{' && content[1] == '}') {
        p->cur = content + 2;
        rech_yaml_skip_inline_spaces(p);
        if (*p->cur == '#') { while (*p->cur != '\0' && *p->cur != '\n') p->cur++; }
        rech_yaml_next_line(p);
        rech_yv v = {0};
        v.kind = RECH_YV_MAP;
        return v;
    }
    if (content[0] == '-' && (content[1] == ' ' || content[1] == '\0' || content[1] == '\n' || content[1] == '\r')) {
        return rech_yaml_parse_seq(p, indent);
    }
    // Голый скаляр как ЦЕЛЫЙ документ (например, "разобрать YAML \"42\" как
    // Целое") - строка не выглядит ключом отображения вовсе, значит это не
    // "ключ: значение", а сам скаляр целиком.
    if (!rech_yaml_line_looks_like_key(content)) {
        p->cur += indent;
        return rech_yaml_parse_scalar_line(p);
    }
    return rech_yaml_parse_map(p, indent, false);
}

static rech_yv rech_yaml_parse(const char *text) {
    rech_yvp p;
    p.cur = text;
    rech_yaml_skip_blank_lines(&p);
    if (strncmp(p.cur, "---", 3) == 0 && (p.cur[3] == '\0' || p.cur[3] == '\n' || p.cur[3] == '\r' || p.cur[3] == ' ')) {
        rech_yaml_next_line(&p);
    }
    rech_yv v = rech_yaml_parse_node(&p, 0);
    rech_yaml_skip_blank_lines(&p);
    if (strncmp(p.cur, "...", 3) == 0) {
        rech_yaml_next_line(&p);
        rech_yaml_skip_blank_lines(&p);
    }
    if (*p.cur != '\0') rech_struct_throw(RECH_YAML_PARSE_ERR);
    return v;
}

static rech_yv rech_yaml_map_get(rech_yv obj, const char *key, bool *found) {
    for (long long i = 0; i < obj.count; i++) {
        if (strcmp(obj.keys[i], key) == 0) {
            *found = true;
            return obj.items[i];
        }
    }
    *found = false;
    rech_yv v = {0};
    return v;
}

`
