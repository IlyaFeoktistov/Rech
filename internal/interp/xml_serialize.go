// "сериализовать XML <выражение>" - обратное к xml.go, см. её package doc
// для отображения Rech-типа на XML. В отличие от JSON-сериализации, всегда
// требует внешний тег: xmlSerializeRoot оборачивает верхний уровень
// (примитив/сущность - тег "значение"; Список<T> целиком - тег "значение"
// с элементами под тегом "элемент", поскольку у XML-документа ровно один
// корневой элемент, а список сам по себе не даёт для него имени).
package interp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

const (
	xmlRootTag = "значение"
	xmlItemTag = "элемент"
)

func (in *Interp) evalXMLSerializeExpr(v Value) Value {
	var sb strings.Builder
	if v.Obj != nil && v.Obj.coll != nil {
		sb.WriteByte('<')
		sb.WriteString(xmlRootTag)
		sb.WriteByte('>')
		for _, elem := range v.Obj.coll.elems {
			in.xmlSerializeTagged(&sb, elem, xmlItemTag)
		}
		sb.WriteString("</")
		sb.WriteString(xmlRootTag)
		sb.WriteByte('>')
	} else {
		in.xmlSerializeTagged(&sb, v, xmlRootTag)
	}
	return stringValue(sb.String())
}

// xmlSerializeTagged пишет v как один элемент <tag>...</tag> - v никогда
// не сама коллекция (Список<T> как поле сущности разворачивается ЗДЕСЬ, в
// цикле по полям, прямо в повторяющиеся <fieldName>...</fieldName> без
// дополнительной обёртки - см. package doc, "Список<T> как поле").
func (in *Interp) xmlSerializeTagged(sb *strings.Builder, v Value, tag string) {
	switch v.Type.Kind {
	case ir.KindInt, ir.KindFloat32, ir.KindFloat, ir.KindBool, ir.KindDecimal:
		fmt.Fprintf(sb, "<%s>%s</%s>", tag, xmlPrimitiveText(v), tag)
	case ir.KindString:
		sb.WriteByte('<')
		sb.WriteString(tag)
		sb.WriteByte('>')
		xmlWriteEscapedText(sb, v.S)
		sb.WriteString("</")
		sb.WriteString(tag)
		sb.WriteByte('>')
	case ir.KindEntity:
		ent := in.entities[v.Obj.typeName]
		sb.WriteByte('<')
		sb.WriteString(tag)
		for _, f := range ent.Fields {
			if name, attr := f.XMLName(); attr {
				sb.WriteByte(' ')
				sb.WriteString(name)
				sb.WriteString(`="`)
				xmlWriteEscapedAttr(sb, xmlPrimitiveText(v.Obj.getField(f.Name)))
				sb.WriteByte('"')
			}
		}
		sb.WriteByte('>')
		for _, f := range ent.Fields {
			name, attr := f.XMLName()
			if attr {
				continue
			}
			fv := v.Obj.getField(f.Name)
			if fv.Obj != nil && fv.Obj.coll != nil {
				for _, elem := range fv.Obj.coll.elems {
					in.xmlSerializeTagged(sb, elem, name)
				}
				continue
			}
			in.xmlSerializeTagged(sb, fv, name)
		}
		sb.WriteString("</")
		sb.WriteString(tag)
		sb.WriteByte('>')
	}
}

// xmlWriteEscapedText - те же 5 предопределённых сущностей, что и
// разбор (xml.go, parseTextRune) - должны оставаться согласованными с
// cbackend.rech_xml_write_escaped.
func xmlWriteEscapedText(sb *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '&':
			sb.WriteString("&amp;")
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		default:
			sb.WriteRune(r)
		}
	}
}

// xmlPrimitiveText - текстовое представление примитива в XML (содержимое
// элемента или значение атрибута), без экранирования.
func xmlPrimitiveText(v Value) string {
	switch v.Type.Kind {
	case ir.KindInt:
		return strconv.FormatInt(v.I, 10)
	case ir.KindFloat32, ir.KindFloat:
		return formatFloat(v)
	case ir.KindDecimal:
		return v.Dec.String()
	case ir.KindBool:
		if v.B {
			return "true"
		}
		return "false"
	default:
		return v.S
	}
}

// xmlWriteEscapedAttr - xmlWriteEscapedText плюс "\"" (значение атрибута
// всегда в двойных кавычках) - согласовано с
// cbackend.rech_xml_write_escaped_attr.
func xmlWriteEscapedAttr(sb *strings.Builder, s string) {
	for _, r := range s {
		if r == '"' {
			sb.WriteString("&quot;")
			continue
		}
		xmlWriteEscapedText(sb, string(r))
	}
}
