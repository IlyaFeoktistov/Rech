package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Вариативная C-функция ("внешняя функция printf(формат: Строка, ...)"):
// аргументы сверх объявленных параметров - каждый своего типа, без
// приведения к общему. Расширение по правилам C (Дробное -> double, целые
// уже int -> int) делают backend'ы.

// analyzeCVariadicArgs - аргументы extra после объявленных параметров
// функции callee; первый из них - аргумент номер first+1.
func analyzeCVariadicArgs(sc *ctx, extra []ast.Expr, first, line int, argLabel string) ([]ir.Expr, error) {
	out := make([]ir.Expr, len(extra))
	for i, e := range extra {
		arg, err := analyzeExpr(sc, e)
		if err != nil {
			return nil, err
		}
		if t := ir.TypeOf(arg); !cVariadicArgAllowed(t) {
			return nil, errAtLine(line, CodeExprCheck, "аргумент %d %s имеет тип %s - на месте \"...\" C-функции допустимы числа, Логическое, Байт, Символ, Строка, указатель и внешняя сущность", first+i+1, argLabel, t)
		}
		out[i] = arg
	}
	return out, nil
}

func cVariadicArgAllowed(t ir.Type) bool {
	return isFFIScalar(t) || t.Kind == ir.KindExternEntity || t.Kind == ir.KindPointer
}
