package parser

import (
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// overloadableOperatorSymbols - бинарные операторы, для которых сущность/
// структура может объявить перегрузку ("статический публичный оператор
// +(...)", см. package doc parseMethod в entity.go) - арифметика,
// сравнение и равенство. Побитовые операторы ("&"/"|"/"^"/"<<"/">>") и
// логические ("и"/"или") сознательно не входят - в языке они и так
// применимы только к Целое/Логическое соответственно (internal/semantic/
// expr.go, analyzeBinaryExpr), перегрузка для пользовательских типов не
// запрашивалась.
var overloadableOperatorSymbols = map[lexer.TokenType]string{
	lexer.PLUS:    "+",
	lexer.MINUS:   "-",
	lexer.STAR:    "*",
	lexer.SLASH:   "/",
	lexer.PERCENT: "%",
	lexer.EQ:      "==",
	lexer.NEQ:     "!=",
	lexer.LT:      "<",
	lexer.GT:      ">",
	lexer.LE:      "<=",
	lexer.GE:      ">=",
}

// overloadableOperatorSymbolsList - все допустимые символы через "/", для
// текста ошибки "после \"оператор\" ожидался один из ...".
func overloadableOperatorSymbolsList() string {
	symbols := make([]string, 0, len(overloadableOperatorSymbols))
	for _, s := range overloadableOperatorSymbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)
	return strings.Join(symbols, "/")
}
