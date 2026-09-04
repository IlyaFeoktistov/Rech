// "сериализовать YAML <выражение>" - обратное к yaml_extract.go: для
// каждого типа из prog.YAMLSerializeTargets (и рекурсивно для любого
// достижимого из него) генерирует функцию "rech_yaml_serialize_<mangled>
// (value, rech_yaml_sb *sb, long long indent)" - ЕДИНАЯ сигнатура для
// сущности/структуры и Список<T> (в отличие от JSON, где отступ не нужен
// вообще): каждая пишет свои строки, начиная с колонки indent, и сама
// решает "пусто ли" (Список<T> длины 0 - "indent-пробелов[]\n", иначе -
// "- элемент" на каждой строке; те же токены "[]"/"{}", что распознаёт
// парсер, см. её package doc в yaml_extract.go, - блочный стиль сам по
// себе не может выразить "далее ноль элементов" никаким отступом). Поле-
// значение уровня "сущности" (не примитив) получает перевод строки СРАЗУ
// после "ключ:", дальше рекурсивный вызов при indent+2 пишет содержимое
// уже с новой строки - тем же способом, каким это принято писать от руки.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

type yamlSerializerGen struct {
	ctx       *irGenCtx
	generics  map[string]ir.GenericInstantiation
	generated map[string]bool
	code      strings.Builder
}

// yamlSerializeRuntimeCode - см. yamlRuntimeCode (yaml_extract.go), тот же
// принцип для обратного направления.
func yamlSerializeRuntimeCode(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error) {
	generics := make(map[string]ir.GenericInstantiation, len(prog.Generics))
	for _, g := range prog.Generics {
		generics[g.Name] = g
	}
	g := &yamlSerializerGen{ctx: ctx, generics: generics, generated: map[string]bool{}}
	for _, t := range targets {
		if err := g.ensure(t); err != nil {
			return "", err
		}
	}
	var sb strings.Builder
	sb.WriteString(yamlSerializeBufCode)
	sb.WriteString(g.code.String())
	wrapped := map[string]bool{}
	for _, t := range targets {
		key := yamlTypeKey(t)
		if wrapped[key] {
			continue
		}
		wrapped[key] = true
		sb.WriteString(yamlToStringWrapperCode(ctx, t))
	}
	return sb.String(), nil
}

func yamlToStringWrapperName(t ir.Type) string {
	if name, ok := yamlPrimitiveSerializers[t.Kind]; ok {
		return name + "_to_string"
	}
	return "rech_yaml_serialize_to_string_" + transliterate(t.Entity)
}

// yamlToStringWrapperCode - "rech_yaml_serialize_to_string_<T>(v) -> char *",
// начинает документ с indent == 0. Для примитива-цели (редкий, но
// допустимый случай - "сериализовать YAML 5") - голый скаляр без отступа,
// своя отдельная обёртка, не через rech_yaml_serialize_<T>(v, sb, indent)
// (у примитивов этой функции вообще нет, только сама rech_yaml_serialize_
// <примитив>(v, sb) без indent - см. ensure).
func yamlToStringWrapperCode(ctx *irGenCtx, t ir.Type) string {
	valueC := fromIRType(t).cType(ctx)
	if name, ok := yamlPrimitiveSerializers[t.Kind]; ok {
		return fmt.Sprintf(`static char *%s_to_string(%s v) {
    rech_yaml_sb sb;
    rech_yaml_sb_init(&sb);
    %s(v, &sb);
    rech_yaml_sb_ensure(&sb, 0);
    sb.buf[sb.len] = '\0';
    return sb.buf;
}

`, name, valueC, name)
	}
	return fmt.Sprintf(`static char *%s(%s v) {
    rech_yaml_sb sb;
    rech_yaml_sb_init(&sb);
    %s(v, &sb, 0);
    rech_yaml_sb_ensure(&sb, 0);
    sb.buf[sb.len] = '\0';
    return sb.buf;
}

`, yamlToStringWrapperName(t), valueC, yamlSerializerNameFor(t))
}

func (g *yamlSerializerGen) ensure(t ir.Type) error {
	key := yamlTypeKey(t)
	if g.generated[key] {
		return nil
	}
	g.generated[key] = true

	if name, ok := yamlPrimitiveSerializers[t.Kind]; ok {
		g.code.WriteString(yamlPrimitiveSerializerCode(name, t))
		return nil
	}
	if t.Kind != ir.KindEntity {
		return fmt.Errorf("генерация C из IR: \"сериализовать YAML\" для неподдержанного типа %s", t)
	}
	if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
		if err := g.ensure(gi.ElemType); err != nil {
			return err
		}
		g.code.WriteString(g.yamlListSerializerCode(gi))
		return nil
	}
	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: \"сериализовать YAML\" для неизвестного типа %q", t.Entity)
	}
	for _, f := range ent.Fields {
		if err := g.ensure(f.Type); err != nil {
			return err
		}
	}
	g.code.WriteString(g.yamlEntitySerializerCode(ent))
	return nil
}

var yamlPrimitiveSerializers = map[ir.TypeKind]string{
	ir.KindInt:     "rech_yaml_serialize_int",
	ir.KindFloat32: "rech_yaml_serialize_float32",
	ir.KindFloat:   "rech_yaml_serialize_float64",
	ir.KindString:  "rech_yaml_serialize_string",
	ir.KindBool:    "rech_yaml_serialize_bool",
}

func yamlSerializerNameFor(t ir.Type) string {
	if name, ok := yamlPrimitiveSerializers[t.Kind]; ok {
		return name
	}
	return "rech_yaml_serialize_" + transliterate(t.Entity)
}

// isYAMLPrimitive - t сериализуется ИНЛАЙН (та же строка, что и "ключ:"/
// "-"), не отдельным блоком с собственным отступом - см. package doc.
func isYAMLPrimitive(t ir.Type) bool {
	_, ok := yamlPrimitiveSerializers[t.Kind]
	return ok
}

func yamlPrimitiveSerializerCode(name string, t ir.Type) string {
	switch t.Kind {
	case ir.KindInt:
		return fmt.Sprintf(`static void %s(long long v, rech_yaml_sb *sb) {
    char buf[32];
    snprintf(buf, sizeof buf, "%%lld", v);
    rech_yaml_sb_puts(sb, buf);
}

`, name)
	case ir.KindFloat32:
		return fmt.Sprintf(`static void %s(float v, rech_yaml_sb *sb) {
    char buf[64];
    snprintf(buf, sizeof buf, "%%g", (double)v);
    rech_yaml_sb_puts(sb, buf);
}

`, name)
	case ir.KindFloat:
		return fmt.Sprintf(`static void %s(double v, rech_yaml_sb *sb) {
    char buf[64];
    snprintf(buf, sizeof buf, "%%g", v);
    rech_yaml_sb_puts(sb, buf);
}

`, name)
	case ir.KindString:
		// Строки ВСЕГДА в двойных кавычках, без исключений для "и так
		// безопасных" голых скаляров - простое, надёжное правило вместо
		// анализа "похоже ли на число/true/false/пусто/содержит ли ': '"
		// (именно из-за такой неоднозначности голые скаляры и нужен
		// отдельный анализ у парсера, classify_plain) - симметрично сразу
		// читается назад (rech_yaml_parse_dquoted).
		return fmt.Sprintf(`static void %s(const char *v, rech_yaml_sb *sb) {
    rech_yaml_sb_put_dquoted(sb, v);
}

`, name)
	default: // ir.KindBool
		return fmt.Sprintf(`static void %s(bool v, rech_yaml_sb *sb) {
    rech_yaml_sb_puts(sb, v ? "true" : "false");
}

`, name)
	}
}

func (g *yamlSerializerGen) yamlListSerializerCode(gi ir.GenericInstantiation) string {
	listC := mangleType(gi.Name)
	name := yamlSerializerNameFor(ir.EntityType(gi.Name))
	if isYAMLPrimitive(gi.ElemType) {
		elemSerializer := yamlSerializerNameFor(gi.ElemType)
		return fmt.Sprintf(`static void %[1]s(%[2]s *v, rech_yaml_sb *sb, long long indent) {
    if (v->len == 0) {
        rech_yaml_sb_indent(sb, indent);
        rech_yaml_sb_puts(sb, "[]\n");
        return;
    }
    for (long long i = 0; i < v->len; i++) {
        rech_yaml_sb_indent(sb, indent);
        rech_yaml_sb_puts(sb, "- ");
        %[3]s(v->data[i], sb);
        rech_yaml_sb_putc(sb, '\n');
    }
}

`, name, listC, elemSerializer)
	}
	elemSerializer := yamlSerializerNameFor(gi.ElemType)
	return fmt.Sprintf(`static void %[1]s(%[2]s *v, rech_yaml_sb *sb, long long indent) {
    if (v->len == 0) {
        rech_yaml_sb_indent(sb, indent);
        rech_yaml_sb_puts(sb, "[]\n");
        return;
    }
    for (long long i = 0; i < v->len; i++) {
        rech_yaml_sb_indent(sb, indent);
        rech_yaml_sb_puts(sb, "-\n");
        %[3]s(v->data[i], sb, indent + 2);
    }
}

`, name, listC, elemSerializer)
}

func (g *yamlSerializerGen) yamlEntitySerializerCode(ent *ir.Entity) string {
	valueC := fromIRType(ir.EntityType(ent.Name)).cType(g.ctx)
	access := "v."
	if g.ctx.entityIsRef(ent.Name) {
		access = "v->"
	}
	var body strings.Builder
	for _, f := range ent.Fields {
		fmt.Fprintf(&body, "    rech_yaml_sb_indent(sb, indent);\n")
		fmt.Fprintf(&body, "    rech_yaml_sb_puts(sb, \"%s:\");\n", escapeCString(f.JSONKey()))
		if isYAMLPrimitive(f.Type) {
			body.WriteString("    rech_yaml_sb_putc(sb, ' ');\n")
			fmt.Fprintf(&body, "    %s(%s%s, sb);\n", yamlSerializerNameFor(f.Type), access, mangleField(f.Name))
			body.WriteString("    rech_yaml_sb_putc(sb, '\\n');\n")
		} else {
			body.WriteString("    rech_yaml_sb_putc(sb, '\\n');\n")
			fmt.Fprintf(&body, "    %s(%s%s, sb, indent + 2);\n", yamlSerializerNameFor(f.Type), access, mangleField(f.Name))
		}
	}

	return fmt.Sprintf(`static void %s(%s v, rech_yaml_sb *sb, long long indent) {
%s}

`, yamlSerializerNameFor(ir.EntityType(ent.Name)), valueC, body.String())
}

// generateIRSerializeExprYAML - вызывается из generateIRSerializeExpr
// (json_serialize.go) для e.Format == "YAML".
func generateIRSerializeExprYAML(t ir.Type, valueCode string) string {
	return fmt.Sprintf("%s(%s)", yamlToStringWrapperName(t), valueCode)
}

// yamlSerializeBufCode - растущий строковый буфер (та же идея, что
// rech_json_sb/rech_xml_sb - malloc/realloc, не GC_MALLOC) и
// rech_yaml_sb_put_dquoted (экранирование строки в двойные кавычки YAML -
// набор escape-последовательностей ДОЛЖЕН оставаться согласованным с
// rech_yaml_parse_dquoted, yaml_extract.go, иначе сериализованный текст не
// разобрался бы обратно тем же самым парсером). rech_yaml_sb_indent -
// indent пробелов подряд.
const yamlSerializeBufCode = `typedef struct { char *buf; long long len; long long cap; } rech_yaml_sb;

static void rech_yaml_sb_init(rech_yaml_sb *sb) {
    sb->cap = 64;
    sb->len = 0;
    sb->buf = malloc((size_t)sb->cap);
}

static void rech_yaml_sb_ensure(rech_yaml_sb *sb, long long extra) {
    if (sb->len + extra + 1 > sb->cap) {
        while (sb->cap < sb->len + extra + 1) {
            sb->cap *= 2;
        }
        sb->buf = realloc(sb->buf, (size_t)sb->cap);
    }
}

static void rech_yaml_sb_putc(rech_yaml_sb *sb, char c) {
    rech_yaml_sb_ensure(sb, 1);
    sb->buf[sb->len++] = c;
}

static void rech_yaml_sb_puts(rech_yaml_sb *sb, const char *s) {
    long long n = (long long)strlen(s);
    rech_yaml_sb_ensure(sb, n);
    memcpy(sb->buf + sb->len, s, (size_t)n);
    sb->len += n;
}

static void rech_yaml_sb_indent(rech_yaml_sb *sb, long long n) {
    for (long long i = 0; i < n; i++) {
        rech_yaml_sb_putc(sb, ' ');
    }
}

static void rech_yaml_sb_put_dquoted(rech_yaml_sb *sb, const char *s) {
    rech_yaml_sb_putc(sb, '"');
    for (const unsigned char *p = (const unsigned char *)s; *p; p++) {
        switch (*p) {
            case '"': rech_yaml_sb_puts(sb, "\\\""); break;
            case '\\': rech_yaml_sb_puts(sb, "\\\\"); break;
            case '\n': rech_yaml_sb_puts(sb, "\\n"); break;
            case '\r': rech_yaml_sb_puts(sb, "\\r"); break;
            case '\t': rech_yaml_sb_puts(sb, "\\t"); break;
            default:
                rech_yaml_sb_putc(sb, (char)*p);
        }
    }
    rech_yaml_sb_putc(sb, '"');
}

`
