package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Запись поля элемента коллекции структур - "сп[i].поле = значение" и
// "сп.получить(i).поле = значение" (то же для Словарь по ключу) меняют
// САМ элемент: структура - значение, и без этого запись ушла бы во
// временную копию элемента и потерялась. Разворачивается в
//
//	{ индекс = i; элемент = сп.получить(индекс); элемент.поле = значение;
//	  сп.установить(индекс, элемент) }
//
// Выход индекса за границы/нет ключа - паника, как у "получить".

// elementFieldTarget - s.Receiver - элемент коллекции структур: коллекция
// и индекс/ключ. ok == false - другой получатель.
func elementFieldTarget(sc *ctx, recv ast.Expr) (coll, index ast.Expr, ok bool) {
	switch r := recv.(type) {
	case *ast.IndexExpr:
		coll, index = r.Receiver, r.Index
	case *ast.MethodCallExpr:
		if r.Method != "получить" || len(r.Args) != 1 || r.Static {
			return nil, nil, false
		}
		coll, index = r.Receiver, r.Args[0]
	default:
		return nil, nil, false
	}
	collIR, err := analyzeExpr(sc, coll)
	if err != nil {
		return nil, nil, false
	}
	sig, isEnt := sc.entities[ir.TypeOf(collIR).Entity]
	if !isEnt || !(sig.IsList || sig.IsDict) {
		return nil, nil, false
	}
	elem, isElemEnt := sc.entities[sig.ElemType.Entity]
	if sig.ElemType.Kind != ir.KindEntity || !isElemEnt || !elem.IsStruct {
		return nil, nil, false
	}
	return coll, index, true
}

// analyzeElementFieldAssign - развёртка (см. package doc выше). Коллекция
// обязана быть переменной/полем: её выражение вычисляется дважды
// (получить/установить).
func analyzeElementFieldAssign(sc *ctx, s *ast.FieldAssign, coll, index ast.Expr) (ir.Stmt, error) {
	if !receiverAddressable(coll) {
		return nil, errAtLine(s.Line, CodeStmtCheck, "запись поля %q элемента коллекции - коллекция должна быть переменной или полем, не результатом вызова", s.Field)
	}
	sc.a.elementAssignSeq++
	idxName := fmt.Sprintf("__индекс_%d", sc.a.elementAssignSeq)
	elemName := fmt.Sprintf("__элемент_%d", sc.a.elementAssignSeq)
	idxID := &ast.Identifier{Name: idxName, Line: s.Line}
	elemID := &ast.Identifier{Name: elemName, Line: s.Line}
	value := s.Value
	// "сп[i].поле += 1"/"сп[i].поле++" - парсер строит значение
	// "<тот же получатель>.поле + 1": читаем поле временного элемента.
	if bin, ok := value.(*ast.BinaryExpr); ok {
		if fa, ok := bin.Left.(*ast.FieldAccess); ok && fa.Receiver == s.Receiver {
			copied := *bin
			copied.Left = &ast.FieldAccess{Receiver: elemID, Field: fa.Field, Line: fa.Line}
			value = &copied
		}
	}
	stmts := []ast.Stmt{
		&ast.VarDecl{Name: idxName, Value: index, Line: s.Line},
		&ast.VarDecl{Name: elemName, Value: &ast.MethodCallExpr{Receiver: coll, Method: "получить", Args: []ast.Expr{idxID}, Line: s.Line}, Line: s.Line},
		&ast.FieldAssign{Receiver: elemID, Field: s.Field, Value: value, Line: s.Line},
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{Receiver: coll, Method: "установить", Args: []ast.Expr{idxID, elemID}, Line: s.Line}, Line: s.Line},
	}
	sc.push()
	defer sc.pop()
	body := make([]ir.Stmt, 0, len(stmts))
	for _, st := range stmts {
		out, err := analyzeStmt(sc, st)
		if err != nil {
			return nil, err
		}
		body = append(body, out)
	}
	return &ir.BlockStmt{Body: body, Line: s.Line}, nil
}

// isCallResult - e - результат вызова (функции, метода, значения) или
// индексации: у структуры это копия.
func isCallResult(e ast.Expr) bool {
	switch e.(type) {
	case *ast.CallExpr, *ast.MethodCallExpr, *ast.CallValueExpr, *ast.IndexExpr:
		return true
	}
	return false
}
