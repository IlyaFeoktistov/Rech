package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// analyzeIndexChain - "л[i][j]", где "л[i]" - "?Коллекция" (безопасная
// индексация списка/словаря): Ничто протекает по цепочке - результат
// "л[i] != Ничто ? л[i][j] : Ничто", тип - "?T". "л[i]" вычисляется
// дважды, поэтому внутренняя часть цепочки обязана быть без побочных
// эффектов (имена, литералы, поля, индексы, арифметика).
func analyzeIndexChain(sc *ctx, e *ast.IndexExpr, inner ir.Expr) (ir.Expr, error) {
	if !sideEffectFree(e.Receiver) {
		return nil, errAtLine(e.Line, CodeSemanticOther, "цепочка индексов \"[i][j]\" вычисляет внутреннюю часть дважды - в ней не должно быть вызовов: вынесите её в переменную")
	}
	innerType := ir.TypeOf(inner)
	unwrapped := &ir.NullUnwrapExpr{Value: inner, Type: nullableInner(sc, innerType), Line: e.Line}
	then, err := analyzeIndexOn(sc, e, unwrapped, true)
	if err != nil {
		return nil, err
	}
	resultType := ir.TypeOf(then)
	if resultType.Kind != ir.KindNullable {
		resultType, err = resolveNullableTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, "?"+resultType.String())
		if err != nil {
			return nil, err
		}
		then = &ir.NullWrapExpr{Value: then, Type: resultType, Line: e.Line}
	}
	cond := &ir.NullCheckExpr{Value: inner, IsNot: true, Line: e.Line}
	return &ir.TernaryExpr{Cond: cond, Then: then, Else: &ir.NullLiteral{Type: resultType, Line: e.Line}, Type: resultType}, nil
}

// sideEffectFree - вычисление e ничего не меняет: его можно повторить.
func sideEffectFree(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Identifier, *ast.ThisExpr, *ast.IntLiteral, *ast.StringLiteral, *ast.FloatLiteral, *ast.BoolLiteral, *ast.CharLiteral:
		return true
	case *ast.FieldAccess:
		return sideEffectFree(v.Receiver)
	case *ast.IndexExpr:
		return sideEffectFree(v.Receiver) && sideEffectFree(v.Index)
	case *ast.BinaryExpr:
		return sideEffectFree(v.Left) && sideEffectFree(v.Right)
	case *ast.UnaryExpr:
		return sideEffectFree(v.Operand)
	default:
		return false
	}
}
