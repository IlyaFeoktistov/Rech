package protoimport

import (
	"fmt"
	"strings"
)

// scalarTypes - имена скалярных типов ".proto", которые совместимы с уже
// существующим wire-кодеком этого компилятора (internal/interp/protobuf.go,
// internal/backend/cbackend/protobuf.go): каждый Rech-Kind кодируется
// РОВНО ОДНИМ способом (Целое - всегда varint без zigzag, Дробное/
// Действительное - всегда fixed32/fixed64 как IEEE-754). У ".proto" есть
// РАЗНЫЕ типы для одного и того же Rech-Kind с РАЗНЫМ представлением на
// wire - `sint32`/`sint64` (zigzag varint - другая битовая раскладка,
// НЕСОВМЕСТИМАЯ с обычным varint) и `fixed32`/`fixed64`/`sfixed32`/
// `sfixed64` (32/64-битное ЦЕЛОЕ на wire-type 5/1, а не IEEE-754 -
// несовместимо с тем, как этот кодек уже интерпретирует эти wire-типы для
// Дробное/Действительное). Сгенерировать для них Rech-поле молча означало
// бы неверно читать/писать байты с настоящим protobuf-собеседником -
// см. package doc generate.go - вместо этого explicitlyUnsupportedTypes
// ниже отказывает явной ошибкой.
var scalarTypes = map[string]string{
	"double": "Действительное",
	"float":  "Дробное",
	"int32":  "Целое",
	"int64":  "Целое",
	"uint32": "Целое",
	"uint64": "Целое",
	"bool":   "Логическое",
	"string": "Строка",
	"bytes":  "Список<Байт>",
}

// explicitlyUnsupportedScalarTypes - см. package doc scalarTypes выше -
// требуют wire-представления, которого этот кодек не реализует (пока).
var explicitlyUnsupportedScalarTypes = map[string]bool{
	"sint32": true, "sint64": true,
	"fixed32": true, "fixed64": true,
	"sfixed32": true, "sfixed64": true,
}

type parser struct {
	toks []token
	pos  int
}

func parseFile(src string) (*file, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	return p.parseFile()
}

func (p *parser) cur() token { return p.toks[p.pos] }
func (p *parser) advance() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) expectPunct(s string) (token, error) {
	if p.cur().kind == tokPunct && p.cur().value == s {
		return p.advance(), nil
	}
	return token{}, fmt.Errorf("строка %d: ожидался %q, получено %q", p.cur().line, s, p.cur().value)
}

func (p *parser) expectIdent() (token, error) {
	if p.cur().kind == tokIdent {
		return p.advance(), nil
	}
	return token{}, fmt.Errorf("строка %d: ожидался идентификатор, получено %q", p.cur().line, p.cur().value)
}

func (p *parser) at(kind tokenKind, value string) bool {
	return p.cur().kind == kind && (value == "" || p.cur().value == value)
}

func (p *parser) parseFile() (*file, error) {
	f := &file{}
	for p.cur().kind != tokEOF {
		switch {
		case p.at(tokIdent, "syntax"):
			if err := p.parseSyntax(f); err != nil {
				return nil, err
			}
		case p.at(tokIdent, "package"):
			if err := p.parsePackage(f); err != nil {
				return nil, err
			}
		case p.at(tokIdent, "import"):
			if err := p.parseImport(f); err != nil {
				return nil, err
			}
		case p.at(tokIdent, "option"):
			if err := p.skipToSemicolon(); err != nil {
				return nil, err
			}
		case p.at(tokIdent, "message"):
			m, err := p.parseMessage()
			if err != nil {
				return nil, err
			}
			f.messages = append(f.messages, m)
		case p.at(tokIdent, "enum"):
			e, err := p.parseEnum()
			if err != nil {
				return nil, err
			}
			f.enums = append(f.enums, e)
		case p.at(tokIdent, "service"):
			svc, err := p.parseService()
			if err != nil {
				return nil, err
			}
			f.services = append(f.services, svc)
		case p.at(tokPunct, ";"):
			p.advance()
		default:
			return nil, fmt.Errorf("строка %d: неожиданная верхнеуровневая конструкция %q", p.cur().line, p.cur().value)
		}
	}
	return f, nil
}

func (p *parser) parseSyntax(f *file) error {
	p.advance() // "syntax"
	if _, err := p.expectPunct("="); err != nil {
		return err
	}
	tok := p.cur()
	if tok.kind != tokString {
		return fmt.Errorf("строка %d: \"syntax\" ожидает строковый литерал", tok.line)
	}
	p.advance()
	f.syntax = tok.value
	if _, err := p.expectPunct(";"); err != nil {
		return err
	}
	if f.syntax != "proto3" {
		return fmt.Errorf("строка %d: поддержан только \"syntax = \\\"proto3\\\"\" - proto2 не поддержан (другая семантика presence у полей, несовместимая с этим импортёром)", tok.line)
	}
	return nil
}

func (p *parser) parsePackage(f *file) error {
	p.advance() // "package"
	var parts []string
	for {
		id, err := p.expectIdent()
		if err != nil {
			return err
		}
		parts = append(parts, id.value)
		if p.at(tokPunct, ".") {
			p.advance()
			continue
		}
		break
	}
	if _, err := p.expectPunct(";"); err != nil {
		return err
	}
	f.pkg = strings.Join(parts, ".")
	return nil
}

func (p *parser) parseImport(f *file) error {
	tok := p.advance() // "import"
	// "import public "..."" / "import weak "..."" - модификатор перед
	// строкой, необязателен - см. https://protobuf.dev/programming-guides/proto3
	if p.at(tokIdent, "public") || p.at(tokIdent, "weak") {
		p.advance()
	}
	strTok := p.cur()
	if strTok.kind != tokString {
		return fmt.Errorf("строка %d: \"import\" ожидает строковый литерал пути", strTok.line)
	}
	p.advance()
	if _, err := p.expectPunct(";"); err != nil {
		return err
	}
	f.imports = append(f.imports, importDecl{path: strTok.value, line: tok.line})
	return nil
}

// parseService - "service Имя { (rpc ... | option ...;)* }" - rpc-методы
// разбираются (см. parseRPC), "option" на уровне самого service (не
// внутри rpc) пропускается целиком, тем же принципом, что и у "option"
// внутри message (см. skipFieldOptions/аналоги).
func (p *parser) parseService() (serviceDecl, error) {
	tok := p.advance() // "service"
	nameTok, err := p.expectIdent()
	if err != nil {
		return serviceDecl{}, err
	}
	if _, err := p.expectPunct("{"); err != nil {
		return serviceDecl{}, err
	}
	svc := serviceDecl{name: nameTok.value, line: tok.line}
	for !p.at(tokPunct, "}") {
		if p.cur().kind == tokEOF {
			return serviceDecl{}, fmt.Errorf("строка %d: незакрытая \"{\" у service %q", tok.line, nameTok.value)
		}
		switch {
		case p.at(tokIdent, "rpc"):
			m, err := p.parseRPC()
			if err != nil {
				return serviceDecl{}, err
			}
			svc.methods = append(svc.methods, m)
		case p.at(tokIdent, "option"):
			if err := p.skipToSemicolon(); err != nil {
				return serviceDecl{}, err
			}
		default:
			return serviceDecl{}, fmt.Errorf("строка %d: неожиданный токен %q внутри service %q (ожидался \"rpc\" или \"option\")", p.cur().line, p.cur().value, nameTok.value)
		}
	}
	p.advance() // "}"
	return svc, nil
}

// parseRPC - "rpc Имя ( [stream] Запрос ) returns ( [stream] Ответ )" +
// либо ";", либо "{ (option ...;)* }" (тело rpc - только options в
// настоящем ".proto", generate.go по нему ничего не порождает).
func (p *parser) parseRPC() (rpcDecl, error) {
	tok := p.advance() // "rpc"
	nameTok, err := p.expectIdent()
	if err != nil {
		return rpcDecl{}, err
	}
	reqType, reqStreaming, err := p.parseRPCParenType()
	if err != nil {
		return rpcDecl{}, err
	}
	if !p.at(tokIdent, "returns") {
		return rpcDecl{}, fmt.Errorf("строка %d: у \"rpc %s\" ожидалось \"returns\"", p.cur().line, nameTok.value)
	}
	p.advance()
	respType, respStreaming, err := p.parseRPCParenType()
	if err != nil {
		return rpcDecl{}, err
	}
	if p.at(tokPunct, "{") {
		if err := p.skipBalancedBraces(); err != nil {
			return rpcDecl{}, err
		}
	} else if _, err := p.expectPunct(";"); err != nil {
		return rpcDecl{}, err
	}
	return rpcDecl{name: nameTok.value, reqType: reqType, reqStreaming: reqStreaming, respType: respType, respStreaming: respStreaming, line: tok.line}, nil
}

// parseRPCParenType - "( [stream] Тип )" - общая форма и у списка запроса,
// и у "returns (...)".
func (p *parser) parseRPCParenType() (typeName string, streaming bool, err error) {
	if _, err := p.expectPunct("("); err != nil {
		return "", false, err
	}
	if p.at(tokIdent, "stream") {
		streaming = true
		p.advance()
	}
	typeName, err = p.parseTypeName()
	if err != nil {
		return "", false, err
	}
	if _, err := p.expectPunct(")"); err != nil {
		return "", false, err
	}
	return typeName, streaming, nil
}

// skipBalancedBraces пропускает "{ ... }" (уже на "{") с учётом вложенности -
// используется для "service ... { ... }" (rpc-методы не разбираются -
// gRPC ещё не реализован, см. package doc generate.go) и любых
// незнакомых/неважных для генерации блоков.
func (p *parser) skipBalancedBraces() error {
	if _, err := p.expectPunct("{"); err != nil {
		return err
	}
	depth := 1
	for depth > 0 {
		if p.cur().kind == tokEOF {
			return fmt.Errorf("строка %d: незакрытая \"{\"", p.cur().line)
		}
		if p.at(tokPunct, "{") {
			depth++
		} else if p.at(tokPunct, "}") {
			depth--
		}
		p.advance()
	}
	return nil
}

func (p *parser) skipToSemicolon() error {
	for {
		if p.cur().kind == tokEOF {
			return fmt.Errorf("строка %d: ожидался \";\"", p.cur().line)
		}
		if p.at(tokPunct, ";") {
			p.advance()
			return nil
		}
		p.advance()
	}
}

func (p *parser) parseMessage() (*message, error) {
	tok := p.advance() // "message"
	nameTok, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	m := &message{name: nameTok.value, line: tok.line}
	if _, err := p.expectPunct("{"); err != nil {
		return nil, err
	}
	for !p.at(tokPunct, "}") {
		if p.cur().kind == tokEOF {
			return nil, fmt.Errorf("строка %d: незакрытое \"message %s {\"", tok.line, m.name)
		}
		switch {
		case p.at(tokIdent, "message"):
			nested, err := p.parseMessage()
			if err != nil {
				return nil, err
			}
			m.nested = append(m.nested, nested)
		case p.at(tokIdent, "enum"):
			e, err := p.parseEnum()
			if err != nil {
				return nil, err
			}
			m.nestedEnums = append(m.nestedEnums, e)
		case p.at(tokIdent, "oneof"):
			if err := p.parseOneof(m); err != nil {
				return nil, err
			}
		case p.at(tokIdent, "reserved"), p.at(tokIdent, "option"):
			if err := p.skipToSemicolon(); err != nil {
				return nil, err
			}
		case p.at(tokPunct, ";"):
			p.advance()
		default:
			f, err := p.parseField()
			if err != nil {
				return nil, err
			}
			m.fields = append(m.fields, f)
		}
	}
	p.advance() // "}"
	return m, nil
}

// parseOneof - "oneof имя { Тип поле1 = N; ... }" - разбирается (не валит
// парсер на реальном файле), но каждое поле помечается isOneofPart - см.
// package doc generate.go, почему "oneof" целиком не поддержан генерацией.
func (p *parser) parseOneof(m *message) error {
	p.advance() // "oneof"
	nameTok, err := p.expectIdent()
	if err != nil {
		return err
	}
	if _, err := p.expectPunct("{"); err != nil {
		return err
	}
	for !p.at(tokPunct, "}") {
		if p.cur().kind == tokEOF {
			return fmt.Errorf("строка %d: незакрытый \"oneof %s {\"", nameTok.line, nameTok.value)
		}
		f, err := p.parseField()
		if err != nil {
			return err
		}
		f.isOneofPart = true
		f.oneofName = nameTok.value
		m.fields = append(m.fields, f)
	}
	p.advance() // "}"
	return nil
}

// parseField - "[repeated] Тип имя = номер [ [опции] ] ;" или
// "map<Ключ, Значение> имя = номер ;".
func (p *parser) parseField() (*field, error) {
	line := p.cur().line
	repeated := false
	if p.at(tokIdent, "repeated") {
		repeated = true
		p.advance()
	}
	if p.at(tokIdent, "optional") {
		// proto3 "optional" (explicit presence, отличается от обычного
		// implicit presence) - модификатор принимается синтаксически, но
		// семантика "явно присвоено или нет" этим импортёром не
		// переносится (генерируемое поле ведёт себя как обычное implicit-
		// presence поле, тем же принципом, что и все остальные скаляры).
		p.advance()
	}
	if p.at(tokIdent, "map") {
		return p.parseMapField(line)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return nil, err
	}
	nameTok, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPunct("="); err != nil {
		return nil, err
	}
	numTok := p.cur()
	if numTok.kind != tokInt {
		return nil, fmt.Errorf("строка %d: ожидался номер поля", numTok.line)
	}
	p.advance()
	num, err := parseIntLiteral(numTok.value)
	if err != nil {
		return nil, fmt.Errorf("строка %d: неверный номер поля %q: %w", numTok.line, numTok.value, err)
	}
	if p.at(tokPunct, "[") {
		if err := p.skipFieldOptions(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expectPunct(";"); err != nil {
		return nil, err
	}
	return &field{name: nameTok.value, rawType: typeName, repeated: repeated, number: num, line: line}, nil
}

// parseTypeName - имя типа поля, возможно квалифицированное точками
// ("пакет.Вложенный.Тип" или ".абсолютный.путь.Тип") - см. resolve.go,
// lastSegment, как это разрешается в реальный тип.
func (p *parser) parseTypeName() (string, error) {
	name := ""
	if p.at(tokPunct, ".") {
		name = "."
		p.advance()
	}
	id, err := p.expectIdent()
	if err != nil {
		return "", err
	}
	name += id.value
	for p.at(tokPunct, ".") {
		p.advance()
		id, err := p.expectIdent()
		if err != nil {
			return "", err
		}
		name += "." + id.value
	}
	return name, nil
}

func (p *parser) parseMapField(line int) (*field, error) {
	p.advance() // "map"
	if _, err := p.expectPunct("<"); err != nil {
		return nil, err
	}
	keyType, err := p.parseTypeName()
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPunct(","); err != nil {
		return nil, err
	}
	valType, err := p.parseTypeName()
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPunct(">"); err != nil {
		return nil, err
	}
	nameTok, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPunct("="); err != nil {
		return nil, err
	}
	numTok := p.cur()
	if numTok.kind != tokInt {
		return nil, fmt.Errorf("строка %d: ожидался номер поля", numTok.line)
	}
	p.advance()
	num, err := parseIntLiteral(numTok.value)
	if err != nil {
		return nil, err
	}
	if p.at(tokPunct, "[") {
		if err := p.skipFieldOptions(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expectPunct(";"); err != nil {
		return nil, err
	}
	return &field{name: nameTok.value, number: num, line: line, mapKeyType: keyType, mapValType: valType}, nil
}

// skipFieldOptions пропускает "[ключ = значение, ...]" после номера поля
// (уже на "[") - не несёт значимой для генерации информации в этом срезе
// (deprecated/json_name/packed и т. п.).
func (p *parser) skipFieldOptions() error {
	if _, err := p.expectPunct("["); err != nil {
		return err
	}
	depth := 1
	for depth > 0 {
		if p.cur().kind == tokEOF {
			return fmt.Errorf("строка %d: незакрытые опции поля \"[\"", p.cur().line)
		}
		if p.at(tokPunct, "[") {
			depth++
		} else if p.at(tokPunct, "]") {
			depth--
		}
		p.advance()
	}
	return nil
}

func (p *parser) parseEnum() (*enumType, error) {
	tok := p.advance() // "enum"
	nameTok, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	e := &enumType{name: nameTok.value, line: tok.line}
	if _, err := p.expectPunct("{"); err != nil {
		return nil, err
	}
	for !p.at(tokPunct, "}") {
		if p.cur().kind == tokEOF {
			return nil, fmt.Errorf("строка %d: незакрытый \"enum %s {\"", tok.line, e.name)
		}
		if p.at(tokIdent, "option") || p.at(tokIdent, "reserved") {
			if err := p.skipToSemicolon(); err != nil {
				return nil, err
			}
			continue
		}
		if p.at(tokPunct, ";") {
			p.advance()
			continue
		}
		valNameTok, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		if _, err := p.expectPunct("="); err != nil {
			return nil, err
		}
		neg := false
		if p.at(tokPunct, "-") {
			neg = true
			p.advance()
		}
		numTok := p.cur()
		if numTok.kind != tokInt {
			return nil, fmt.Errorf("строка %d: ожидалось числовое значение перечисления", numTok.line)
		}
		p.advance()
		num, err := parseIntLiteral(numTok.value)
		if err != nil {
			return nil, err
		}
		if neg {
			num = -num
		}
		if p.at(tokPunct, "[") {
			if err := p.skipFieldOptions(); err != nil {
				return nil, err
			}
		}
		if _, err := p.expectPunct(";"); err != nil {
			return nil, err
		}
		e.values = append(e.values, enumValue{name: valNameTok.value, number: num})
	}
	p.advance() // "}"
	return e, nil
}
