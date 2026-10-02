package parser

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// Понятные ошибки для поля, записанного не по грамматике "имеет:":
// "[статический] [видимость] Тип имя [= значение]". Иначе поле в форме
// "имя: Тип" упиралось бы в "ожидался тип" (внутри "имеет:"), а поле вне
// "имеет:" разбиралось бы как метод.

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

// modifierTokens - непустые из staticTok/visTok списком.
func modifierTokens(staticTok, visTok *lexer.Token) []lexer.Token {
	var mods []lexer.Token
	for _, m := range []*lexer.Token{staticTok, visTok} {
		if m != nil {
			mods = append(mods, *m)
		}
	}
	return mods
}

// fieldModifierPrefix - модификаторы поля в словах автора для подсказки с
// исправленной строкой: "" или "статическое публичное ".
func fieldModifierPrefix(mods []lexer.Token) string {
	var sb strings.Builder
	for _, m := range mods {
		sb.WriteString(m.Value + " ")
	}
	return sb.String()
}

// lookaheadField - пробует прочитать с позиции p.pos+offset поле в любой из
// двух форм: "Тип имя" или "имя: Тип"; позиция парсера не меняется.
// nameFirst - форма "имя: Тип"; ok == false - не похоже на поле.
func (p *Parser) lookaheadField(offset int) (typeName, fieldName string, nameFirst, ok bool) {
	save, saveGT := p.pos, p.pendingGT
	defer func() { p.pos, p.pendingGT = save, saveGT }()
	p.pos += offset
	if p.cur().Type == lexer.IDENT && p.peekAt(1).Type == lexer.COLON {
		fieldName = p.cur().Value
		p.pos += 2
		t, err := p.parseTypeName()
		if err != nil {
			return "", "", false, false
		}
		return t, fieldName, true, true
	}
	t, err := p.parseTypeName()
	if err != nil || p.cur().Type != lexer.IDENT {
		return "", "", false, false
	}
	return t, p.cur().Value, false, true
}

// fieldOutsideHasError - член сущности вне "имеет:" выглядит как поле, а не
// как метод (у метода после имени всегда "("): ошибка с подсказкой перенести
// его в "имеет:". nil - похоже на метод, пусть разбирает parseMethod.
func (p *Parser) fieldOutsideHasError() error {
	var mods []lexer.Token
	for p.peekAt(len(mods)).Type == lexer.KwStatic || isVisibilityKeyword(p.peekAt(len(mods)).Type) {
		mods = append(mods, p.peekAt(len(mods)))
	}
	first, next := p.peekAt(len(mods)), p.peekAt(len(mods)+1)
	if first.Type == lexer.KwOperator || (first.Type == lexer.IDENT && next.Type == lexer.LPAREN) {
		return nil
	}
	typeName, name, nameFirst, ok := p.lookaheadField(len(mods))
	// "имя: Тип" - и поле, и метод без "()": полем считается, только если
	// модификатор в форме, невозможной у метода.
	if !ok || (nameFirst && !hasNonMethodModifier(mods)) {
		return nil
	}
	tok := p.cur()
	return errAt(tok.Line, tok.Col, CodeUnexpectedToken, "поле объявляется в разделе \"имеет:\" в форме \"Тип имя\"").
		WithHint("имеет:\n    %s%s %s", fieldModifierPrefix(mods), typeName, name)
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

// nameFirstFieldError - внутри "имеет:" поле записано как "имя: Тип"
// вместо "Тип имя". p.pos - на первом слове после модификаторов. nil - это
// не такая форма.
func (p *Parser) nameFirstFieldError(mods []lexer.Token) error {
	if p.cur().Type != lexer.IDENT || p.peekAt(1).Type != lexer.COLON {
		return nil
	}
	tok := p.cur()
	typeName, name, _, ok := p.lookaheadField(0)
	if !ok {
		return nil
	}
	return errAt(tok.Line, tok.Col, CodeUnexpectedToken, "поле объявляется в форме \"Тип имя\", не \"имя: Тип\"").
		WithHint("%s%s %s", fieldModifierPrefix(mods), typeName, name)
}
