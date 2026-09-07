// Поиск MethodCallExpr по (строка, имя метода) в уже разобранном IR -
// нужен implementation.go для случая, когда получатель вызова - результат
// ДРУГОГО вызова (`контейнер.логгер().инфо(...)`), а не простое имя
// переменной/поля - chainBefore/resolveChain (resolve.go) такую цепочку не
// строят вовсе (там курсор должен стоять на самом методе/поле, получатель -
// не результат вызова, см. package doc resolveChain). IR же уже несёт
// разрешённый Type у получателя ЛЮБОЙ формы (в т. ч. результата вызова) -
// достаточно найти ТОТ САМЫЙ узел по (строка инструкции, имя метода) и
// прочитать тип его Receiver напрямую, вместо повторного разбора цепочки
// вручную.
package lsp

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// findMethodCallOnLine ищет первый *ir.MethodCallExpr с Method == name
// внутри инструкции с ir.LineOf(s) == targetLine (рекурсивно спускается во
// вложенные блоки/тела в поисках этой строки). Best-effort - покрывает
// частые места выражения (значение print/var/return/assign/условия,
// получатель/аргументы вызовов), не абсолютно ВСЕ теоретически возможные
// (лямбды с захватом, generic-вызовы и т. п.) - тот же принцип приближения,
// что и у references.go.
func findMethodCallOnLine(stmts []ir.Stmt, targetLine int, name string) (*ir.MethodCallExpr, bool) {
	for _, s := range stmts {
		if ir.LineOf(s) == targetLine {
			for _, e := range stmtExprRoots(s) {
				if m, ok := findMethodCallInExpr(e, name); ok {
					return m, true
				}
			}
		}
		for _, body := range stmtChildBodies(s) {
			if m, ok := findMethodCallOnLine(body, targetLine, name); ok {
				return m, true
			}
		}
	}
	return nil, false
}

// stmtExprRoots - выражения, с которых начинается поиск внутри одной
// инструкции (см. package doc).
func stmtExprRoots(s ir.Stmt) []ir.Expr {
	switch s := s.(type) {
	case *ir.PrintStmt:
		return []ir.Expr{s.Value}
	case *ir.ShowStmt:
		return []ir.Expr{s.Value}
	case *ir.AssertStmt:
		return []ir.Expr{s.Cond}
	case *ir.VarDecl:
		return []ir.Expr{s.Value}
	case *ir.Assign:
		return []ir.Expr{s.Value}
	case *ir.FieldAssign:
		return []ir.Expr{s.Receiver, s.Value}
	case *ir.StaticFieldAssign:
		return []ir.Expr{s.Value}
	case *ir.ReturnStmt:
		if s.Value != nil {
			return []ir.Expr{s.Value}
		}
	case *ir.IfStmt:
		return []ir.Expr{s.Cond}
	case *ir.WhileStmt:
		return []ir.Expr{s.Cond}
	case *ir.CStyleForStmt:
		return []ir.Expr{s.Cond}
	case *ir.ExprStmt:
		switch {
		case s.Call != nil:
			return []ir.Expr{s.Call}
		case s.CallValue != nil:
			return []ir.Expr{s.CallValue}
		case s.MethodCall != nil:
			return []ir.Expr{s.MethodCall}
		case s.StaticMethodCall != nil:
			return []ir.Expr{s.StaticMethodCall}
		case s.Await != nil:
			return []ir.Expr{s.Await}
		}
	}
	return nil
}

// stmtChildBodies - вложенные списки инструкций (если/пока/для/попытаться/
// деепричастие/блок), в которые рекурсия спускается независимо от
// собственной строки родителя.
func stmtChildBodies(s ir.Stmt) [][]ir.Stmt {
	switch s := s.(type) {
	case *ir.IfStmt:
		return [][]ir.Stmt{s.Then, s.Else}
	case *ir.WhileStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.ForStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.ForEachStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.CStyleForStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.TryStmt:
		return [][]ir.Stmt{s.Body, s.CatchBody}
	case *ir.GerundStmt:
		return [][]ir.Stmt{s.Body}
	case *ir.BlockStmt:
		return [][]ir.Stmt{s.Body}
	}
	return nil
}

// findMethodCallInExpr обходит дерево выражения в поисках MethodCallExpr с
// Method == name - см. package doc про неполноту охвата видов выражений.
func findMethodCallInExpr(e ir.Expr, name string) (*ir.MethodCallExpr, bool) {
	switch e := e.(type) {
	case nil:
		return nil, false
	case *ir.MethodCallExpr:
		if e.Method == name {
			return e, true
		}
		if m, ok := findMethodCallInExpr(e.Receiver, name); ok {
			return m, true
		}
		return findMethodCallInExprList(e.Args, name)
	case *ir.CallExpr:
		return findMethodCallInExprList(e.Args, name)
	case *ir.StaticMethodCallExpr:
		return findMethodCallInExprList(e.Args, name)
	case *ir.BinaryExpr:
		if m, ok := findMethodCallInExpr(e.Left, name); ok {
			return m, true
		}
		return findMethodCallInExpr(e.Right, name)
	case *ir.UnaryExpr:
		return findMethodCallInExpr(e.Operand, name)
	case *ir.CastExpr:
		return findMethodCallInExpr(e.Value, name)
	case *ir.FieldAccess:
		return findMethodCallInExpr(e.Receiver, name)
	case *ir.NewExpr:
		return findMethodCallInExprList(e.Args, name)
	}
	return nil, false
}

func findMethodCallInExprList(list []ir.Expr, name string) (*ir.MethodCallExpr, bool) {
	for _, e := range list {
		if m, ok := findMethodCallInExpr(e, name); ok {
			return m, true
		}
	}
	return nil, false
}
