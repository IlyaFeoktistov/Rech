package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// maybeDecimalMethod - методы "Десятичное": "д.округлить(знаков = 0)" -
// банковское округление до знаков после запятой. handled=false - не
// Десятичное или не его метод.
func maybeDecimalMethod(sc *ctx, e *ast.MethodCallExpr) (ir.Expr, bool, error) {
	if e.Method != "округлить" {
		return nil, false, nil
	}
	receiver, err := analyzeExpr(sc, e.Receiver)
	if err != nil {
		return nil, false, nil
	}
	receiver = maybeUnwrapNarrowed(sc, receiver)
	if ir.TypeOf(receiver) != ir.TypeDecimal {
		return nil, false, nil
	}
	if len(e.Args) > 1 {
		return nil, true, errAtLine(e.Line, CodeExprCheck, "\"округлить\" у Десятичное принимает не больше 1 аргумента (знаков после запятой), получено %d", len(e.Args))
	}
	var places ir.Expr = &ir.IntLiteral{Value: 0}
	if len(e.Args) == 1 {
		places, err = coerceExpr(sc, e.Args[0], ir.TypeInt)
		if err != nil {
			return nil, true, err
		}
		if t := ir.TypeOf(places); t != ir.TypeInt {
			return nil, true, errAtLine(e.Line, CodeExprCheck, "\"округлить\": число знаков должно быть Целое, получено %s", t)
		}
	}
	return &ir.DecimalRoundExpr{Value: receiver, Places: places}, true, nil
}
