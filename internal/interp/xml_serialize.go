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
	case ir.KindInt:
		fmt.Fprintf(sb, "<%s>%d</%s>", tag, v.I, tag)
	case ir.KindFloat32, ir.KindFloat:
		fmt.Fprintf(sb, "<%s>%s</%s>", tag, strconv.FormatFloat(v.F, 'g', -1, 64), tag)
	case ir.KindBool:
		b := "false"
		if v.B {
			b = "true"
		}
		fmt.Fprintf(sb, "<%s>%s</%s>", tag, b, tag)
	case ir.KindString:
		sb.WriteByte('<')
		sb.WriteString(tag)
		sb.WriteByte('>')
		xmlWriteEscapedText(sb, v.S)
		sb.WriteString("</")
		sb.WriteString(tag)
		sb.WriteByte('>')
	case ir.KindEntity:
		sb.WriteByte('<')
		sb.WriteString(tag)
		sb.WriteByte('>')
		ent := in.entities[v.Obj.typeName]
		for _, f := range ent.Fields {
			fv := v.Obj.getField(f.Name)
			if fv.Obj != nil && fv.Obj.coll != nil {
				for _, elem := range fv.Obj.coll.elems {
					in.xmlSerializeTagged(sb, elem, f.Name)
				}
				continue
			}
			in.xmlSerializeTagged(sb, fv, f.Name)
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
