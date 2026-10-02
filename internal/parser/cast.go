package parser

import (
	"unicode"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// primitiveCastTypes - типы, для которых допустим явный каст "(Тип)
// значение" - примитивы плюс непрозрачные встроенные типы Файл/Сокет/БД
// (ir.KindFile/KindSocket/KindDB - как и примитивы, не сущности: сущности
// создаются через "новый", не приведением типа). Единственное практическое
// применение для Файл/Сокет/БД - разворот "?Файл"/"?Сокет"/"?БД" (см.
// semantic.analyzeCastExpr, ветка "from.Kind == ir.KindNullable") в местах,
// где сужение "если х != Ничто" неприменимо. Фиксированный набор нужен,
// чтобы разобрать "(" IDENT ")" как каст уже во время разбора, не дожидаясь
// семантического анализа - иначе нельзя было бы отличить "(Целое) x" (каст)
// от "(x)" (скобки вокруг выражения) без таблицы типов на этапе разбора.
var primitiveCastTypes = map[string]bool{
	"Целое":          true,
	"Действительное": true,
	"Строка":         true,
	"Логическое":     true,
	"Дробное":        true,
	"Символ":         true,
	"Байт":           true,
	"Целое8":         true,
	"Целое16":        true,
	"Целое32":        true,
	"Целое64":        true,
	"Натуральное":    true,
	"Натуральное8":   true,
	"Натуральное16":  true,
	"Натуральное32":  true,
	"Натуральное64":  true,
	"Значение":       true,
	"Файл":           true,
	"Сокет":          true,
	"БД":             true,
	"HTTP2Сессия":    true,
}

// tryParseCast проверяет, не начинается ли текущая позиция с "(Тип)" (Тип -
// одно из primitiveCastTypes) - если да, разбирает каст целиком, включая
// операнд на том же уровне приоритета, что и остальные унарные операторы
// (см. parseUnary - каст стоит первым в цепочке проверок, поэтому "(Целое)
// -x" и "-(Целое) x" разбираются рекурсивно правильно без отдельной
// грамматики). Если это не каст - позиция парсера не меняется (только
// peek), вызывающая сторона продолжает обычный разбор.
func (p *Parser) tryParseCast() (ast.Expr, bool, error) {
	if p.cur().Type != lexer.LPAREN {
		return nil, false, nil
	}
	typeTok := p.peekAt(1)
	if typeTok.Type == lexer.STAR || (typeTok.Type == lexer.IDENT && !primitiveCastTypes[typeTok.Value]) {
		return p.tryParseNamedCast()
	}
	if typeTok.Type != lexer.IDENT {
		return nil, false, nil
	}
	if p.peekAt(2).Type != lexer.RPAREN {
		return nil, false, nil
	}
	lparen := p.advance() // "("
	p.advance()           // Тип
	p.advance()           // ")"
	value, err := p.parseUnary()
	if err != nil {
		return nil, false, diagnostics.Wrap(err, "после каста (%s) ожидалось выражение", typeTok.Value)
	}
	return &ast.CastExpr{Type: typeTok.Value, Value: value, Line: lparen.Line}, true, nil
}

// startsOperand - tok может начинать операнд выражения.
func startsOperand(tok lexer.Token) bool {
	switch tok.Type {
	case lexer.IDENT, lexer.INT, lexer.FLOAT, lexer.STRING, lexer.CHAR, lexer.LPAREN, lexer.LBRACKET, lexer.KwNew, lexer.KwThis:
		return true
	}
	return false
}

// pointerTypeName - имя Указателя<T> в касте "(Указатель<T>) x".
const pointerTypeName = "Указатель"

// tryParseNamedCast - "(Тип) значение", где Тип - не примитив:
// "(Указатель<T>) x", "(*T) x", "(ВнешняяСущность) x" (приведение между
// адресами), а также "(Пара) з" - ошибка с подсказкой "з как Пара" уже в
// semantic, когда тип известен. Каст - только когда Тип начинается с
// заглавной буквы и сразу за ")" на той же строке идёт операнд: "(*п).x" и
// "(х)" - скобки вокруг выражения. Операнд не может начинаться с "*" -
// "(*а) * б", "(Икс) * 2" - умножение (кроме "(Указатель<T>) *пп").
// Не каст - позиция парсера не меняется.
func (p *Parser) tryParseNamedCast() (ast.Expr, bool, error) {
	if !startsTypeName(p, 1) {
		return nil, false, nil
	}
	first := p.peekAt(1)
	explicitPointer := first.Type == lexer.IDENT && first.Value == pointerTypeName && p.peekAt(2).Type == lexer.LT
	save, saveGT := p.pos, p.pendingGT
	lparen := p.advance() // "("
	typeName, err := p.parseTypeName()
	if err != nil || p.cur().Type != lexer.RPAREN {
		p.pos, p.pendingGT = save, saveGT
		return nil, false, nil
	}
	rparen := p.advance() // ")"
	next := p.cur()
	operand := startsOperand(next) || next.Type == lexer.BITAND || (explicitPointer && next.Type == lexer.STAR)
	if !operand || next.Line != rparen.Line {
		p.pos, p.pendingGT = save, saveGT
		return nil, false, nil
	}
	value, err := p.parseUnary()
	if err != nil {
		return nil, false, diagnostics.Wrap(err, "после каста (%s) ожидалось выражение", typeName)
	}
	return &ast.CastExpr{Type: typeName, Value: value, Line: lparen.Line, Named: true}, true, nil
}

// startsTypeName - с токена на смещении off (после цепочки "*") начинается
// имя типа: IDENT с заглавной буквы.
func startsTypeName(p *Parser, off int) bool {
	for p.peekAt(off).Type == lexer.STAR {
		off++
	}
	tok := p.peekAt(off)
	return tok.Type == lexer.IDENT && tok.Value != "" && unicode.IsUpper([]rune(tok.Value)[0])
}
