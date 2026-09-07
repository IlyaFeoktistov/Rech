package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
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
	if typeTok.Type != lexer.IDENT || !primitiveCastTypes[typeTok.Value] {
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
		return nil, false, fmt.Errorf("после каста (%s) ожидалось выражение: %w", typeTok.Value, err)
	}
	return &ast.CastExpr{Type: typeTok.Value, Value: value, Line: lparen.Line}, true, nil
}
