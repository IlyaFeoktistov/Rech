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

// funcSig - сигнатура функции, собранная предварительным проходом
// (collectFuncSigs) до анализа тел - функции могут вызывать друг друга
// независимо от порядка объявления в файле (включая взаимную рекурсию),
// как и в AST-пути (cbackend.funcSig, тот же принцип, отдельный тип - IR
// не должен знать про cbackend).
type funcSig struct {
	Params     []ir.Type
	ReturnType ir.Type
	HasReturn  bool
}

// scope - символьная таблица одной области видимости: имя переменной ->
// тип.
type scope map[string]ir.Type

// ctx - состояние анализа одной функции: стек областей видимости (шаг 2 -
// если/пока/для вводят вложенные блоки, затенение разрешено - declare
// проверяет только текущую область, не внешние) плюс сигнатуры всех
// функций программы (для вызовов, шаг 3) и тип результата текущей
// функции (для проверки "вернуть <значение>"). Аналог genCtx в cbackend,
// только на уровне IR/семантики, не C.
type ctx struct {
	funcs      map[string]funcSig
	scopes     []scope
	returnType ir.Type
	hasReturn  bool
}

func newCtx(funcs map[string]funcSig, returnType ir.Type, hasReturn bool) *ctx {
	return &ctx{funcs: funcs, scopes: []scope{{}}, returnType: returnType, hasReturn: hasReturn}
}

func (c *ctx) push() {
	c.scopes = append(c.scopes, scope{})
}

func (c *ctx) pop() {
	c.scopes = c.scopes[:len(c.scopes)-1]
}

func (c *ctx) declare(name string, typ ir.Type) error {
	top := c.scopes[len(c.scopes)-1]
	if _, exists := top[name]; exists {
		return fmt.Errorf("переменная %q уже объявлена в этой области видимости", name)
	}
	top[name] = typ
	return nil
}

func (c *ctx) lookup(name string) (ir.Type, bool) {
	for _, sc := range slices.Backward(c.scopes) {
		if typ, ok := sc[name]; ok {
			return typ, true
		}
	}
	return ir.TypeUnknown, false
}

// resolveTypeName - единственное место, где имя типа Rech (строка из AST)
// маппится на ir.Type. Только встроенные примитивы - сущности и generic-
// коллекции (шаги 4, 4.2) в IR ещё не перенесены.
func resolveTypeName(name string) (ir.Type, error) {
	switch name {
	case "Строка":
		return ir.TypeString, nil
	case "Целое":
		return ir.TypeInt, nil
	case "Число":
		return ir.TypeFloat, nil
	case "Логическое":
		return ir.TypeBool, nil
	default:
		return ir.TypeUnknown, fmt.Errorf("тип %q пока не реализован в IR (шаг 5.b) - только Строка/Целое/Число/Логическое", name)
	}
}

// Analyze производит IR из AST. Покрытие на данный момент - шаги 0-3
// (`вывести`, переменные/арифметика, если/пока/для, функции с параметрами
// и типом результата, вызов) - см. internal/ir, package doc. Расширяется
// вместе с internal/backend/cbackend.GenerateFromIR подшаг за подшагом,
// шаг 5.
func Analyze(prog *ast.Program) (*ir.Program, error) {
	funcs, err := collectFuncSigs(prog)
	if err != nil {
		return nil, err
	}
	out := &ir.Program{}
	for _, fn := range prog.Functions {
		irFn, err := analyzeFunction(funcs, fn)
		if err != nil {
			return nil, fmt.Errorf("функция %q: %w", fn.Name, err)
		}
		out.Functions = append(out.Functions, irFn)
	}
	return out, nil
}

// collectFuncSigs - предварительный проход: сигнатуры всех функций
// программы, до анализа любого тела (нужно для вызовов вперёд/взаимной
// рекурсии - как и в cbackend.Generate).
func collectFuncSigs(prog *ast.Program) (map[string]funcSig, error) {
	funcs := map[string]funcSig{}
	for _, fn := range prog.Functions {
		if _, exists := funcs[fn.Name]; exists {
			return nil, fmt.Errorf("функция %q объявлена более одного раза", fn.Name)
		}
		params := make([]ir.Type, len(fn.Params))
		for i, p := range fn.Params {
			typ, err := resolveTypeName(p.Type)
			if err != nil {
				return nil, fmt.Errorf("функция %q: параметр %q: %w", fn.Name, p.Name, err)
			}
			params[i] = typ
		}
		sig := funcSig{Params: params}
		if fn.ReturnType != "" {
			typ, err := resolveTypeName(fn.ReturnType)
			if err != nil {
				return nil, fmt.Errorf("функция %q: тип результата: %w", fn.Name, err)
			}
			sig.ReturnType = typ
			sig.HasReturn = true
		}
		funcs[fn.Name] = sig
	}
	return funcs, nil
}

func analyzeFunction(funcs map[string]funcSig, fn *ast.FunctionDecl) (*ir.Function, error) {
	sig := funcs[fn.Name]
	sc := newCtx(funcs, sig.ReturnType, sig.HasReturn)
	params := make([]ir.Param, len(fn.Params))
	for i, p := range fn.Params {
		params[i] = ir.Param{Name: p.Name, Type: sig.Params[i]}
		if err := sc.declare(p.Name, sig.Params[i]); err != nil {
			return nil, fmt.Errorf("строка %d: параметр %q: %w", p.Line, p.Name, err)
		}
	}
	body, err := analyzeBlock(sc, fn.Body)
	if err != nil {
		return nil, err
	}
	if sig.HasReturn && !endsWithReturnValue(body) {
		return nil, fmt.Errorf("с объявленным типом результата %s должна заканчиваться \"вернуть <значение>\" (полная проверка всех путей выполнения - шаг 5)", sig.ReturnType)
	}
	return &ir.Function{
		Name:       fn.Name,
		Params:     params,
		ReturnType: sig.ReturnType,
		HasReturn:  sig.HasReturn,
		Body:       body,
	}, nil
}

// endsWithReturnValue - true, если последняя инструкция тела (верхнего
// уровня, не внутри вложенных если/пока/для) - "вернуть <значение>". Как и
// в AST-пути (cbackend.endsWithReturnValue) - простая синтаксическая
// проверка, не flow-sensitive анализ всех путей выполнения (это шаг 5,
// когда появится сам flow-sensitive слой - см. docs/ROADMAP.md).
func endsWithReturnValue(body []ir.Stmt) bool {
	if len(body) == 0 {
		return false
	}
	last, ok := body[len(body)-1].(*ir.ReturnStmt)
	return ok && last.Value != nil
}

// analyzeBlock анализирует список инструкций внутри уже открытой области
// видимости - вызывающий код сам решает, нужно ли sc.push()/sc.pop()
// вокруг вызова (тело функции - нет, тело если/пока/для - да).
func analyzeBlock(sc *ctx, stmts []ast.Stmt) ([]ir.Stmt, error) {
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

func analyzeStmt(sc *ctx, s ast.Stmt) (ir.Stmt, error) {
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
		if s.Value == nil {
			if sc.hasReturn {
				return nil, fmt.Errorf("строка %d: функция с объявленным типом результата должна возвращать значение (\"вернуть <значение>\")", s.Line)
			}
			return &ir.ReturnStmt{}, nil
		}
		if !sc.hasReturn {
			return nil, fmt.Errorf("строка %d: у функции не объявлен тип результата - используйте \"вернуть\" без значения", s.Line)
		}
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		if typ := ir.TypeOf(value); typ != sc.returnType {
			return nil, fmt.Errorf("строка %d: функция объявлена с типом результата %s, но возвращается значение типа %s", s.Line, sc.returnType, typ)
		}
		return &ir.ReturnStmt{Value: value}, nil

	case *ast.ExprStmt:
		if s.MethodCall != nil {
			return nil, fmt.Errorf("строка %d: вызов метода пока не реализован в IR (шаг 5.b)", s.Line)
		}
		call, err := analyzeCallExpr(sc, s.Call)
		if err != nil {
			return nil, err
		}
		return &ir.ExprStmt{Call: call}, nil

	default:
		return nil, fmt.Errorf("инструкция %T пока не реализована в IR (шаг 5.b)", s)
	}
}

func analyzeExpr(sc *ctx, e ast.Expr) (ir.Expr, error) {
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

	case *ast.CallExpr:
		call, err := analyzeCallExpr(sc, e)
		if err != nil {
			return nil, err
		}
		if !call.HasReturn {
			return nil, fmt.Errorf("строка %d: функция %q не имеет объявленного типа результата - её нельзя использовать как выражение", e.Line, e.Callee)
		}
		return call, nil

	default:
		return nil, fmt.Errorf("выражение %T пока не реализовано в IR (шаг 5.b)", e)
	}
}

// analyzeCallExpr - общий разбор вызова функции для ExprStmt (результат
// отбрасывается, HasReturn не важен) и для CallExpr-выражения (требует
// HasReturn - проверяется вызывающей стороной, analyzeExpr).
func analyzeCallExpr(sc *ctx, e *ast.CallExpr) (*ir.CallExpr, error) {
	sig, ok := sc.funcs[e.Callee]
	if !ok {
		return nil, fmt.Errorf("строка %d: функция %q не объявлена", e.Line, e.Callee)
	}
	if len(e.Args) != len(sig.Params) {
		return nil, fmt.Errorf("строка %d: функция %q ожидает аргументов: %d, передано: %d", e.Line, e.Callee, len(sig.Params), len(e.Args))
	}
	args := make([]ir.Expr, len(e.Args))
	for i, a := range e.Args {
		arg, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(arg); typ != sig.Params[i] {
			return nil, fmt.Errorf("строка %d: аргумент %d вызова %q должен быть типа %s, получен %s", e.Line, i+1, e.Callee, sig.Params[i], typ)
		}
		args[i] = arg
	}
	return &ir.CallExpr{Callee: e.Callee, Args: args, Type: sig.ReturnType, HasReturn: sig.HasReturn}, nil
}

func analyzeUnaryExpr(sc *ctx, e *ast.UnaryExpr) (ir.Expr, error) {
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

func analyzeBinaryExpr(sc *ctx, e *ast.BinaryExpr) (ir.Expr, error) {
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
// решает backend при генерации кода конкретной платформы, глядя на Type
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
