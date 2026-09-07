package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// analyzeTernaryExpr - "условие ? тогда : иначе" (docs/SPEC.md, тернарный
// условный оператор) - условие обязано быть Логическое (та же
// формулировка ошибки, что и у "если"/"пока"/"утверждать"), "тогда"/
// "иначе" - строго одного типа, без неявного расширения (тот же принцип,
// что и у элементов литерала списка, analyzeListLiteralExpr) - результат
// получает этот общий тип.
func analyzeTernaryExpr(sc *ctx, e *ast.TernaryExpr) (ir.Expr, error) {
	cond, err := analyzeExpr(sc, e.Cond)
	if err != nil {
		return nil, err
	}
	cond = maybeUnwrapNarrowed(sc, cond)
	if typ := ir.TypeOf(cond); typ != ir.TypeBool {
		return nil, fmt.Errorf("строка %d: условие тернарного оператора \"? :\" должно быть логическим, получено %s", e.Line, typ)
	}

	then, err := analyzeExpr(sc, e.Then)
	if err != nil {
		return nil, err
	}
	then = maybeUnwrapNarrowed(sc, then)

	elseVal, err := analyzeExpr(sc, e.Else)
	if err != nil {
		return nil, err
	}
	elseVal = maybeUnwrapNarrowed(sc, elseVal)

	thenType, elseType := ir.TypeOf(then), ir.TypeOf(elseVal)
	if thenType != elseType {
		return nil, fmt.Errorf("строка %d: ветви тернарного оператора \"? :\" должны быть одного типа - \"тогда\" %s, \"иначе\" %s", e.Line, thenType, elseType)
	}

	return &ir.TernaryExpr{Cond: cond, Then: then, Else: elseVal, Type: thenType}, nil
}
