package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Запись поля элемента коллекции структур - "сп[i].поле = значение",
// "м[i][j][k].поле = значение", "сп.получить(i).поле = значение" (то же
// для Словарь по ключу) - меняет САМ элемент: структура - значение, и без
// этого запись ушла бы во временную копию элемента и потерялась.
// Разворачивается в
//
//	{ и1 = i; э1 = м.получить(и1); и2 = j; э2 = э1.получить(и2); ...
//	  эN.поле = значение; э(N-1).установить(иN, эN); ...; м.установить(и1, э1) }
//
// Выход индекса за границы/нет ключа - паника, как у "получить".

// elementLevel - один уровень цепочки: коллекция и индекс/ключ в ней.
type elementLevel struct {
	coll, index ast.Expr
}

// elementFieldTarget - recv - элемент-структура (возможно вложенной)
// коллекции: база цепочки (переменная/поле/результат вызова) и уровни от
// внешнего к внутреннему. ok == false - другой получатель.
func elementFieldTarget(sc *ctx, recv ast.Expr) (base ast.Expr, levels []elementLevel, ok bool) {
	cur := recv
	for {
		switch r := cur.(type) {
		case *ast.IndexExpr:
			levels = append([]elementLevel{{r.Receiver, r.Index}}, levels...)
			cur = r.Receiver
			continue
		case *ast.MethodCallExpr:
			if r.Method == "получить" && len(r.Args) == 1 && !r.Static {
				levels = append([]elementLevel{{r.Receiver, r.Args[0]}}, levels...)
				cur = r.Receiver
				continue
			}
		}
		break
	}
	if len(levels) == 0 {
		return nil, nil, false
	}
	baseIR, err := analyzeExpr(sc, cur)
	if err != nil {
		return nil, nil, false
	}
	typ := ir.TypeOf(baseIR)
	for range levels {
		if typ.Kind == ir.KindPointer || typ.Kind == ir.KindArray {
			typ = sc.entities[typ.Entity].ElemType
			continue
		}
		sig, isEnt := sc.entities[typ.Entity]
		if typ.Kind != ir.KindEntity || !isEnt || !(sig.IsList || sig.IsDict) {
			return nil, nil, false
		}
		typ = sig.ElemType
	}
	elem, isElemEnt := sc.entities[typ.Entity]
	if typ.Kind != ir.KindEntity || !isElemEnt || !elem.IsStruct {
		return nil, nil, false
	}
	return cur, levels, true
}

// analyzeElementFieldAssign - развёртка (см. package doc выше). База
// цепочки обязана быть переменной/полем: её выражение вычисляется дважды
// (получить/установить).
func analyzeElementFieldAssign(sc *ctx, s *ast.FieldAssign, base ast.Expr, levels []elementLevel) (ir.Stmt, error) {
	if !receiverAddressable(base) {
		return nil, errAtLine(s.Line, CodeStmtCheck, "запись поля %q элемента коллекции - коллекция должна быть переменной или полем, не результатом вызова", s.Field)
	}
	var stmts, writeBack []ast.Stmt
	var coll = base
	var elemID *ast.Identifier
	for _, lv := range levels {
		sc.a.elementAssignSeq++
		idxID := &ast.Identifier{Name: fmt.Sprintf("__индекс_%d", sc.a.elementAssignSeq), Line: s.Line}
		elemID = &ast.Identifier{Name: fmt.Sprintf("__элемент_%d", sc.a.elementAssignSeq), Line: s.Line}
		stmts = append(stmts,
			&ast.VarDecl{Name: idxID.Name, Value: lv.index, Line: s.Line},
			&ast.VarDecl{Name: elemID.Name, Value: &ast.MethodCallExpr{Receiver: coll, Method: "получить", Args: []ast.Expr{idxID}, Line: s.Line}, Line: s.Line})
		writeBack = append([]ast.Stmt{&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{Receiver: coll, Method: "установить", Args: []ast.Expr{idxID, elemID}, Line: s.Line}, Line: s.Line}}, writeBack...)
		coll = elemID
	}
	value := s.Value
	// "м[i].поле += 1"/"м[i].поле++" - парсер строит значение
	// "<тот же получатель>.поле + 1": читаем поле временного элемента.
	if bin, ok := value.(*ast.BinaryExpr); ok {
		if fa, ok := bin.Left.(*ast.FieldAccess); ok && fa.Receiver == s.Receiver {
			copied := *bin
			copied.Left = &ast.FieldAccess{Receiver: elemID, Field: fa.Field, Line: fa.Line}
			value = &copied
		}
	}
	stmts = append(stmts, &ast.FieldAssign{Receiver: elemID, Field: s.Field, Value: value, Line: s.Line})
	stmts = append(stmts, writeBack...)
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
