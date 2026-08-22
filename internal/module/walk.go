package module

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
)

// builtinTypeBases - имена типов, встроенных в язык (не требуют ни
// объявления, ни импорта): четыре примитива и три generic-коллекции. У
// generic-инстанциации ("Список<Целое>") аргумент внутри "<...>" сейчас
// может быть только примитивом (internal/backend/cbackend.resolveType -
// "элементы встроенных коллекций пока поддерживаются только для
// Строка/Целое/Число/Логическое, не для сущностей"), поэтому у любой
// generic-инстанциации, которую вообще можно написать, база уже входит
// сюда - resolveTypeRef ниже этим пользуется, чтобы не резолвить аргумент
// отдельно.
var builtinTypeBases = map[string]bool{
	"Целое":      true,
	"Число":      true,
	"Строка":     true,
	"Логическое": true,
	"Список":     true,
	"Стопка":     true,
	"Очередь":    true,
}

// resolveBareName ищет name (имя функции или простого - не generic - типа)
// в names (карта "как написано в файле" -> "итоговое, возможно
// переименованное имя", см. link.go) - встроенные имена (типы из
// builtinTypeBases, функции шага 8 из internal/builtins - ввод консоли,
// математика) возвращаются как есть: они не объявлены ни в одном файле
// Rech-программы и потому не участвуют в переименовании при линковке.
// Не найденное имя - ошибка ("не объявлено и не импортировано").
func resolveBareName(name string, names map[string]string) (string, error) {
	if builtinTypeBases[name] {
		return name, nil
	}
	if _, ok := builtins.ByName(name); ok {
		return name, nil
	}
	resolved, ok := names[name]
	if !ok {
		return "", fmt.Errorf("%q не объявлено в этом файле и не импортировано", name)
	}
	return resolved, nil
}

// resolveTypeRef - то же самое для имени типа, включая generic-инстанциации
// ("Список<Целое>") - см. builtinTypeBases про то, почему аргумент внутри
// "<...>" не нужно резолвить отдельно.
func resolveTypeRef(typeName string, names map[string]string) (string, error) {
	if typeName == "" || strings.Contains(typeName, "<") {
		return typeName, nil
	}
	return resolveBareName(typeName, names)
}

// rewriter переписывает каждую ссылку на глобальное имя (вызов функции,
// имя типа) в дереве по names - см. link.go, buildNames.
type rewriter struct {
	names map[string]string
}

func (r *rewriter) rewriteParams(params []ast.Param) error {
	for i, p := range params {
		resolved, err := resolveTypeRef(p.Type, r.names)
		if err != nil {
			return fmt.Errorf("параметр %q: %w", p.Name, err)
		}
		params[i].Type = resolved
	}
	return nil
}

func (r *rewriter) rewriteFields(fields []ast.Field) error {
	for i, f := range fields {
		resolved, err := resolveTypeRef(f.Type, r.names)
		if err != nil {
			return fmt.Errorf("поле %q: %w", f.Name, err)
		}
		fields[i].Type = resolved
	}
	return nil
}

func (r *rewriter) rewriteReturnType(returnType string) (string, error) {
	return resolveTypeRef(returnType, r.names)
}

func (r *rewriter) rewriteStmts(stmts []ast.Stmt) error {
	for _, s := range stmts {
		if err := r.rewriteStmt(s); err != nil {
			return err
		}
	}
	return nil
}

func (r *rewriter) rewriteStmt(s ast.Stmt) error {
	switch s := s.(type) {
	case *ast.PrintStmt:
		return r.rewriteExpr(s.Value)
	case *ast.VarDecl:
		return r.rewriteExpr(s.Value)
	case *ast.Assign:
		return r.rewriteExpr(s.Value)
	case *ast.FieldAssign:
		if err := r.rewriteExpr(s.Receiver); err != nil {
			return err
		}
		return r.rewriteExpr(s.Value)
	case *ast.BecomeStmt:
		return r.rewriteExpr(s.Receiver)
	case *ast.AssertStmt:
		return r.rewriteExpr(s.Cond)
	case *ast.IfStmt:
		if err := r.rewriteExpr(s.Cond); err != nil {
			return err
		}
		if err := r.rewriteStmts(s.Then); err != nil {
			return err
		}
		return r.rewriteStmts(s.Else)
	case *ast.WhileStmt:
		if err := r.rewriteExpr(s.Cond); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.ForStmt:
		if err := r.rewriteExpr(s.From); err != nil {
			return err
		}
		if err := r.rewriteExpr(s.To); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.ForEachStmt:
		if err := r.rewriteExpr(s.Collection); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.ReturnStmt:
		if s.Value == nil {
			return nil
		}
		return r.rewriteExpr(s.Value)
	case *ast.ExprStmt:
		if s.Call != nil {
			return r.rewriteExpr(s.Call)
		}
		if s.MethodCall != nil {
			return r.rewriteExpr(s.MethodCall)
		}
		return nil
	default:
		return fmt.Errorf("внутренняя ошибка: неизвестная инструкция %T", s)
	}
}

func (r *rewriter) rewriteExprs(exprs []ast.Expr) error {
	for _, e := range exprs {
		if err := r.rewriteExpr(e); err != nil {
			return err
		}
	}
	return nil
}

func (r *rewriter) rewriteExpr(e ast.Expr) error {
	switch e := e.(type) {
	case *ast.StringLiteral, *ast.IntLiteral, *ast.FloatLiteral, *ast.BoolLiteral, *ast.Identifier, *ast.ThisExpr, *ast.IncDecExpr:
		return nil
	case *ast.CallExpr:
		resolved, err := resolveBareName(e.Callee, r.names)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.Callee = resolved
		return r.rewriteExprs(e.Args)
	case *ast.NewExpr:
		resolved, err := resolveTypeRef(e.Type, r.names)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.Type = resolved
		return r.rewriteExprs(e.Args)
	case *ast.FieldAccess:
		return r.rewriteExpr(e.Receiver)
	case *ast.MethodCallExpr:
		if err := r.rewriteExpr(e.Receiver); err != nil {
			return err
		}
		return r.rewriteExprs(e.Args)
	case *ast.BinaryExpr:
		if err := r.rewriteExpr(e.Left); err != nil {
			return err
		}
		return r.rewriteExpr(e.Right)
	case *ast.UnaryExpr:
		return r.rewriteExpr(e.Operand)
	default:
		return fmt.Errorf("внутренняя ошибка: неизвестное выражение %T", e)
	}
}
