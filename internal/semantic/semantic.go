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
	"slices"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// scope - символьная таблица одной области видимости: имя переменной ->
// тип.
type scope map[string]ir.Type

// scopeStack - стек областей видимости одной функции (шаг 2 - если/пока/
// для вводят вложенные блоки, как в cbackend, только здесь, а не в
// backend'е - в этом и смысл шага 5). declare добавляет переменную в
// самую внутреннюю область; lookup ищет от самой внутренней к внешним;
// затенение (повторное имя во вложенном блоке) разрешено - declare
// проверяет только текущую область, не внешние.
type scopeStack struct {
	scopes []scope
}

func newScopeStack() *scopeStack {
	return &scopeStack{scopes: []scope{{}}}
}

func (s *scopeStack) push() {
	s.scopes = append(s.scopes, scope{})
}

func (s *scopeStack) pop() {
	s.scopes = s.scopes[:len(s.scopes)-1]
}

func (s *scopeStack) declare(name string, typ ir.Type) error {
	top := s.scopes[len(s.scopes)-1]
	if _, exists := top[name]; exists {
		return fmt.Errorf("переменная %q уже объявлена в этой области видимости", name)
	}
	top[name] = typ
	return nil
}

func (s *scopeStack) lookup(name string) (ir.Type, bool) {
	for _, sc := range slices.Backward(s.scopes) {
		if typ, ok := sc[name]; ok {
			return typ, true
		}
	}
	return ir.TypeUnknown, false
}

// Analyze производит IR из AST. Покрытие на данный момент - шаг 0
// (`вывести <строка>`), шаг 1 (переменные, арифметика, сравнения,
// логические операторы) и шаг 2 (если/пока/для, блочная область
// видимости) - см. internal/ir, package doc. Расширяется вместе с
// internal/backend/cbackend.GenerateFromIR подшаг за подшагом, шаг 5.
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
	sc := newScopeStack()
	body, err := analyzeBlock(sc, fn.Body)
	if err != nil {
		return nil, err
	}
	return &ir.Function{Name: fn.Name, Body: body}, nil
}

// analyzeBlock анализирует список инструкций внутри уже открытой области
// видимости - вызывающий код сам решает, нужно ли sc.push()/sc.pop()
// вокруг вызова (тело функции - нет, тело если/пока/для - да).
func analyzeBlock(sc *scopeStack, stmts []ast.Stmt) ([]ir.Stmt, error) {
	body := make([]ir.Stmt, len(stmts))
	for i, s := range stmts {
		stmt, err := analyzeStmt(sc, s)
		if err != nil {
			return nil, err
		}
		body[i] = stmt
	}
	return body, nil
}

func analyzeStmt(sc *scopeStack, s ast.Stmt) (ir.Stmt, error) {
	switch s := s.(type) {
	case *ast.PrintStmt:
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		return &ir.PrintStmt{Value: value}, nil

	case *ast.VarDecl:
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		if err := sc.declare(s.Name, ir.TypeOf(value)); err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		return &ir.VarDecl{Name: s.Name, Value: value}, nil

	case *ast.Assign:
		declared, exists := sc.lookup(s.Name)
		if !exists {
			return nil, fmt.Errorf("строка %d: переменная %q не объявлена - используйте \"переменная\" для объявления", s.Line, s.Name)
		}
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		if typ := ir.TypeOf(value); typ != declared {
			return nil, fmt.Errorf("строка %d: переменной %q типа %s нельзя присвоить значение типа %s", s.Line, s.Name, declared, typ)
		}
		return &ir.Assign{Name: s.Name, Value: value}, nil

	case *ast.IfStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: условие \"если\" должно быть логическим, получено %s", s.Line, typ)
		}
		sc.push()
		then, err := analyzeBlock(sc, s.Then)
		sc.pop()
		if err != nil {
			return nil, err
		}
		var elseBody []ir.Stmt
		if s.Else != nil {
			sc.push()
			elseBody, err = analyzeBlock(sc, s.Else)
			sc.pop()
			if err != nil {
				return nil, err
			}
		}
		return &ir.IfStmt{Cond: cond, Then: then, Else: elseBody}, nil

	case *ast.WhileStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: условие \"пока\" должно быть логическим, получено %s", s.Line, typ)
		}
		sc.push()
		body, err := analyzeBlock(sc, s.Body)
		sc.pop()
		if err != nil {
			return nil, err
		}
		return &ir.WhileStmt{Cond: cond, Body: body}, nil

	case *ast.ForStmt:
		from, err := analyzeExpr(sc, s.From)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		if typ := ir.TypeOf(from); typ != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: граница диапазона после \"от\" должна быть целым числом, получено %s", s.Line, typ)
		}
		to, err := analyzeExpr(sc, s.To)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		if typ := ir.TypeOf(to); typ != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: граница диапазона после \"до\" должна быть целым числом, получено %s", s.Line, typ)
		}
		sc.push()
		if err := sc.declare(s.Var, ir.TypeInt); err != nil {
			sc.pop()
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		body, err := analyzeBlock(sc, s.Body)
		sc.pop()
		if err != nil {
			return nil, err
		}
		return &ir.ForStmt{Var: s.Var, From: from, To: to, Body: body}, nil

	case *ast.ReturnStmt:
		if s.Value != nil {
			return nil, fmt.Errorf("строка %d: возврат значения пока не реализован в IR (шаг 5.b) - функция без объявленного типа результата допускает только \"вернуть\" без значения", s.Line)
		}
		return &ir.ReturnStmt{}, nil

	default:
		return nil, fmt.Errorf("инструкция %T пока не реализована в IR (шаг 5.b)", s)
	}
}

func analyzeExpr(sc *scopeStack, e ast.Expr) (ir.Expr, error) {
	switch e := e.(type) {
	case *ast.StringLiteral:
		return &ir.StringLiteral{Value: e.Value}, nil

	case *ast.IntLiteral:
		return &ir.IntLiteral{Value: e.Value}, nil

	case *ast.FloatLiteral:
		return &ir.FloatLiteral{Value: e.Value}, nil

	case *ast.BoolLiteral:
		return &ir.BoolLiteral{Value: e.Value}, nil

	case *ast.Identifier:
		typ, exists := sc.lookup(e.Name)
		if !exists {
			return nil, fmt.Errorf("строка %d: переменная %q не объявлена", e.Line, e.Name)
		}
		return &ir.Identifier{Name: e.Name, Type: typ}, nil

	case *ast.UnaryExpr:
		return analyzeUnaryExpr(sc, e)

	case *ast.BinaryExpr:
		return analyzeBinaryExpr(sc, e)

	default:
		return nil, fmt.Errorf("выражение %T пока не реализовано в IR (шаг 5.b)", e)
	}
}

func analyzeUnaryExpr(sc *scopeStack, e *ast.UnaryExpr) (ir.Expr, error) {
	operand, err := analyzeExpr(sc, e.Operand)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(operand)
	switch e.Op {
	case "-":
		if typ != ir.TypeInt && typ != ir.TypeFloat {
			return nil, fmt.Errorf("строка %d: унарный \"-\" применим только к числам, получен %s", e.Line, typ)
		}
		return &ir.UnaryExpr{Op: e.Op, Operand: operand, Type: typ}, nil
	case "не":
		if typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: \"не\" применимо только к логическому значению, получен %s", e.Line, typ)
		}
		return &ir.UnaryExpr{Op: e.Op, Operand: operand, Type: ir.TypeBool}, nil
	default:
		return nil, fmt.Errorf("строка %d: неизвестная унарная операция %q", e.Line, e.Op)
	}
}

func analyzeBinaryExpr(sc *scopeStack, e *ast.BinaryExpr) (ir.Expr, error) {
	left, err := analyzeExpr(sc, e.Left)
	if err != nil {
		return nil, err
	}
	right, err := analyzeExpr(sc, e.Right)
	if err != nil {
		return nil, err
	}
	lt, rt := ir.TypeOf(left), ir.TypeOf(right)

	switch e.Op {
	case "+":
		if lt == ir.TypeString && rt == ir.TypeString {
			return &ir.BinaryExpr{Op: "+", Left: left, Right: right, Type: ir.TypeString}, nil
		}
		return analyzeArithmetic(e, left, lt, right, rt)

	case "-", "*", "/":
		return analyzeArithmetic(e, left, lt, right, rt)

	case "==", "!=":
		if lt == ir.TypeString && rt == ir.TypeString {
			return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil
		}
		if lt != rt || (lt != ir.TypeInt && lt != ir.TypeFloat && lt != ir.TypeBool) {
			return nil, fmt.Errorf("строка %d: оператор %q не поддерживается для типов %s и %s", e.Line, e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	case "<", ">", "<=", ">=":
		if lt != rt || (lt != ir.TypeInt && lt != ir.TypeFloat) {
			return nil, fmt.Errorf("строка %d: оператор %q применим только к числам одного типа, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	case "и", "или":
		if lt != ir.TypeBool || rt != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: оператор %q применим только к логическим значениям, получены %s и %s", e.Line, e.Op, lt, rt)
		}
		return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: ir.TypeBool}, nil

	default:
		return nil, fmt.Errorf("строка %d: неизвестная операция %q", e.Line, e.Op)
	}
}

// analyzeArithmetic - "+" (нестроковый)/"-"/"*"/"/": Целое+Целое -> Целое,
// иначе (хотя бы один операнд Число) -> Число - та же промоция, что и в
// cbackend.genArithmetic (шаг 1), только результат - тип в IR-узле, без
// самого приведения ((double)...) - приведение операнда, если нужно,
// решает backend при генерации кода конкретной платформы, glядя на Type
// каждого операнда и Type результата.
func analyzeArithmetic(e *ast.BinaryExpr, left ir.Expr, lt ir.Type, right ir.Expr, rt ir.Type) (ir.Expr, error) {
	if lt != ir.TypeInt && lt != ir.TypeFloat {
		return nil, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, e.Op, lt)
	}
	if rt != ir.TypeInt && rt != ir.TypeFloat {
		return nil, fmt.Errorf("строка %d: оператор %q не поддерживается для типа %s", e.Line, e.Op, rt)
	}
	result := ir.TypeInt
	if lt == ir.TypeFloat || rt == ir.TypeFloat {
		result = ir.TypeFloat
	}
	return &ir.BinaryExpr{Op: e.Op, Left: left, Right: right, Type: result}, nil
}
