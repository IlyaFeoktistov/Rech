package parser

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// Понятные ошибки для поля, записанного не по грамматике "имеет:":
// "[статический] [видимость] имя: Тип [= значение]". Иначе поле в форме
// "тип первым" ("публичное Целое начало = 100") разбиралось бы как метод
// (вне "имеет:") или упиралось в "ожидалось ':'" (внутри).

// fieldModifierPrefix - модификаторы поля в словах автора, но в порядке
// грамматики ("статический" первым), для подсказки с исправленной строкой:
// "" или "статическое публичное ".
func fieldModifierPrefix(mods []lexer.Token) string {
	var static, rest strings.Builder
	for _, m := range mods {
		if m.Type == lexer.KwStatic {
			static.WriteString(m.Value + " ")
		} else {
			rest.WriteString(m.Value + " ")
		}
	}
	return static.String() + rest.String()
}

// typeFirstField - пробует прочитать с позиции p.pos "Тип имя" (форма
// "тип первым"); позиция парсера не меняется. ok == false - это не она.
func (p *Parser) typeFirstField() (typeName, fieldName string, ok bool) {
	save, saveGT := p.pos, p.pendingGT
	defer func() { p.pos, p.pendingGT = save, saveGT }()
	t, err := p.parseTypeName()
	if err != nil || p.cur().Type != lexer.IDENT {
		return "", "", false
	}
	return t, p.cur().Value, true
}

// fieldOutsideHasError - член сущности вне "имеет:" выглядит как поле, а не
// как метод (у метода после имени всегда "("): ошибка с подсказкой перенести
// его в "имеет:". nil - похоже на метод, пусть разбирает parseMethod.
func (p *Parser) fieldOutsideHasError() error {
	start := p.pos
	var mods []lexer.Token
	for p.peekAt(len(mods)).Type == lexer.KwStatic || isVisibilityKeyword(p.peekAt(len(mods)).Type) {
		mods = append(mods, p.peekAt(len(mods)))
	}
	first, next := p.peekAt(len(mods)), p.peekAt(len(mods)+1)
	if first.Type == lexer.KwOperator || (first.Type == lexer.IDENT && next.Type == lexer.LPAREN) {
		return nil
	}
	var line string
	switch {
	case first.Type == lexer.IDENT && next.Type == lexer.COLON:
		// "имя: Тип" - и поле, и метод без "()": полем считается, только
		// если модификатор в форме, невозможной у метода.
		if !hasNonMethodModifier(mods) {
			return nil
		}
		line = fieldModifierPrefix(mods) + first.Value + ": ..."
	default:
		p.pos = start + len(mods)
		typeName, name, ok := p.typeFirstField()
		p.pos = start
		if !ok {
			return nil
		}
		line = fieldModifierPrefix(mods) + name + ": " + typeName
	}
	tok := p.cur()
	return errAt(tok.Line, tok.Col, CodeUnexpectedToken, "поле объявляется в разделе \"имеет:\" в форме \"имя: Тип\"").
		WithHint("имеет:\n    %s", line)
}

// hasNonMethodModifier - среди модификаторов есть форма не мужского рода
// ("публичное", "статическая"), у метода такой не бывает.
func hasNonMethodModifier(mods []lexer.Token) bool {
	for _, m := range mods {
		if m.Type == lexer.KwStatic {
			if m.Value != "статический" {
				return true
			}
		} else if m.Value != methodVisibilityWord(visibilityFromToken(m)) {
			return true
		}
	}
	return false
}

// typeFirstFieldError - внутри "имеет:" после "имени" поля стоит ещё одно
// имя: автор написал "Тип имя" вместо "имя: Тип". p.pos - на первом слове
// после модификаторов. nil - это не форма "тип первым".
func (p *Parser) typeFirstFieldError(mods []lexer.Token) error {
	tok := p.cur()
	typeName, name, ok := p.typeFirstField()
	if !ok {
		return nil
	}
	return errAt(tok.Line, tok.Col, CodeUnexpectedToken, "поле объявляется в форме \"имя: Тип\", не \"Тип имя\"").
		WithHint("%s%s: %s", fieldModifierPrefix(mods), name, typeName)
}

// memberModifiers - "статический" и модификатор видимости члена сущности
// в любом порядке, каждый не больше одного раза; nil - модификатора нет.
func (p *Parser) memberModifiers() (staticTok, visTok *lexer.Token) {
	for {
		switch {
		case staticTok == nil && p.cur().Type == lexer.KwStatic:
			tok := p.advance()
			staticTok = &tok
		case visTok == nil && isVisibilityKeyword(p.cur().Type):
			tok := p.advance()
			visTok = &tok
		default:
			return staticTok, visTok
		}
	}
}
