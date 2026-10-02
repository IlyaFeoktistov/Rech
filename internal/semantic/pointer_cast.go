package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// analyzeNamedCast - "(Тип) x" с не-примитивным Тип (ast.CastExpr.Named):
// приведение есть только между адресами (pointerCastAllowed); "(Пара) з"
// у "Значение" - распаковка, она пишется через "как".
func analyzeNamedCast(sc *ctx, e *ast.CastExpr, value ir.Expr, from, to ir.Type) (ir.Expr, error) {
	if pointerCastAllowed(from, to) {
		return &ir.CastExpr{Value: value, To: to}, nil
	}
	if to.Kind == ir.KindPointer || to.Kind == ir.KindExternEntity {
		return nil, errAtLine(e.Line, CodeTypeMismatch, "нельзя привести %s к %s - к указателю и внешней сущности приводятся только указатель, Целое/Натуральное (адрес) и внешняя сущность", from, to)
	}
	operand := "значение"
	if id, ok := e.Value.(*ast.Identifier); ok {
		operand = id.Name
	}
	return nil, errAtLine(e.Line, CodeTypeMismatch, "приведение \"(%s) значение\" есть только у примитивов, указателей и внешних сущностей - распаковать \"Значение\" в %s можно через \"как\"", e.Type, e.Type).
		WithHint("%s как %s", operand, e.Type)
}

// pointerCastAllowed - хотя бы одна сторона - Указатель<T>, другая - тоже
// адрес: Указатель<U> (другой тип элемента, "void *" -> "T *" в C),
// Целое/Натуральное (адрес числом, intptr_t/uintptr_t) или внешняя
// сущность (непрозрачный "void *").
func pointerCastAllowed(from, to ir.Type) bool {
	if from.Kind != ir.KindPointer && to.Kind != ir.KindPointer {
		return false
	}
	return isAddressType(from) && isAddressType(to)
}

func isAddressType(t ir.Type) bool {
	return t.Kind == ir.KindPointer || t.Kind == ir.KindExternEntity || t == ir.TypeInt || t == ir.TypeUint
}
