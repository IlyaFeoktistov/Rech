package parser

import "github.com/IlyaFeoktistov/Rech/internal/lexer"

// ContinuationOperators - типы токенов, вокруг которых перенос строки
// внутри одной инструкции незначим (docs/SPEC.md, раздел 2) - тот же
// набор, что реально принимают parseLeftAssoc/parseComparison (expr.go) и
// явные проверки "=" (stmt.go, parseVarDecl/parseAssign) - собраны сюда
// одним местом, а не только внутри самого разбора, потому что internal/
// format (rech fmt) тоже должен отступать строку-продолжение на
// дополнительный уровень, тем же критерием, каким парсер решает, что
// перенос вообще допустим (иначе оба места держали бы свою копию списка
// операторов, расходящуюся при следующем расширении грамматики). "|" -
// тот же токен BITOR, что и у побитового "|", используется и в списке
// значений одной ветки "когда" (switch.go) - отдельной записи не нужно.
var ContinuationOperators = map[lexer.TokenType]bool{
	lexer.KwOr: true, lexer.KwAnd: true, lexer.QUESTIONQUESTION: true,
	lexer.BITOR: true, lexer.BITXOR: true, lexer.BITAND: true,
	lexer.SHL: true, lexer.SHR: true,
	lexer.PLUS: true, lexer.MINUS: true,
	lexer.SLASH: true, lexer.PERCENT: true, lexer.STAR: true,
	lexer.EQ: true, lexer.NEQ: true, lexer.LT: true, lexer.GT: true, lexer.LE: true, lexer.GE: true,
	lexer.ASSIGN: true,
}

// ContinuationLeadExcluded - член ContinuationOperators, не распознаваемый
// переносом строки, если оказывается ПЕРВЫМ токеном строки (в отличие от
// последнего токена ПРЕДЫДУЩЕЙ строки, где он допустим как обычно) - "*" в
// начале строки всегда самостоятельная инструкция через указатель
// (разыменование, "*p = значение"), не продолжение умножения (см.
// parseLeftAssoc, expr.go).
var ContinuationLeadExcluded = map[lexer.TokenType]bool{
	lexer.STAR: true,
}
