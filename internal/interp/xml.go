// "разобрать XML <выражение> как <Тип>" (internal/ir.ParseExpr, Format ==
// "XML") - собственный рекурсивный XML-парсер (без пролога/комментариев -
// известные ограничения этого среза, см. package doc
// internal/backend/cbackend/xml_extract.go) в универсальное дерево
// (xmlNode), затем структурное извлечение по целевому ir.Type (xmlExtract) -
// тот же принцип, что и json.go, но для формата, у которого нет
// готового "any"-разбора в стандартной библиотеке Go.
//
// Отображение Rech-типа на XML (симметрично cbackend/xml_extract.go и
// xml_serialize.go, оба backend'а должны сериализовать/разбирать
// одинаково):
//   - Примитив - текстовое содержимое элемента.
//   - Сущность/структура - дочерние элементы, найденные по имени поля
//     (без учёта регистра НЕ ведётся - точное совпадение, как и в JSON);
//     тег "xml:имя" переименовывает элемент, "xml:@имя"/"xml:@" делает
//     поле-примитив атрибутом элемента сущности (ir.Field.XMLName).
//   - Список<T> как ПОЛЕ сущности - все дочерние элементы с именем поля
//     (без обёртки), каждый - один элемент списка.
//   - Список<T> как ЦЕЛЬ РАЗБОРА целиком (не поле) - ВСЕ дочерние элементы
//     корневого узла, имя тега не проверяется (сериализатор оборачивает их
//     в "элемент", но парсер это не требует - см. xmlSerialize.go).
//   - Имя корневого тега и имена тегов элементов списка не проверяются -
//     значимо только вложение и текст.
package interp

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// xmlNode - разобранный XML-элемент: text - конкатенированное и
// обрезанное по краям текстовое содержимое непосредственно внутри (не
// внутри дочерних элементов), children - дочерние элементы в порядке
// документа (имя сохраняется рядом с узлом, а не как ключ map - порядок
// между РАЗНЫМИ именами тегов важен для "Список<T> целиком", см. package
// doc).
type xmlNode struct {
	text     string
	children []xmlChild
	attrs    map[string]string
}

type xmlChild struct {
	name string
	node xmlNode
}

func (n xmlNode) childrenNamed(name string) []xmlNode {
	var out []xmlNode
	for _, c := range n.children {
		if c.name == name {
			out = append(out, c.node)
		}
	}
	return out
}

type xmlRawParser struct {
	s   string
	pos int
}

func (in *Interp) xmlParseFail(msg string) error {
	return newSystemException(parseErrorEntity, msg)
}

func (in *Interp) evalXMLParseExpr(e *ir.ParseExpr, src Value) (Value, error) {
	p := &xmlRawParser{s: src.S}
	p.skipWS()
	_, root, err := p.parseElement()
	if err != nil {
		return Value{}, in.xmlParseFail(rtmsg.XMLParseError)
	}
	p.skipWS()
	if p.pos != len(p.s) {
		return Value{}, in.xmlParseFail(rtmsg.XMLParseError)
	}
	return in.xmlExtract(root, e.To)
}

func (in *Interp) xmlExtract(node xmlNode, target ir.Type) (Value, error) {
	mismatch := func() error { return in.xmlParseFail(rtmsg.XMLValueMismatch) }
	switch target.Kind {
	case ir.KindInt:
		n, err := strconv.ParseInt(node.text, 10, 64)
		if err != nil {
			return Value{}, mismatch()
		}
		return intValue(n), nil
	case ir.KindFloat32:
		f, err := strconv.ParseFloat(node.text, 64)
		if err != nil {
			return Value{}, mismatch()
		}
		return Value{Type: ir.TypeFloat32, F: f}, nil
	case ir.KindFloat:
		f, err := strconv.ParseFloat(node.text, 64)
		if err != nil {
			return Value{}, mismatch()
		}
		return floatValue(f), nil
	case ir.KindString:
		return stringValue(node.text), nil
	case ir.KindBool:
		switch node.text {
		case "true":
			return boolValue(true), nil
		case "false":
			return boolValue(false), nil
		default:
			return Value{}, mismatch()
		}
	case ir.KindEntity:
		if g, ok := in.generics[target.Entity]; ok && g.Template == "Список" {
			elems := make([]Value, 0, len(node.children))
			for _, c := range node.children {
				v, err := in.xmlExtract(c.node, g.ElemType)
				if err != nil {
					return Value{}, err
				}
				elems = append(elems, v)
			}
			return Value{Type: target, Obj: &entityObj{typeName: target.Entity, coll: &collection{template: "Список", elems: elems}}}, nil
		}
		ent, ok := in.entities[target.Entity]
		if !ok {
			return Value{}, mismatch()
		}
		fields := make(map[string]Value, len(ent.Fields))
		for _, f := range ent.Fields {
			xmlName, attr := f.XMLName()
			if attr {
				raw, present := node.attrs[xmlName]
				if !present {
					return Value{}, in.xmlParseFail(rtmsg.XMLFieldMissing(xmlName))
				}
				v, err := in.xmlExtract(xmlNode{text: raw}, f.Type)
				if err != nil {
					return Value{}, err
				}
				fields[f.Name] = v
				continue
			}
			matches := node.childrenNamed(xmlName)
			if g, ok := in.generics[f.Type.Entity]; f.Type.Kind == ir.KindEntity && ok && g.Template == "Список" {
				elems := make([]Value, 0, len(matches))
				for _, m := range matches {
					v, err := in.xmlExtract(m, g.ElemType)
					if err != nil {
						return Value{}, err
					}
					elems = append(elems, v)
				}
				fields[f.Name] = Value{Type: f.Type, Obj: &entityObj{typeName: f.Type.Entity, coll: &collection{template: "Список", elems: elems}}}
				continue
			}
			switch len(matches) {
			case 0:
				return Value{}, in.xmlParseFail(rtmsg.XMLFieldMissing(xmlName))
			case 1:
				v, err := in.xmlExtract(matches[0], f.Type)
				if err != nil {
					return Value{}, err
				}
				fields[f.Name] = v
			default:
				return Value{}, in.xmlParseFail(rtmsg.XMLFieldDuplicated(xmlName))
			}
		}
		return Value{Type: target, Obj: &entityObj{typeName: target.Entity, fields: fields}}, nil
	default:
		return Value{}, mismatch()
	}
}

func (p *xmlRawParser) skipWS() {
	for p.pos < len(p.s) {
		r, size := utf8.DecodeRuneInString(p.s[p.pos:])
		if !unicode.IsSpace(r) {
			return
		}
		p.pos += size
	}
}

// parseElement разбирает "<имя [атрибуты]>...</имя>" или самозакрывающийся
// "<имя [атрибуты]/>" - атрибуты сохраняются в xmlNode.attrs.
func (p *xmlRawParser) parseElement() (string, xmlNode, error) {
	if p.pos >= len(p.s) || p.s[p.pos] != '<' {
		return "", xmlNode{}, errXMLSyntax
	}
	p.pos++
	name, err := p.parseName()
	if err != nil {
		return "", xmlNode{}, err
	}
	var node xmlNode
	for {
		p.skipWS()
		if p.pos >= len(p.s) {
			return "", xmlNode{}, errXMLSyntax
		}
		if p.s[p.pos] == '/' {
			p.pos++
			if p.pos >= len(p.s) || p.s[p.pos] != '>' {
				return "", xmlNode{}, errXMLSyntax
			}
			p.pos++
			return name, node, nil
		}
		if p.s[p.pos] == '>' {
			p.pos++
			break
		}
		attrName, err := p.parseName()
		if err != nil {
			return "", xmlNode{}, err
		}
		p.skipWS()
		if p.pos >= len(p.s) || p.s[p.pos] != '=' {
			return "", xmlNode{}, errXMLSyntax
		}
		p.pos++
		p.skipWS()
		value, err := p.parseAttrValue()
		if err != nil {
			return "", xmlNode{}, err
		}
		if node.attrs == nil {
			node.attrs = map[string]string{}
		}
		if _, dup := node.attrs[attrName]; dup {
			return "", xmlNode{}, errXMLSyntax
		}
		node.attrs[attrName] = value
	}

	var text strings.Builder
	for {
		if p.pos >= len(p.s) {
			return "", xmlNode{}, errXMLSyntax
		}
		if p.s[p.pos] == '<' {
			if p.pos+1 < len(p.s) && p.s[p.pos+1] == '/' {
				p.pos += 2
				closeName, err := p.parseName()
				if err != nil {
					return "", xmlNode{}, err
				}
				p.skipWS()
				if p.pos >= len(p.s) || p.s[p.pos] != '>' || closeName != name {
					return "", xmlNode{}, errXMLSyntax
				}
				p.pos++
				node.text = strings.TrimSpace(text.String())
				return name, node, nil
			}
			childName, child, err := p.parseElement()
			if err != nil {
				return "", xmlNode{}, err
			}
			node.children = append(node.children, xmlChild{name: childName, node: child})
			continue
		}
		r, err := p.parseTextRune()
		if err != nil {
			return "", xmlNode{}, err
		}
		text.WriteRune(r)
	}
}

func (p *xmlRawParser) parseName() (string, error) {
	start := p.pos
	first := true
	for p.pos < len(p.s) {
		r, size := utf8.DecodeRuneInString(p.s[p.pos:])
		if !isXMLNameRune(r, first) {
			break
		}
		first = false
		p.pos += size
	}
	if p.pos == start {
		return "", errXMLSyntax
	}
	return p.s[start:p.pos], nil
}

func isXMLNameRune(r rune, first bool) bool {
	if unicode.IsLetter(r) || r == '_' || r == ':' {
		return true
	}
	return !first && (unicode.IsDigit(r) || r == '-' || r == '.')
}

// parseAttrValue читает значение атрибута в кавычках (двойных или
// одинарных) с тем же раскрытием сущностей, что и у текста (parseTextRune).
func (p *xmlRawParser) parseAttrValue() (string, error) {
	if p.pos >= len(p.s) || (p.s[p.pos] != '"' && p.s[p.pos] != '\'') {
		return "", errXMLSyntax
	}
	quote := p.s[p.pos]
	p.pos++
	var val strings.Builder
	for p.pos < len(p.s) && p.s[p.pos] != quote {
		if p.s[p.pos] == '<' {
			return "", errXMLSyntax
		}
		r, err := p.parseTextRune()
		if err != nil {
			return "", err
		}
		val.WriteRune(r)
	}
	if p.pos >= len(p.s) {
		return "", errXMLSyntax
	}
	p.pos++
	return val.String(), nil
}

// parseTextRune читает один символ текстового содержимого, разворачивая
// предопределённые сущности (&amp; &lt; &gt; &quot; &apos;) и числовые
// ссылки (&#NNN; / &#xHHH;) - без DOCTYPE/произвольных именованных
// сущностей (известное ограничение этого среза, см. package doc).
func (p *xmlRawParser) parseTextRune() (rune, error) {
	if p.s[p.pos] != '&' {
		r, size := utf8.DecodeRuneInString(p.s[p.pos:])
		p.pos += size
		return r, nil
	}
	end := strings.IndexByte(p.s[p.pos:], ';')
	if end < 0 {
		return 0, errXMLSyntax
	}
	ent := p.s[p.pos+1 : p.pos+end]
	p.pos += end + 1
	switch ent {
	case "amp":
		return '&', nil
	case "lt":
		return '<', nil
	case "gt":
		return '>', nil
	case "quot":
		return '"', nil
	case "apos":
		return '\'', nil
	}
	if strings.HasPrefix(ent, "#x") || strings.HasPrefix(ent, "#X") {
		n, err := strconv.ParseInt(ent[2:], 16, 32)
		if err != nil {
			return 0, errXMLSyntax
		}
		return rune(n), nil
	}
	if strings.HasPrefix(ent, "#") {
		n, err := strconv.ParseInt(ent[1:], 10, 32)
		if err != nil {
			return 0, errXMLSyntax
		}
		return rune(n), nil
	}
	return 0, errXMLSyntax
}

var errXMLSyntax = xmlSyntaxError{}

type xmlSyntaxError struct{}

func (xmlSyntaxError) Error() string { return "некорректный XML" }
