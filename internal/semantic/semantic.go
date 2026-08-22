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

// entitySig - сигнатура сущности: поля в порядке объявления (для
// конструктора и доступа к полю, шаг 4.b), собранная предварительным
// проходом (collectEntitySigs) - тот же принцип, что и funcSig, отдельный
// тип от cbackend.entitySig (IR не должен знать про cbackend).
type entitySig struct {
	FieldNames []string
	FieldTypes []ir.Type
}

func (s entitySig) fieldIndex(name string) int {
	for i, n := range s.FieldNames {
		if n == name {
			return i
		}
	}
	return -1
}

// scope - символьная таблица одной области видимости: имя переменной ->
// тип.
type scope map[string]ir.Type

// ctx - состояние анализа одной функции/метода: стек областей видимости
// (шаг 2 - если/пока/для вводят вложенные блоки, затенение разрешено -
// declare проверяет только текущую область, не внешние), сигнатуры всех
// функций/сущностей/методов программы (для вызовов и доступа к полям,
// шаги 3-4), тип результата текущей функции/метода (для проверки "вернуть
// <значение>") и имя сущности-владельца, если сейчас анализируется метод
// ("" - обычная функция, "этот" внутри неё синтаксически невозможен -
// парсер уже это гарантирует). Аналог genCtx в cbackend, только на уровне
// IR/семантики, не C.
type ctx struct {
	funcs      map[string]funcSig
	entities   map[string]entitySig
	methods    map[string]map[string]funcSig
	thisEntity string
	scopes     []scope
	returnType ir.Type
	hasReturn  bool
}

func newCtx(funcs map[string]funcSig, entities map[string]entitySig, methods map[string]map[string]funcSig, thisEntity string, returnType ir.Type, hasReturn bool) *ctx {
	return &ctx{
		funcs: funcs, entities: entities, methods: methods, thisEntity: thisEntity,
		scopes: []scope{{}}, returnType: returnType, hasReturn: hasReturn,
	}
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
// маппится на ir.Type: встроенные примитивы или уже собранная сущность
// (entities - сигнатуры, собранные раньше по файлу, см. collectEntities;
// сущность, объявленная позже в файле, чем место, где её имя встретилось
// как тип - та же граница, что и в cbackend.resolveType). Generic-
// коллекции (шаг 4.2) сюда пока не перенесены.
func resolveTypeName(entities map[string]entitySig, name string) (ir.Type, error) {
	switch name {
	case "Строка":
		return ir.TypeString, nil
	case "Целое":
		return ir.TypeInt, nil
	case "Число":
		return ir.TypeFloat, nil
	case "Логическое":
		return ir.TypeBool, nil
	}
	if _, ok := entities[name]; ok {
		return ir.EntityType(name), nil
	}
	return ir.TypeUnknown, fmt.Errorf("неизвестный тип %q", name)
}

// Analyze производит IR из AST. Покрытие на данный момент - шаги 0-4
// (`вывести`, переменные/арифметика, если/пока/для, функции с параметрами
// и типом результата, вызов, сущности/поля/конструктор/методы/"этот") -
// см. internal/ir, package doc. Расширяется вместе с internal/backend/
// cbackend.GenerateFromIR подшаг за подшагом, шаг 5.
func Analyze(prog *ast.Program) (*ir.Program, error) {
	entities, methods, err := collectEntities(prog)
	if err != nil {
		return nil, err
	}
	funcs, err := collectFuncSigs(entities, prog)
	if err != nil {
		return nil, err
	}

	out := &ir.Program{}
	for _, ent := range prog.Entities {
		irEnt, err := analyzeEntity(funcs, entities, methods, ent)
		if err != nil {
			return nil, fmt.Errorf("сущность %q: %w", ent.Name, err)
		}
		out.Entities = append(out.Entities, irEnt)
	}
	for _, fn := range prog.Functions {
		irFn, err := analyzeFunction(funcs, entities, methods, fn)
		if err != nil {
			return nil, fmt.Errorf("функция %q: %w", fn.Name, err)
		}
		out.Functions = append(out.Functions, irFn)
	}
	return out, nil
}

// collectFuncSigs - предварительный проход: сигнатуры всех функций
// программы, до анализа любого тела (нужно для вызовов вперёд/взаимной
// рекурсии - как и в cbackend.Generate). entities уже полностью собраны
// (collectEntities) - функция может ссылаться на любую сущность,
// независимо от того, где та объявлена в файле относительно функции.
func collectFuncSigs(entities map[string]entitySig, prog *ast.Program) (map[string]funcSig, error) {
	funcs := map[string]funcSig{}
	for _, fn := range prog.Functions {
		if _, exists := funcs[fn.Name]; exists {
			return nil, fmt.Errorf("функция %q объявлена более одного раза", fn.Name)
		}
		params := make([]ir.Type, len(fn.Params))
		for i, p := range fn.Params {
			typ, err := resolveTypeName(entities, p.Type)
			if err != nil {
				return nil, fmt.Errorf("функция %q: параметр %q: %w", fn.Name, p.Name, err)
			}
			params[i] = typ
		}
		sig := funcSig{Params: params}
		if fn.ReturnType != "" {
			typ, err := resolveTypeName(entities, fn.ReturnType)
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

// collectEntities - предварительный проход: поля и сигнатуры методов всех
// сущностей программы, до анализа любого тела. Одним циклом по
// prog.Entities, как и в cbackend.Generate - поле/параметр/тип результата
// метода сущности N может ссылаться только на сущность, уже занесённую в
// entities к этому моменту (объявленную раньше в файле, включая саму N) -
// та же граница, что и в cbackend. Состояния (шаг 4.1) явно не поддержаны,
// не тихий пропуск.
func collectEntities(prog *ast.Program) (map[string]entitySig, map[string]map[string]funcSig, error) {
	entities := map[string]entitySig{}
	methods := map[string]map[string]funcSig{}
	for _, ent := range prog.Entities {
		if _, exists := entities[ent.Name]; exists {
			return nil, nil, fmt.Errorf("сущность %q объявлена более одного раза", ent.Name)
		}
		if len(ent.States) > 0 {
			return nil, nil, fmt.Errorf("сущность %q: состояния пока не поддерживаются", ent.Name)
		}
		var sig entitySig
		for _, f := range ent.Fields {
			if sig.fieldIndex(f.Name) != -1 {
				return nil, nil, fmt.Errorf("сущность %q: поле %q объявлено более одного раза", ent.Name, f.Name)
			}
			typ, err := resolveTypeName(entities, f.Type)
			if err != nil {
				return nil, nil, fmt.Errorf("сущность %q: поле %q: %w", ent.Name, f.Name, err)
			}
			sig.FieldNames = append(sig.FieldNames, f.Name)
			sig.FieldTypes = append(sig.FieldTypes, typ)
		}
		entities[ent.Name] = sig

		methods[ent.Name] = map[string]funcSig{}
		for _, m := range ent.Methods {
			if _, exists := methods[ent.Name][m.Name]; exists {
				return nil, nil, fmt.Errorf("сущность %q: метод %q объявлен более одного раза", ent.Name, m.Name)
			}
			params := make([]ir.Type, len(m.Params))
			for i, p := range m.Params {
				typ, err := resolveTypeName(entities, p.Type)
				if err != nil {
					return nil, nil, fmt.Errorf("сущность %q: метод %q: параметр %q: %w", ent.Name, m.Name, p.Name, err)
				}
				params[i] = typ
			}
			msig := funcSig{Params: params}
			if m.ReturnType != "" {
				typ, err := resolveTypeName(entities, m.ReturnType)
				if err != nil {
					return nil, nil, fmt.Errorf("сущность %q: метод %q: тип результата: %w", ent.Name, m.Name, err)
				}
				msig.ReturnType = typ
				msig.HasReturn = true
			}
			methods[ent.Name][m.Name] = msig
		}
	}
	return entities, methods, nil
}

func analyzeEntity(funcs map[string]funcSig, entities map[string]entitySig, methods map[string]map[string]funcSig, ent *ast.EntityDecl) (*ir.Entity, error) {
	sig := entities[ent.Name]
	fields := make([]ir.Field, len(sig.FieldNames))
	for i, name := range sig.FieldNames {
		fields[i] = ir.Field{Name: name, Type: sig.FieldTypes[i]}
	}
	irMethods := make([]*ir.Method, len(ent.Methods))
	for i, m := range ent.Methods {
		irM, err := analyzeMethod(funcs, entities, methods, ent.Name, m)
		if err != nil {
			return nil, fmt.Errorf("метод %q: %w", m.Name, err)
		}
		irMethods[i] = irM
	}
	return &ir.Entity{Name: ent.Name, Fields: fields, Methods: irMethods}, nil
}

func analyzeMethod(funcs map[string]funcSig, entities map[string]entitySig, methods map[string]map[string]funcSig, entityName string, m *ast.MethodDecl) (*ir.Method, error) {
	sig := methods[entityName][m.Name]
	sc := newCtx(funcs, entities, methods, entityName, sig.ReturnType, sig.HasReturn)
	params := make([]ir.Param, len(m.Params))
	for i, p := range m.Params {
		params[i] = ir.Param{Name: p.Name, Type: sig.Params[i]}
		if err := sc.declare(p.Name, sig.Params[i]); err != nil {
			return nil, fmt.Errorf("строка %d: параметр %q: %w", p.Line, p.Name, err)
		}
	}
	body, err := analyzeBlock(sc, m.Body)
	if err != nil {
		return nil, err
	}
	if sig.HasReturn && !endsWithReturnValue(body) {
		return nil, fmt.Errorf("с объявленным типом результата %s должен заканчиваться \"вернуть <значение>\"", sig.ReturnType)
	}
	return &ir.Method{
		Name:       m.Name,
		Params:     params,
		ReturnType: sig.ReturnType,
		HasReturn:  sig.HasReturn,
		Body:       body,
	}, nil
}

func analyzeFunction(funcs map[string]funcSig, entities map[string]entitySig, methods map[string]map[string]funcSig, fn *ast.FunctionDecl) (*ir.Function, error) {
	sig := funcs[fn.Name]
	sc := newCtx(funcs, entities, methods, "", sig.ReturnType, sig.HasReturn)
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
		return nil, fmt.Errorf("с объявленным типом результата %s должна заканчиваться \"вернуть <значение>\"", sig.ReturnType)
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
			call, err := analyzeMethodCallExpr(sc, s.MethodCall)
			if err != nil {
				return nil, err
			}
			return &ir.ExprStmt{MethodCall: call}, nil
		}
		call, err := analyzeCallExpr(sc, s.Call)
		if err != nil {
			return nil, err
		}
		return &ir.ExprStmt{Call: call}, nil

	case *ast.FieldAssign:
		if !receiverAddressable(s.Receiver) {
			return nil, fmt.Errorf("строка %d: получателем присваивания полю может быть только переменная, поле или \"этот\"", s.Line)
		}
		receiver, err := analyzeExpr(sc, s.Receiver)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		typ := ir.TypeOf(receiver)
		if typ.Kind != ir.KindEntity {
			return nil, fmt.Errorf("строка %d: нельзя присвоить полю %q - тип %s не сущность", s.Line, s.Field, typ)
		}
		sig := sc.entities[typ.Entity]
		idx := sig.fieldIndex(s.Field)
		if idx == -1 {
			return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", s.Line, typ.Entity, s.Field)
		}
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		if vt := ir.TypeOf(value); vt != sig.FieldTypes[idx] {
			return nil, fmt.Errorf("строка %d: полю %q типа %s нельзя присвоить значение типа %s", s.Line, s.Field, sig.FieldTypes[idx], vt)
		}
		return &ir.FieldAssign{Receiver: receiver, Field: s.Field, Value: value}, nil

	default:
		return nil, fmt.Errorf("инструкция %T не поддерживается", s)
	}
}

// receiverAddressable - получатели вызова метода/присваивания полю, у
// которых в C-пути есть постоянный адрес (переменная/"этот"/доступ к
// полю) - тот же список, что и cbackend.receiverAddressable, но здесь, до
// backend'а: результат вызова функции/конструктора/метода сюда не входит
// (временное значение), явная ошибка Rech лучше невнятной ошибки
// компилятора C.
func receiverAddressable(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Identifier, *ast.ThisExpr, *ast.FieldAccess:
		return true
	default:
		return false
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

	case *ast.NewExpr:
		return analyzeNewExpr(sc, e)

	case *ast.FieldAccess:
		receiver, err := analyzeExpr(sc, e.Receiver)
		if err != nil {
			return nil, err
		}
		typ := ir.TypeOf(receiver)
		if typ.Kind != ir.KindEntity {
			return nil, fmt.Errorf("строка %d: нельзя обратиться к полю %q - тип выражения %s, не сущность", e.Line, e.Field, typ)
		}
		sig := sc.entities[typ.Entity]
		idx := sig.fieldIndex(e.Field)
		if idx == -1 {
			return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", e.Line, typ.Entity, e.Field)
		}
		return &ir.FieldAccess{Receiver: receiver, Field: e.Field, Type: sig.FieldTypes[idx]}, nil

	case *ast.ThisExpr:
		// Парсер уже гарантирует, что "этот" встречается только внутри
		// тела метода - sc.thisEntity здесь всегда заполнен.
		return &ir.ThisExpr{Entity: sc.thisEntity}, nil

	case *ast.MethodCallExpr:
		call, err := analyzeMethodCallExpr(sc, e)
		if err != nil {
			return nil, err
		}
		if !call.HasReturn {
			return nil, fmt.Errorf("строка %d: метод %q не имеет объявленного типа результата - его нельзя использовать как выражение", e.Line, e.Method)
		}
		return call, nil

	default:
		return nil, fmt.Errorf("выражение %T не поддерживается", e)
	}
}

// analyzeNewExpr разбирает "новый Тип(...)" в любой из форм (позиционной
// или именованной) и нормализует Args в позиционный порядок по полю (см.
// package doc ir.NewExpr) - backend'у после этого не нужно знать, какой
// формой был написан конструктор в исходнике.
func analyzeNewExpr(sc *ctx, e *ast.NewExpr) (ir.Expr, error) {
	sig, ok := sc.entities[e.Type]
	if !ok {
		return nil, fmt.Errorf("строка %d: сущность %q не объявлена", e.Line, e.Type)
	}
	args := make([]ir.Expr, len(sig.FieldNames))
	if e.FieldNames != nil {
		if len(e.FieldNames) != len(sig.FieldNames) {
			return nil, fmt.Errorf("строка %d: конструктор %q ожидает значений полей: %d, передано: %d", e.Line, e.Type, len(sig.FieldNames), len(e.FieldNames))
		}
		seen := make([]bool, len(sig.FieldNames))
		for i, label := range e.FieldNames {
			idx := sig.fieldIndex(label)
			if idx == -1 {
				return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", e.Line, e.Type, label)
			}
			if seen[idx] {
				return nil, fmt.Errorf("строка %d: поле %q конструктора %q указано более одного раза", e.Line, label, e.Type)
			}
			seen[idx] = true
			arg, err := analyzeExpr(sc, e.Args[i])
			if err != nil {
				return nil, err
			}
			if typ := ir.TypeOf(arg); typ != sig.FieldTypes[idx] {
				return nil, fmt.Errorf("строка %d: поле %q сущности %q должно быть типа %s, получено %s", e.Line, label, e.Type, sig.FieldTypes[idx], typ)
			}
			args[idx] = arg
		}
		return &ir.NewExpr{Entity: e.Type, Args: args}, nil
	}
	if len(e.Args) != len(sig.FieldNames) {
		return nil, fmt.Errorf("строка %d: конструктор %q ожидает значений полей: %d, передано: %d", e.Line, e.Type, len(sig.FieldNames), len(e.Args))
	}
	for i, a := range e.Args {
		arg, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(arg); typ != sig.FieldTypes[i] {
			return nil, fmt.Errorf("строка %d: аргумент %d конструктора %q должен быть типа %s, получен %s", e.Line, i+1, e.Type, sig.FieldTypes[i], typ)
		}
		args[i] = arg
	}
	return &ir.NewExpr{Entity: e.Type, Args: args}, nil
}

// analyzeMethodCallExpr - общий разбор вызова метода для ExprStmt
// (результат отбрасывается) и для MethodCallExpr-выражения (требует
// HasReturn - проверяется вызывающей стороной, analyzeExpr) - тот же
// принцип, что и analyzeCallExpr для функций.
func analyzeMethodCallExpr(sc *ctx, e *ast.MethodCallExpr) (*ir.MethodCallExpr, error) {
	if !receiverAddressable(e.Receiver) {
		return nil, fmt.Errorf("строка %d: получателем вызова метода может быть только переменная, поле или \"этот\"", e.Line)
	}
	receiver, err := analyzeExpr(sc, e.Receiver)
	if err != nil {
		return nil, err
	}
	typ := ir.TypeOf(receiver)
	if typ.Kind != ir.KindEntity {
		return nil, fmt.Errorf("строка %d: нельзя вызвать метод %q - тип %s не сущность", e.Line, e.Method, typ)
	}
	sig, ok := sc.methods[typ.Entity][e.Method]
	if !ok {
		return nil, fmt.Errorf("строка %d: у сущности %q нет метода %q", e.Line, typ.Entity, e.Method)
	}
	if len(e.Args) != len(sig.Params) {
		return nil, fmt.Errorf("строка %d: метод %q ожидает аргументов: %d, передано: %d", e.Line, e.Method, len(sig.Params), len(e.Args))
	}
	args := make([]ir.Expr, len(e.Args))
	for i, a := range e.Args {
		arg, err := analyzeExpr(sc, a)
		if err != nil {
			return nil, err
		}
		if t := ir.TypeOf(arg); t != sig.Params[i] {
			return nil, fmt.Errorf("строка %d: аргумент %d вызова метода %q должен быть типа %s, получен %s", e.Line, i+1, e.Method, sig.Params[i], t)
		}
		args[i] = arg
	}
	return &ir.MethodCallExpr{Receiver: receiver, Entity: typ.Entity, Method: e.Method, Args: args, Type: sig.ReturnType, HasReturn: sig.HasReturn}, nil
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
