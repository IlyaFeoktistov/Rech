// Package semantic производит internal/ir.Program из ast.Program (шаг 5.b,
// docs/ROADMAP.md) - проверка типов и разрешение имён на отдельном слое,
// а не внутри backend'а, как сейчас (internal/backend/cbackend делает это
// сам во время генерации C, шаг 1).
//
// Реализуется вертикальным срезом за срезом по конструкциям языка - см.
// internal/ir, package doc. Analyze возвращает понятную ошибку "не
// реализовано в IR" для всего, что ещё не перенесено - это не тихая
// деградация, а явная граница текущего покрытия.
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Analyze - первый вертикальный срез: функция без параметров и без типа
// результата, тело - только "вывести <строковый литерал>" (то же
// подмножество, что и шаг 0). Расширяется вместе с internal/backend/
// cbackend.GenerateFromIR подшаг за подшагом, шаг 5.
func Analyze(prog *ast.Program) (*ir.Program, error) {
	out := &ir.Program{}
	for _, fn := range prog.Functions {
		irFn, err := analyzeFunction(fn)
		if err != nil {
			return nil, fmt.Errorf("функция %q: %w", fn.Name, err)
		}
		out.Functions = append(out.Functions, irFn)
	}
	return out, nil
}

func analyzeFunction(fn *ast.FunctionDecl) (*ir.Function, error) {
	if len(fn.Params) > 0 {
		return nil, fmt.Errorf("параметры функций пока не реализованы в IR (шаг 5.b)")
	}
	if fn.ReturnType != "" {
		return nil, fmt.Errorf("тип результата функции пока не реализован в IR (шаг 5.b)")
	}
	body := make([]ir.Stmt, len(fn.Body))
	for i, s := range fn.Body {
		stmt, err := analyzeStmt(s)
		if err != nil {
			return nil, err
		}
		body[i] = stmt
	}
	return &ir.Function{Name: fn.Name, Body: body}, nil
}

func analyzeStmt(s ast.Stmt) (ir.Stmt, error) {
	switch s := s.(type) {
	case *ast.PrintStmt:
		value, err := analyzeExpr(s.Value)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		return &ir.PrintStmt{Value: value}, nil
	default:
		return nil, fmt.Errorf("инструкция %T пока не реализована в IR (шаг 5.b)", s)
	}
}

func analyzeExpr(e ast.Expr) (ir.Expr, error) {
	switch e := e.(type) {
	case *ast.StringLiteral:
		return &ir.StringLiteral{Value: e.Value}, nil
	default:
		return nil, fmt.Errorf("выражение %T пока не реализовано в IR (шаг 5.b)", e)
	}
}
