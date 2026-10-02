// "сериализовать JSON <выражение>" - обратное к json_extract.go: для
// каждого типа из prog.SerializeTargets (и рекурсивно для любого
// достижимого из него типа поля сущности/элемента Список<T>) генерирует
// функцию "rech_json_serialize_<mangled>(value, rech_json_sb *sb)",
// дописывающую JSON-текст value в общий растущий буфер (rech_json_sb,
// malloc/realloc - не GC_MALLOC: тот же принцип, что и у Список<T>
// (realloc), результат - обычная C-строка "Строка", как у любой другой
// builtin-функции, возвращающей Строка). Сериализация корректно
// типизированного значения не может провалиться - нет rech_throw/
// rech_panic здесь вообще, в отличие от json_extract.go.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

type jsonSerializerGen struct {
	ctx       *irGenCtx
	generics  map[string]ir.GenericInstantiation
	generated map[string]bool
	code      strings.Builder
}

// jsonSerializeRuntimeCode - см. jsonRuntimeCode (json_extract.go), тот же
// принцип для обратного направления.
func jsonSerializeRuntimeCode(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error) {
	generics := make(map[string]ir.GenericInstantiation, len(prog.Generics))
	for _, g := range prog.Generics {
		generics[g.Name] = g
	}
	g := &jsonSerializerGen{ctx: ctx, generics: generics, generated: map[string]bool{}}
	for _, t := range targets {
		if err := g.ensure(t); err != nil {
			return "", err
		}
	}
	var sb strings.Builder
	sb.WriteString(jsonSerializeBufCode)
	sb.WriteString(g.code.String())
	// jsonToStringWrapperCode - только для targets (типы, реально
	// встретившиеся как ВЕРХНИЙ уровень "сериализовать JSON ...") - вложенные
	// типы, достижимые только как поле/элемент, вызываются напрямую через
	// двухаргументную rech_json_serialize_<T>(value, sb) внутри родителя
	// (см. jsonEntitySerializerCode/jsonListSerializerCode), собственная
	// обёртка "в готовую строку" им не нужна.
	wrapped := map[string]bool{}
	for _, t := range targets {
		key := jsonTypeKey(t)
		if wrapped[key] {
			continue
		}
		wrapped[key] = true
		sb.WriteString(jsonToStringWrapperCode(ctx, t))
	}
	return sb.String(), nil
}

// jsonToStringWrapperCode - "rech_json_serialize_to_string_<mangled>(v) ->
// char *": заводит свежий rech_json_sb, вызывает уже сгенерированную
// (ensure) функцию сериализации в буфер, возвращает готовую NUL-
// терминированную C-строку.
func jsonToStringWrapperCode(ctx *irGenCtx, t ir.Type) string {
	valueC := fromIRType(t).cType(ctx)
	return fmt.Sprintf(`static char *%s(%s v) {
    rech_json_sb sb;
    rech_json_sb_init(&sb);
    %s(v, &sb);
    rech_json_sb_ensure(&sb, 0);
    sb.buf[sb.len] = '\0';
    return sb.buf;
}

`, jsonToStringWrapperName(t), valueC, jsonSerializerNameFor(t))
}

func jsonToStringWrapperName(t ir.Type) string {
	if name, ok := jsonPrimitiveSerializers[t.Kind]; ok {
		return name + "_to_string"
	}
	return "rech_json_serialize_to_string_" + transliterate(t.Entity)
}

func (g *jsonSerializerGen) ensure(t ir.Type) error {
	key := jsonTypeKey(t)
	if g.generated[key] {
		return nil
	}
	g.generated[key] = true

	if name, ok := jsonPrimitiveSerializers[t.Kind]; ok {
		g.code.WriteString(jsonPrimitiveSerializerCode(name, t))
		return nil
	}
	if t.Kind != ir.KindEntity {
		return fmt.Errorf("генерация C из IR: \"сериализовать JSON\" для неподдержанного типа %s", t)
	}
	if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
		if err := g.ensure(gi.ElemType); err != nil {
			return err
		}
		g.code.WriteString(g.jsonListSerializerCode(gi))
		return nil
	}
	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: \"сериализовать JSON\" для неизвестного типа %q", t.Entity)
	}
	for _, f := range ent.Fields {
		if err := g.ensure(f.Type); err != nil {
			return err
		}
	}
	g.code.WriteString(g.jsonEntitySerializerCode(ent))
	return nil
}

var jsonPrimitiveSerializers = map[ir.TypeKind]string{
	ir.KindInt:     "rech_json_serialize_int",
	ir.KindFloat32: "rech_json_serialize_float32",
	ir.KindFloat:   "rech_json_serialize_float64",
	ir.KindString:  "rech_json_serialize_string",
	ir.KindBool:    "rech_json_serialize_bool",
}

func jsonSerializerNameFor(t ir.Type) string {
	if name, ok := jsonPrimitiveSerializers[t.Kind]; ok {
		return name
	}
	return "rech_json_serialize_" + transliterate(t.Entity)
}

func jsonPrimitiveSerializerCode(name string, t ir.Type) string {
	switch t.Kind {
	case ir.KindInt:
		return fmt.Sprintf(`static void %s(long long v, rech_json_sb *sb) {
    char buf[32];
    snprintf(buf, sizeof buf, "%%lld", v);
    rech_json_sb_puts(sb, buf);
}

`, name)
	case ir.KindFloat32:
		return fmt.Sprintf(`static void %s(float v, rech_json_sb *sb) {
    char buf[64];
    rech_fmt_float(buf, (double)v, 1);
    rech_json_sb_puts(sb, buf);
}

`, name)
	case ir.KindFloat:
		return fmt.Sprintf(`static void %s(double v, rech_json_sb *sb) {
    char buf[64];
    rech_fmt_float(buf, v, 0);
    rech_json_sb_puts(sb, buf);
}

`, name)
	case ir.KindString:
		return fmt.Sprintf(`static void %s(const char *v, rech_json_sb *sb) {
    rech_json_sb_put_escaped(sb, v);
}

`, name)
	default: // ir.KindBool
		return fmt.Sprintf(`static void %s(bool v, rech_json_sb *sb) {
    rech_json_sb_puts(sb, v ? "true" : "false");
}

`, name)
	}
}

func (g *jsonSerializerGen) jsonListSerializerCode(gi ir.GenericInstantiation) string {
	listC := mangleType(gi.Name)
	elemSerializer := jsonSerializerNameFor(gi.ElemType)
	return fmt.Sprintf(`static void %[1]s(%[2]s *v, rech_json_sb *sb) {
    rech_json_sb_putc(sb, '[');
    for (long long i = 0; i < v->len; i++) {
        if (i > 0) {
            rech_json_sb_putc(sb, ',');
        }
        %[3]s(v->data[i], sb);
    }
    rech_json_sb_putc(sb, ']');
}

`, "rech_json_serialize_"+transliterate(gi.Name), listC, elemSerializer)
}

func (g *jsonSerializerGen) jsonEntitySerializerCode(ent *ir.Entity) string {
	valueC := fromIRType(ir.EntityType(ent.Name)).cType(g.ctx)
	access := "v."
	if g.ctx.entityIsRef(ent.Name) {
		access = "v->"
	}
	var body strings.Builder
	body.WriteString("    rech_json_sb_putc(sb, '{');\n")
	for i, f := range ent.Fields {
		if i > 0 {
			body.WriteString("    rech_json_sb_putc(sb, ',');\n")
		}
		fmt.Fprintf(&body, "    rech_json_sb_put_escaped(sb, \"%s\");\n", escapeCString(f.JSONKey()))
		body.WriteString("    rech_json_sb_putc(sb, ':');\n")
		fmt.Fprintf(&body, "    %s(%s%s, sb);\n", jsonSerializerNameFor(f.Type), access, mangleField(f.Name))
	}
	body.WriteString("    rech_json_sb_putc(sb, '}');\n")

	return fmt.Sprintf(`static void %s(%s v, rech_json_sb *sb) {
%s}

`, "rech_json_serialize_"+transliterate(ent.Name), valueC, body.String())
}

// generateIRSerializeExpr - "сериализовать JSON <выражение>": вызывает
// функцию сериализации целевого типа в свежий rech_json_sb и возвращает
// готовую C-строку - см. package doc.
func generateIRSerializeExpr(ctx *irGenCtx, e *ir.SerializeExpr) (string, valueType, error) {
	valueCode, _, err := generateIRExpr(ctx, e.Value)
	if err != nil {
		return "", typeUnknown, err
	}
	t := ir.TypeOf(e.Value)
	if e.Format == "XML" {
		code := fmt.Sprintf("%s(%s)", xmlToStringWrapperName(t), valueCode)
		return code, typeString, nil
	}
	if e.Format == "YAML" {
		return generateIRSerializeExprYAML(t, valueCode), typeString, nil
	}
	if e.Format == "PROTOBUF" {
		return generateIRSerializeExprPROTOBUF(ctx, t, valueCode)
	}
	code := fmt.Sprintf("%s(%s)", jsonToStringWrapperName(t), valueCode)
	return code, typeString, nil
}

// jsonSerializeBufCode - растущий строковый буфер (rech_json_sb, malloc/
// realloc) и rech_json_sb_put_escaped (экранирование строки в JSON-строковый
// литерал - тот же набор escape-последовательностей, что и
// internal/interp/json_serialize.go, jsonWriteEscapedString, должны
// оставаться согласованными). Обёртка "в готовую строку" на каждый целевой
// тип - jsonToStringWrapperCode (генерируется отдельно для каждого типа из
// prog.SerializeTargets, C не умеет параметризовать функцию именем другой
// функции без указателей на функции разной сигнатуры для каждого типа -
// проще сгенерировать саму обёртку, чем городить единый диспетчер).
const jsonSerializeBufCode = `typedef struct { char *buf; long long len; long long cap; } rech_json_sb;

static void rech_json_sb_init(rech_json_sb *sb) {
    sb->cap = 64;
    sb->len = 0;
    sb->buf = malloc((size_t)sb->cap);
}

static void rech_json_sb_ensure(rech_json_sb *sb, long long extra) {
    if (sb->len + extra + 1 > sb->cap) {
        while (sb->cap < sb->len + extra + 1) {
            sb->cap *= 2;
        }
        sb->buf = realloc(sb->buf, (size_t)sb->cap);
    }
}

static void rech_json_sb_putc(rech_json_sb *sb, char c) {
    rech_json_sb_ensure(sb, 1);
    sb->buf[sb->len++] = c;
}

static void rech_json_sb_puts(rech_json_sb *sb, const char *s) {
    long long n = (long long)strlen(s);
    rech_json_sb_ensure(sb, n);
    memcpy(sb->buf + sb->len, s, (size_t)n);
    sb->len += n;
}

static void rech_json_sb_put_escaped(rech_json_sb *sb, const char *s) {
    rech_json_sb_putc(sb, '"');
    for (const unsigned char *p = (const unsigned char *)s; *p; p++) {
        switch (*p) {
            case '"': rech_json_sb_puts(sb, "\\\""); break;
            case '\\': rech_json_sb_puts(sb, "\\\\"); break;
            case '\n': rech_json_sb_puts(sb, "\\n"); break;
            case '\r': rech_json_sb_puts(sb, "\\r"); break;
            case '\t': rech_json_sb_puts(sb, "\\t"); break;
            default:
                if (*p < 0x20) {
                    char esc[8];
                    snprintf(esc, sizeof esc, "\\u%04x", *p);
                    rech_json_sb_puts(sb, esc);
                } else {
                    rech_json_sb_putc(sb, (char)*p);
                }
        }
    }
    rech_json_sb_putc(sb, '"');
}

`
