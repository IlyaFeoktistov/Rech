package module

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
)

// builtinTypeBases - имена типов, встроенных в язык (не требуют ни
// объявления, ни импорта): примитивы, "Файл"/"Сокет" (непрозрачные
// дескрипторы), "Запрос"/"Ответ" (HTTP-клиент,
// internal/builtins.ReservedEntityNames), "Исключение" (база для
// "наследует" в любом файле без явного объявления, internal/semantic) и
// три generic-коллекции. У
// generic-инстанциации ("Список<Целое>") аргумент внутри "<...>" сейчас
// может быть только примитивом (internal/semantic.resolveGenericTypeName -
// "элементы встроенных коллекций пока поддерживаются только для
// Строка/Целое/Действительное/Логическое, не для сущностей"), поэтому у
// любой generic-инстанциации, которую вообще можно написать, база уже
// входит сюда - resolveTypeRef ниже этим пользуется, чтобы не резолвить
// аргумент отдельно.
var builtinTypeBases = map[string]bool{
	"Целое":          true,
	"Действительное": true,
	"Дробное":        true,
	"Символ":         true,
	"Байт":           true,
	"Строка":         true,
	"Логическое":     true,
	"Файл":           true,
	"Сокет":          true,
	"Запрос":         true,
	"Ответ":          true,
	"Исключение":     true,
	"Список":         true,
	"Стопка":         true,
	"Очередь":        true,
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
	if builtins.IsReservedName(name) {
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
// имя типа) в дереве по names - см. link.go, buildNames. typeParams - имена
// параметров типа generic-сущности/функции, чьи поля/параметры/тип
// результата сейчас переписываются (шаг 16) - локальные заглушки ("T"), не
// ссылки на объявленный где-то тип, поэтому исключены из резолюции;
// выставляется/сбрасывается вызывающим кодом (link.go) перед/после каждого
// generic-объявления, nil - вне такого объявления (обычная резолюция).
type rewriter struct {
	names      map[string]string
	typeParams map[string]bool
}

// typeParamSet - имена параметров типа params в виде множества, для
// rewriter.typeParams - nil, если params пуст (не generic-объявление).
func typeParamSet(params []ast.GenericParam) map[string]bool {
	if len(params) == 0 {
		return nil
	}
	set := make(map[string]bool, len(params))
	for _, p := range params {
		set[p.Name] = true
	}
	return set
}

// resolveType - resolveTypeRef, но пропускающий типы, равные одному из
// typeParams как есть (см. package doc rewriter).
func (r *rewriter) resolveType(typeName string) (string, error) {
	if r.typeParams[typeName] {
		return typeName, nil
	}
	return resolveTypeRef(typeName, r.names)
}

func (r *rewriter) rewriteParams(params []ast.Param) error {
	for i, p := range params {
		resolved, err := r.resolveType(p.Type)
		if err != nil {
			return fmt.Errorf("параметр %q: %w", p.Name, err)
		}
		params[i].Type = resolved
	}
	return nil
}

func (r *rewriter) rewriteFields(fields []ast.Field) error {
	for i, f := range fields {
		resolved, err := r.resolveType(f.Type)
		if err != nil {
			return fmt.Errorf("поле %q: %w", f.Name, err)
		}
		fields[i].Type = resolved
	}
	return nil
}

func (r *rewriter) rewriteReturnType(returnType string) (string, error) {
	return r.resolveType(returnType)
}

// rewriteTypeParams переписывает имена типов внутри ограничений параметров
// типа generic-сущности/функции (шаг 16) - сами имена параметров (T, U)
// НЕ переписываются (это не ссылки на объявленные где-то типы, а
// локальные заглушки, вводимые прямо здесь).
func (r *rewriter) rewriteTypeParams(params []ast.GenericParam) error {
	for i, p := range params {
		for j, c := range p.Constraints {
			resolved, err := resolveTypeRef(c, r.names)
			if err != nil {
				return fmt.Errorf("параметр типа %q: %w", p.Name, err)
			}
			params[i].Constraints[j] = resolved
		}
	}
	return nil
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
	case *ast.ShowStmt:
		return r.rewriteExpr(s.Value)
	case *ast.VarDecl:
		return r.rewriteExpr(s.Value)
	case *ast.ConstDecl:
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
		if s.Await != nil {
			return r.rewriteExpr(s.Await)
		}
		return nil
	case *ast.ThrowStmt:
		return r.rewriteExpr(s.Value)
	case *ast.TryStmt:
		if err := r.rewriteStmts(s.Body); err != nil {
			return err
		}
		for i, t := range s.CatchTypes {
			resolved, err := resolveTypeRef(t, r.names)
			if err != nil {
				return fmt.Errorf("\"поймать(...)\": %w", err)
			}
			s.CatchTypes[i] = resolved
		}
		return r.rewriteStmts(s.CatchBody)
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
	case *ast.StringLiteral, *ast.IntLiteral, *ast.FloatLiteral, *ast.BoolLiteral, *ast.CharLiteral, *ast.Identifier, *ast.ThisExpr, *ast.IncDecExpr:
		return nil
	case *ast.CallExpr:
		resolved, err := resolveBareName(e.Callee, r.names)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.Callee = resolved
		for i, t := range e.TypeArgs {
			resolvedType, err := resolveTypeRef(t, r.names)
			if err != nil {
				return fmt.Errorf("строка %d: аргумент типа: %w", e.Line, err)
			}
			e.TypeArgs[i] = resolvedType
		}
		return r.rewriteExprs(e.Args)
	case *ast.NewExpr:
		resolved, err := r.resolveType(e.Type)
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
	case *ast.CastExpr:
		return r.rewriteExpr(e.Value)
	case *ast.IsExpr:
		resolved, err := r.resolveType(e.Target)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.Target = resolved
		return r.rewriteExpr(e.Value)
	case *ast.LaunchExpr:
		return r.rewriteExpr(e.Call)
	case *ast.AwaitExpr:
		return r.rewriteExpr(e.Task)
	default:
		return fmt.Errorf("внутренняя ошибка: неизвестное выражение %T", e)
	}
}
