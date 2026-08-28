// "сериализовать XML <выражение>" - симметрично json_serialize.go, но для
// XML: см. package doc internal/interp/xml_serialize.go для отображения на
// XML. Каждая функция сериализации принимает ДОПОЛНИТЕЛЬНЫЙ параметр tag
// (имя элемента-обёртки) - в отличие от JSON, XML-элемент обязан иметь имя
// тега, а откуда оно берётся зависит от контекста (имя поля у сущности,
// фиксированное "элемент" для элементов списка, "значение" для корня) -
// сама функция сериализации типа T не решает, каким тегом её обернут,
// решает вызывающий код (родительская сущность/список, либо
// xmlToStringWrapperCode для верхнего уровня).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

const (
	xmlRootTag = "значение"
	xmlItemTag = "элемент"
)

type xmlSerializerGen struct {
	ctx       *irGenCtx
	generics  map[string]ir.GenericInstantiation
	generated map[string]bool
	code      strings.Builder
}

func xmlSerializeRuntimeCode(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error) {
	generics := make(map[string]ir.GenericInstantiation, len(prog.Generics))
	for _, g := range prog.Generics {
		generics[g.Name] = g
	}
	g := &xmlSerializerGen{ctx: ctx, generics: generics, generated: map[string]bool{}}
	for _, t := range targets {
		if err := g.ensure(t); err != nil {
			return "", err
		}
	}
	var sb strings.Builder
	sb.WriteString(xmlWriteEscapedHelper)
	sb.WriteString(g.code.String())
	wrapped := map[string]bool{}
	for _, t := range targets {
		key := jsonTypeKey(t)
		if wrapped[key] {
			continue
		}
		wrapped[key] = true
		sb.WriteString(xmlToStringWrapperCode(ctx, g.generics, t))
	}
	return sb.String(), nil
}

// xmlToStringWrapperCode - "rech_xml_serialize_to_string_<mangled>(v) ->
// char *". Список<T> как ЦЕЛЬ РАЗБОРА/СЕРИАЛИЗАЦИИ целиком нуждается в
// собственной обёртке-корне (XML-документ обязан иметь ровно один корневой
// элемент, список сам по себе не даёт для него тега) - каждый элемент
// списка сериализуется под тегом xmlItemTag, всё целиком - под xmlRootTag;
// примитив/сущность просто оборачиваются в xmlRootTag напрямую.
func xmlToStringWrapperCode(ctx *irGenCtx, generics map[string]ir.GenericInstantiation, t ir.Type) string {
	valueC := fromIRType(t).cType(ctx)
	if gi, ok := generics[t.Entity]; t.Kind == ir.KindEntity && ok && gi.Template == "Список" {
		elemSerializer := xmlSerializerNameFor(gi.ElemType)
		return fmt.Sprintf(`static char *%[1]s(%[2]s v) {
    rech_xml_sb sb;
    rech_xml_sb_init(&sb);
    rech_xml_sb_putc(&sb, '<');
    rech_xml_sb_puts(&sb, "%[3]s");
    rech_xml_sb_putc(&sb, '>');
    for (long long i = 0; i < v.len; i++) {
        %[4]s(v.data[i], "%[5]s", &sb);
    }
    rech_xml_sb_puts(&sb, "</");
    rech_xml_sb_puts(&sb, "%[3]s");
    rech_xml_sb_putc(&sb, '>');
    return rech_xml_sb_finish(&sb);
}

`, xmlToStringWrapperName(t), valueC, xmlRootTag, elemSerializer, xmlItemTag)
	}
	return fmt.Sprintf(`static char *%[1]s(%[2]s v) {
    rech_xml_sb sb;
    rech_xml_sb_init(&sb);
    %[3]s(v, "%[4]s", &sb);
    return rech_xml_sb_finish(&sb);
}

`, xmlToStringWrapperName(t), valueC, xmlSerializerNameFor(t), xmlRootTag)
}

func xmlToStringWrapperName(t ir.Type) string {
	if name, ok := xmlPrimitiveSerializers[t.Kind]; ok {
		return name + "_to_string"
	}
	return "rech_xml_serialize_to_string_" + transliterate(t.Entity)
}

func (g *xmlSerializerGen) ensure(t ir.Type) error {
	key := jsonTypeKey(t)
	if g.generated[key] {
		return nil
	}
	g.generated[key] = true

	if name, ok := xmlPrimitiveSerializers[t.Kind]; ok {
		g.code.WriteString(xmlPrimitiveSerializerCode(name, t))
		return nil
	}
	if t.Kind != ir.KindEntity {
		return fmt.Errorf("генерация C из IR: \"сериализовать XML\" для неподдержанного типа %s", t)
	}
	if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
		if err := g.ensure(gi.ElemType); err != nil {
			return err
		}
		g.code.WriteString(g.xmlListSerializerCode(gi))
		return nil
	}
	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: \"сериализовать XML\" для неизвестного типа %q", t.Entity)
	}
	for _, f := range ent.Fields {
		if err := g.ensure(f.Type); err != nil {
			return err
		}
	}
	g.code.WriteString(g.xmlEntitySerializerCode(ent))
	return nil
}

var xmlPrimitiveSerializers = map[ir.TypeKind]string{
	ir.KindInt:     "rech_xml_serialize_int",
	ir.KindFloat32: "rech_xml_serialize_float32",
	ir.KindFloat:   "rech_xml_serialize_float64",
	ir.KindString:  "rech_xml_serialize_string",
	ir.KindBool:    "rech_xml_serialize_bool",
}

func xmlSerializerNameFor(t ir.Type) string {
	if name, ok := xmlPrimitiveSerializers[t.Kind]; ok {
		return name
	}
	return "rech_xml_serialize_" + transliterate(t.Entity)
}

func xmlPrimitiveSerializerCode(name string, t ir.Type) string {
	open := `    rech_xml_sb_putc(sb, '<');
    rech_xml_sb_puts(sb, tag);
    rech_xml_sb_putc(sb, '>');
`
	close := `    rech_xml_sb_puts(sb, "</");
    rech_xml_sb_puts(sb, tag);
    rech_xml_sb_putc(sb, '>');
`
	switch t.Kind {
	case ir.KindInt:
		return fmt.Sprintf(`static void %s(long long v, const char *tag, rech_xml_sb *sb) {
%s    char buf[32];
    snprintf(buf, sizeof buf, "%%lld", v);
    rech_xml_sb_puts(sb, buf);
%s}

`, name, open, close)
	case ir.KindFloat32:
		return fmt.Sprintf(`static void %s(float v, const char *tag, rech_xml_sb *sb) {
%s    char buf[64];
    snprintf(buf, sizeof buf, "%%g", (double)v);
    rech_xml_sb_puts(sb, buf);
%s}

`, name, open, close)
	case ir.KindFloat:
		return fmt.Sprintf(`static void %s(double v, const char *tag, rech_xml_sb *sb) {
%s    char buf[64];
    snprintf(buf, sizeof buf, "%%g", v);
    rech_xml_sb_puts(sb, buf);
%s}

`, name, open, close)
	case ir.KindString:
		return fmt.Sprintf(`static void %s(const char *v, const char *tag, rech_xml_sb *sb) {
%s    rech_xml_write_escaped(sb, v);
%s}

`, name, open, close)
	default: // ir.KindBool
		return fmt.Sprintf(`static void %s(bool v, const char *tag, rech_xml_sb *sb) {
%s    rech_xml_sb_puts(sb, v ? "true" : "false");
%s}

`, name, open, close)
	}
}

// xmlListSerializerCode - Список<T> как ПОЛЕ сущности сериализуется без
// этой функции вообще (родитель повторяет элементы напрямую под именем
// поля - см. xmlEntitySerializerCode); эта функция нужна только когда
// Список<T> сам оказался полем-элементом ДРУГОГО списка (Список<Список<T>>)
// - редкий, но не запрещённый случай, оборачивает элементы в xmlItemTag.
func (g *xmlSerializerGen) xmlListSerializerCode(gi ir.GenericInstantiation) string {
	listC := mangleType(gi.Name)
	elemSerializer := xmlSerializerNameFor(gi.ElemType)
	return fmt.Sprintf(`static void %[1]s(%[2]s v, const char *tag, rech_xml_sb *sb) {
    rech_xml_sb_putc(sb, '<');
    rech_xml_sb_puts(sb, tag);
    rech_xml_sb_putc(sb, '>');
    for (long long i = 0; i < v.len; i++) {
        %[3]s(v.data[i], "%[4]s", sb);
    }
    rech_xml_sb_puts(sb, "</");
    rech_xml_sb_puts(sb, tag);
    rech_xml_sb_putc(sb, '>');
}

`, "rech_xml_serialize_"+transliterate(gi.Name), listC, elemSerializer, xmlItemTag)
}

func (g *xmlSerializerGen) xmlEntitySerializerCode(ent *ir.Entity) string {
	valueC := fromIRType(ir.EntityType(ent.Name)).cType(g.ctx)
	access := "v."
	if g.ctx.entityIsRef(ent.Name) {
		access = "v->"
	}
	var body strings.Builder
	body.WriteString("    rech_xml_sb_putc(sb, '<');\n    rech_xml_sb_puts(sb, tag);\n    rech_xml_sb_putc(sb, '>');\n")
	for _, f := range ent.Fields {
		fieldTag := escapeCString(f.Name)
		if gi, ok := g.generics[f.Type.Entity]; f.Type.Kind == ir.KindEntity && ok && gi.Template == "Список" {
			elemSerializer := xmlSerializerNameFor(gi.ElemType)
			fmt.Fprintf(&body, "    for (long long i = 0; i < %s%s.len; i++) {\n", access, mangleField(f.Name))
			fmt.Fprintf(&body, "        %s(%s%s.data[i], \"%s\", sb);\n", elemSerializer, access, mangleField(f.Name), fieldTag)
			body.WriteString("    }\n")
			continue
		}
		fmt.Fprintf(&body, "    %s(%s%s, \"%s\", sb);\n", xmlSerializerNameFor(f.Type), access, mangleField(f.Name), fieldTag)
	}
	body.WriteString("    rech_xml_sb_puts(sb, \"</\");\n    rech_xml_sb_puts(sb, tag);\n    rech_xml_sb_putc(sb, '>');\n")

	return fmt.Sprintf(`static void %s(%s v, const char *tag, rech_xml_sb *sb) {
%s}

`, "rech_xml_serialize_"+transliterate(ent.Name), valueC, body.String())
}

// generateIRSerializeExpr (json_serialize.go) диспетчеризует сюда через
// xmlToStringWrapperName при e.Format == "XML".

// xmlWriteEscapedHelper - rech_xml_write_escaped, разделяемый всеми
// примитивными сериализаторами строки - экранирует "&"/"<"/">" (тот же
// набор, что internal/interp/xml_serialize.go, xmlWriteEscapedText;
// достаточно этих трёх - "\""/"'" нужны только внутри значений атрибутов,
// которых у сериализованного текста нет вообще, атрибуты не
// поддерживаются, см. package doc xml_extract.go).
const xmlWriteEscapedHelper = `static void rech_xml_write_escaped(rech_xml_sb *sb, const char *s) {
    for (const unsigned char *p = (const unsigned char *)s; *p; p++) {
        switch (*p) {
            case '&': rech_xml_sb_puts(sb, "&amp;"); break;
            case '<': rech_xml_sb_puts(sb, "&lt;"); break;
            case '>': rech_xml_sb_puts(sb, "&gt;"); break;
            default: rech_xml_sb_putc(sb, (char)*p);
        }
    }
}

`
