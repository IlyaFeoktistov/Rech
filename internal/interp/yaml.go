// "разобрать YAML <выражение> как <Тип>" (internal/ir.ParseExpr) - тот же
// принцип, что json.go/xml.go, но БЕЗ стандартной библиотеки Go (в
// отличие от encoding/json - у Go нет встроенного YAML) - ручной блочный
// парсер, порт байт-в-байт по алгоритму
// internal/backend/cbackend/yaml_extract.go (см. её package doc для
// полного описания поддержанного подмножества и синтаксиса). Оба
// backend'а обязаны разбирать один и тот же текст одинаково - при
// малейшем расхождении поведения сверяться сначала с C-версией, не
// заново придумывать логику здесь.
package interp

import (
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/decimal"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

type yamlKind int

const (
	yamlNull yamlKind = iota
	yamlBool
	yamlNum
	yamlStr
	yamlSeq
	yamlMap
)

// yamlValue - тот же принцип, что rech_yv (cbackend) - размеченное
// объединение kind + один активный вариант.
type yamlValue struct {
	kind  yamlKind
	b     bool
	num   float64
	str   string
	items []yamlValue
	keys  []string
}

// yamlParser - курсор по тексту (байтовый индекс - структурные символы
// YAML в этом подмножестве все ASCII, декодировать руны для отступов/
// двоеточий/тире не нужно; содержимое строк копируется как есть, включая
// многобайтовые UTF-8 символы, byte-slice остаётся валидным UTF-8).
type yamlParser struct {
	text string
	pos  int
}

func (in *Interp) yamlParseFail(msg string) error {
	return newSystemException(parseErrorEntity, msg)
}

func (in *Interp) evalYAMLParseExpr(e *ir.ParseExpr, src Value) (Value, error) {
	p := &yamlParser{text: src.S}
	v, err := in.yamlParseDocument(p)
	if err != nil {
		return Value{}, err
	}
	return in.yamlExtract(v, e.To)
}

func (p *yamlParser) atEOF() bool { return p.pos >= len(p.text) }
func (p *yamlParser) cur() byte {
	if p.atEOF() {
		return 0
	}
	return p.text[p.pos]
}
func (p *yamlParser) at(off int) byte {
	if p.pos+off >= len(p.text) {
		return 0
	}
	return p.text[p.pos+off]
}

func (p *yamlParser) isBlankOrCommentAt(pos int) bool {
	i := pos
	for i < len(p.text) && p.text[i] == ' ' {
		i++
	}
	if i >= len(p.text) {
		return true
	}
	c := p.text[i]
	return c == '\n' || c == '\r' || c == '#'
}

func (p *yamlParser) nextLine() {
	for !p.atEOF() && p.text[p.pos] != '\n' {
		p.pos++
	}
	if !p.atEOF() {
		p.pos++
	}
}

func (p *yamlParser) skipBlankLines() {
	for !p.atEOF() && p.isBlankOrCommentAt(p.pos) {
		p.nextLine()
	}
}

func (p *yamlParser) curIndent() int {
	n := 0
	for p.pos+n < len(p.text) && p.text[p.pos+n] == ' ' {
		n++
	}
	return n
}

func (p *yamlParser) skipInlineSpaces() {
	for !p.atEOF() && (p.cur() == ' ' || p.cur() == '\t') {
		p.pos++
	}
}

func (p *yamlParser) atEOLOrComment() bool {
	return p.atEOF() || p.cur() == '\n' || p.cur() == '\r' || p.cur() == '#'
}

func (p *yamlParser) skipTrailingCommentAndNewline() error {
	p.skipInlineSpaces()
	if p.cur() == '#' {
		for !p.atEOF() && p.cur() != '\n' {
			p.pos++
		}
	}
	if !p.atEOF() && p.cur() != '\n' && p.cur() != '\r' {
		return newSystemException(parseErrorEntity, rtmsg.YAMLParseError)
	}
	p.nextLine()
	return nil
}

func (p *yamlParser) parseDquoted() (string, error) {
	p.pos++ // "\""
	var sb strings.Builder
	for {
		if p.atEOF() || p.cur() == '\n' {
			return "", newSystemException(parseErrorEntity, rtmsg.YAMLParseError)
		}
		c := p.cur()
		if c == '"' {
			p.pos++
			break
		}
		if c == '\\' {
			p.pos++
			switch p.cur() {
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			default:
				sb.WriteByte(p.cur())
			}
			p.pos++
			continue
		}
		sb.WriteByte(c)
		p.pos++
	}
	return sb.String(), nil
}

func (p *yamlParser) parseSquoted() (string, error) {
	p.pos++ // "'"
	var sb strings.Builder
	for {
		if p.atEOF() || p.cur() == '\n' {
			return "", newSystemException(parseErrorEntity, rtmsg.YAMLParseError)
		}
		c := p.cur()
		if c == '\'' {
			if p.at(1) == '\'' {
				sb.WriteByte('\'')
				p.pos += 2
				continue
			}
			p.pos++
			break
		}
		sb.WriteByte(c)
		p.pos++
	}
	return sb.String(), nil
}

// classifyPlain - тот же принцип, что rech_yaml_classify_plain (cbackend).
func classifyPlain(text string) yamlValue {
	switch text {
	case "", "~", "null", "Null", "NULL":
		return yamlValue{kind: yamlNull}
	case "true", "True", "TRUE":
		return yamlValue{kind: yamlBool, b: true}
	case "false", "False", "FALSE":
		return yamlValue{kind: yamlBool, b: false}
	}
	if n, err := strconv.ParseFloat(text, 64); err == nil {
		return yamlValue{kind: yamlNum, num: n, str: text}
	}
	return yamlValue{kind: yamlStr, str: text}
}

// parseScalarLine - тот же принцип, что rech_yaml_parse_scalar_line -
// разбирает скаляр, начиная с p.pos, и продвигает курсор до начала
// следующей строки.
func (p *yamlParser) parseScalarLine() (yamlValue, error) {
	p.skipInlineSpaces()
	var v yamlValue
	switch {
	case p.cur() == '"':
		s, err := p.parseDquoted()
		if err != nil {
			return yamlValue{}, err
		}
		v = yamlValue{kind: yamlStr, str: s}
	case p.cur() == '\'':
		s, err := p.parseSquoted()
		if err != nil {
			return yamlValue{}, err
		}
		v = yamlValue{kind: yamlStr, str: s}
	case p.cur() == '[' && p.at(1) == ']':
		p.pos += 2
		v = yamlValue{kind: yamlSeq}
	case p.cur() == '{' && p.at(1) == '}':
		p.pos += 2
		v = yamlValue{kind: yamlMap}
	default:
		start := p.pos
		end := start
		for end < len(p.text) && p.text[end] != '\n' && p.text[end] != '\r' {
			if p.text[end] == '#' && end > start && p.text[end-1] == ' ' {
				break
			}
			end++
		}
		for end > start && (p.text[end-1] == ' ' || p.text[end-1] == '\t') {
			end--
		}
		v = classifyPlain(p.text[start:end])
		p.pos = end
	}
	if err := p.skipTrailingCommentAndNewline(); err != nil {
		return yamlValue{}, err
	}
	return v, nil
}

// parseKey - тот же принцип, что rech_yaml_parse_key - разбирает ключ
// отображения (включая двоеточие-разделитель), не трогая значение после
// него.
func (p *yamlParser) parseKey() (string, error) {
	var key string
	switch p.cur() {
	case '"':
		s, err := p.parseDquoted()
		if err != nil {
			return "", err
		}
		key = s
	case '\'':
		s, err := p.parseSquoted()
		if err != nil {
			return "", err
		}
		key = s
	default:
		start := p.pos
		colon := -1
		i := start
		for i < len(p.text) && p.text[i] != '\n' && p.text[i] != '\r' {
			if p.text[i] == ':' {
				next := byte(0)
				if i+1 < len(p.text) {
					next = p.text[i+1]
				}
				if next == ' ' || next == 0 || next == '\n' || next == '\r' {
					colon = i
					break
				}
			}
			i++
		}
		if colon < 0 {
			return "", newSystemException(parseErrorEntity, rtmsg.YAMLParseError)
		}
		end := colon
		for end > start && (p.text[end-1] == ' ' || p.text[end-1] == '\t') {
			end--
		}
		key = p.text[start:end]
		p.pos = colon
	}
	p.skipInlineSpaces()
	if p.cur() != ':' {
		return "", newSystemException(parseErrorEntity, rtmsg.YAMLParseError)
	}
	p.pos++
	return key, nil
}

// parseMap - тот же принцип, что rech_yaml_parse_map - "ключ: значение"
// на каждой строке с отступом РОВНО keyCol; firstInline - для компактной
// записи внутри последовательности ("- ключ: значение"), см. её package
// doc в yaml_extract.go.
func (p *yamlParser) parseMap(keyCol int, firstInline bool) (yamlValue, error) {
	v := yamlValue{kind: yamlMap}
	first := true
	for {
		if !(first && firstInline) {
			p.skipBlankLines()
			if p.atEOF() {
				break
			}
			if p.curIndent() != keyCol {
				break
			}
			p.pos += keyCol
		}
		first = false
		key, err := p.parseKey()
		if err != nil {
			return yamlValue{}, err
		}
		p.skipInlineSpaces()
		var val yamlValue
		if p.atEOLOrComment() {
			if p.cur() == '#' {
				for !p.atEOF() && p.cur() != '\n' {
					p.pos++
				}
			}
			p.nextLine()
			p.skipBlankLines()
			if p.atEOF() || p.curIndent() <= keyCol {
				val = yamlValue{kind: yamlNull}
			} else {
				childIndent := p.curIndent()
				val, err = p.parseNode(childIndent)
				if err != nil {
					return yamlValue{}, err
				}
			}
		} else {
			val, err = p.parseScalarLine()
			if err != nil {
				return yamlValue{}, err
			}
		}
		v.keys = append(v.keys, key)
		v.items = append(v.items, val)
	}
	return v, nil
}

// parseSeq - тот же принцип, что rech_yaml_parse_seq.
func (p *yamlParser) parseSeq(seqCol int) (yamlValue, error) {
	v := yamlValue{kind: yamlSeq}
	for {
		p.skipBlankLines()
		if p.atEOF() {
			break
		}
		if p.curIndent() != seqCol {
			break
		}
		at := p.pos + seqCol
		if !(p.text[at] == '-' && (at+1 >= len(p.text) || p.text[at+1] == ' ' || p.text[at+1] == '\n' || p.text[at+1] == '\r')) {
			break
		}
		p.pos = at + 1
		p.skipInlineSpaces()
		itemCol := seqCol + 2
		var item yamlValue
		var err error
		switch {
		case p.atEOLOrComment():
			if p.cur() == '#' {
				for !p.atEOF() && p.cur() != '\n' {
					p.pos++
				}
			}
			p.nextLine()
			p.skipBlankLines()
			if p.atEOF() || p.curIndent() < itemCol {
				item = yamlValue{kind: yamlNull}
			} else {
				childIndent := p.curIndent()
				item, err = p.parseNode(childIndent)
				if err != nil {
					return yamlValue{}, err
				}
			}
		default:
			// Компактная запись отображения "- ключ: значение".
			s := p.pos
			colon := -1
			for s < len(p.text) && p.text[s] != '\n' && p.text[s] != '\r' {
				if p.text[s] == ':' {
					next := byte(0)
					if s+1 < len(p.text) {
						next = p.text[s+1]
					}
					if next == ' ' || next == 0 || next == '\n' || next == '\r' {
						colon = s
						break
					}
				}
				s++
			}
			if colon >= 0 && p.cur() != '"' && p.cur() != '\'' {
				item, err = p.parseMap(itemCol, true)
			} else {
				item, err = p.parseScalarLine()
			}
			if err != nil {
				return yamlValue{}, err
			}
		}
		v.items = append(v.items, item)
	}
	return v, nil
}

// parseNode - тот же принцип, что rech_yaml_parse_node.
func (p *yamlParser) parseNode(minIndent int) (yamlValue, error) {
	p.skipBlankLines()
	if p.atEOF() {
		return yamlValue{kind: yamlNull}, nil
	}
	indent := p.curIndent()
	if indent < minIndent {
		return yamlValue{kind: yamlNull}, nil
	}
	content := p.pos + indent
	if content+1 < len(p.text) && p.text[content] == '[' && p.text[content+1] == ']' {
		p.pos = content + 2
		if err := p.skipTrailingCommentAndNewline(); err != nil {
			return yamlValue{}, err
		}
		return yamlValue{kind: yamlSeq}, nil
	}
	if content+1 < len(p.text) && p.text[content] == '{' && p.text[content+1] == '}' {
		p.pos = content + 2
		if err := p.skipTrailingCommentAndNewline(); err != nil {
			return yamlValue{}, err
		}
		return yamlValue{kind: yamlMap}, nil
	}
	if content < len(p.text) && p.text[content] == '-' &&
		(content+1 >= len(p.text) || p.text[content+1] == ' ' || p.text[content+1] == '\n' || p.text[content+1] == '\r') {
		return p.parseSeq(indent)
	}
	// Голый скаляр как ЦЕЛЫЙ документ/вложенный блок (например, "разобрать
	// YAML \"42\" как Целое") - строка не выглядит ключом отображения
	// вовсе, значит это не "ключ: значение", а сам скаляр целиком - тот же
	// принцип, что и rech_yaml_line_looks_like_key (cbackend).
	if !lineLooksLikeYAMLKey(p.text[content:]) {
		p.pos = content
		return p.parseScalarLine()
	}
	return p.parseMap(indent, false)
}

// lineLooksLikeYAMLKey - тот же принцип, что rech_yaml_line_looks_like_key
// (cbackend/yaml_extract.go) - кавычки (голый ключ может быть в них)
// сначала пропускаются целиком (с учётом экранирования), чтобы двоеточие
// ВНУТРИ строкового скаляра ("- часы: 10:30" как значение, не ключ) не
// спутало проверку.
func lineLooksLikeYAMLKey(content string) bool {
	i := 0
	n := len(content)
	switch {
	case i < n && content[i] == '"':
		i++
		for i < n && content[i] != '\n' && content[i] != '"' {
			if content[i] == '\\' && i+1 < n {
				i += 2
				continue
			}
			i++
		}
		if i < n && content[i] == '"' {
			i++
		}
	case i < n && content[i] == '\'':
		i++
		for i < n && content[i] != '\n' {
			if content[i] == '\'' && i+1 < n && content[i+1] == '\'' {
				i += 2
				continue
			}
			if content[i] == '\'' {
				i++
				break
			}
			i++
		}
	default:
		for i < n && content[i] != '\n' && content[i] != '\r' {
			if content[i] == ':' {
				next := byte(0)
				if i+1 < n {
					next = content[i+1]
				}
				if next == ' ' || next == 0 || next == '\n' || next == '\r' {
					return true
				}
			}
			i++
		}
		return false
	}
	for i < n && (content[i] == ' ' || content[i] == '\t') {
		i++
	}
	if i >= n {
		return false
	}
	next := byte(0)
	if i+1 < n {
		next = content[i+1]
	}
	return content[i] == ':' && (next == ' ' || next == 0 || next == '\n' || next == '\r')
}

func (in *Interp) yamlParseDocument(p *yamlParser) (yamlValue, error) {
	p.skipBlankLines()
	if strings.HasPrefix(p.text[p.pos:], "---") {
		after := p.pos + 3
		if after >= len(p.text) || p.text[after] == '\n' || p.text[after] == '\r' || p.text[after] == ' ' {
			p.nextLine()
		}
	}
	v, err := p.parseNode(0)
	if err != nil {
		return yamlValue{}, err
	}
	p.skipBlankLines()
	if strings.HasPrefix(p.text[p.pos:], "...") {
		p.nextLine()
		p.skipBlankLines()
	}
	if !p.atEOF() {
		return yamlValue{}, newSystemException(parseErrorEntity, rtmsg.YAMLParseError)
	}
	return v, nil
}

// yamlExtract - тот же принцип, что jsonExtract (json.go).
func (in *Interp) yamlExtract(v yamlValue, target ir.Type) (Value, error) {
	mismatch := func() error { return in.yamlParseFail(rtmsg.YAMLValueMismatch) }
	switch target.Kind {
	case ir.KindInt:
		if v.kind != yamlNum || v.num != float64(int64(v.num)) {
			return Value{}, mismatch()
		}
		return intValue(int64(v.num)), nil
	case ir.KindFloat32:
		if v.kind != yamlNum {
			return Value{}, mismatch()
		}
		return Value{Type: ir.TypeFloat32, F: v.num}, nil
	case ir.KindFloat:
		if v.kind != yamlNum {
			return Value{}, mismatch()
		}
		return floatValue(v.num), nil
	case ir.KindString:
		if v.kind != yamlStr {
			return Value{}, mismatch()
		}
		return stringValue(v.str), nil
	case ir.KindDecimal:
		d, err := decimal.Parse(v.str)
		if v.kind != yamlNum && v.kind != yamlStr || err != nil {
			return Value{}, mismatch()
		}
		return decimalValue(d), nil
	case ir.KindBool:
		if v.kind != yamlBool {
			return Value{}, mismatch()
		}
		return boolValue(v.b), nil
	case ir.KindEntity:
		if g, ok := in.generics[target.Entity]; ok && g.Template == "Список" {
			if v.kind != yamlSeq {
				return Value{}, mismatch()
			}
			elems := make([]Value, len(v.items))
			for i, item := range v.items {
				ev, err := in.yamlExtract(item, g.ElemType)
				if err != nil {
					return Value{}, err
				}
				elems[i] = ev
			}
			return Value{Type: target, Obj: &entityObj{typeName: target.Entity, coll: &collection{template: "Список", elems: elems}}}, nil
		}
		ent, ok := in.entities[target.Entity]
		if !ok {
			return Value{}, mismatch()
		}
		if v.kind != yamlMap {
			return Value{}, mismatch()
		}
		fields := make(map[string]Value, len(ent.Fields))
		for _, f := range ent.Fields {
			idx := -1
			for i, k := range v.keys {
				if k == f.JSONKey() {
					idx = i
					break
				}
			}
			if idx < 0 {
				return Value{}, in.yamlParseFail(rtmsg.YAMLFieldMissing(f.JSONKey()))
			}
			fv, err := in.yamlExtract(v.items[idx], f.Type)
			if err != nil {
				return Value{}, err
			}
			fields[f.Name] = fv
		}
		return Value{Type: target, Obj: &entityObj{typeName: target.Entity, fields: fields}}, nil
	default:
		return Value{}, mismatch()
	}
}
