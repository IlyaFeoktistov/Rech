package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// analyzeCallResultCall - "выражение(<арг>, ...)", где выражение - результат
// вызова/индексации типа "Вызываемый" ("сп.получить(0)(3)"); результат
// может быть пустым (вызов как инструкция).
func analyzeCallResultCall(sc *ctx, e *ast.CallValueExpr) (*ir.CallValueExpr, error) {
	callee, err := analyzeExpr(sc, e.Callee)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(callee)
	if typ.Kind == ir.KindNullable && nullableInner(sc, typ).Kind == ir.KindCallable {
		return nil, errAtLine(e.Line, CodeNullable, "результат выражения имеет тип %s и может быть Ничто - вызывать можно только непустое значение (сохраните его в переменную и проверьте \"если ф != Ничто\")", typ)
	}
	if typ.Kind != ir.KindCallable {
		return nil, errAtLine(e.Line, CodeExprCheck, "результат выражения имеет тип %s - вызвать можно только значение типа \"Вызываемый\"", typ)
	}
	return analyzeCallValueExpr(sc, callee, &ast.CallExpr{Args: e.Args, Line: e.Line})
}

// analyzeCallResultExpr - то же внутри выражения: результат обязан быть.
func analyzeCallResultExpr(sc *ctx, e *ast.CallValueExpr) (ir.Expr, error) {
	call, err := analyzeCallResultCall(sc, e)
	if err != nil {
		return nil, err
	}
	if !call.HasReturn {
		return nil, errAtLine(e.Line, CodeExprCheck, "вызов %s не имеет результата - его нельзя использовать как выражение", ir.TypeOf(call.Value))
	}
	return call, nil
}
