package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

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

// analyzeLoopBody запускает тело цикла (пока/для/для-в) в новой области
// видимости и откатывает established к состоянию ДО цикла после выхода -
// цикл может не выполниться ни разу, поэтому established, доказанное
// только внутри тела, не гарантировано после цикла (см. package doc
// ctx.established). declareVar == "" - без объявления переменной цикла
// (у "пока" её нет), иначе объявляет declareVar:declareType в той же новой
// области ДО анализа тела. Общая для WhileStmt/ForStmt/ForEachStmt (обе
// ветки - Файл и Список<T>).
func analyzeLoopBody(sc *ctx, declareVar string, declareType ir.Type, body []ast.Stmt, line int) ([]ir.Stmt, error) {
	preLoop := snapshotEstablished(sc.established)
	sc.push()
	if declareVar != "" {
		if err := sc.declare(declareVar, declareType); err != nil {
			sc.pop()
			return nil, fmt.Errorf("строка %d: %w", line, err)
		}
	}
	analyzed, err := analyzeBlock(sc, body)
	sc.pop()
	if err != nil {
		return nil, err
	}
	sc.established = preLoop
	return analyzed, nil
}

func analyzeStmt(sc *ctx, s ast.Stmt) (ir.Stmt, error) {
	switch s := s.(type) {
	case *ast.PrintStmt:
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		value = maybeUnwrapNarrowed(sc, value)
		if typ := ir.TypeOf(value); typ.Kind == ir.KindNullable {
			return nil, fmt.Errorf("строка %d: нельзя напрямую вывести значение nullable-типа %s - разверните его сначала (\"?? запасное\" или явный каст)", s.Line, typ)
		}
		return &ir.PrintStmt{Value: value, Line: s.Line}, nil

	case *ast.ShowStmt:
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		value = maybeUnwrapNarrowed(sc, value)
		if typ := ir.TypeOf(value); typ.Kind == ir.KindNullable {
			return nil, fmt.Errorf("строка %d: нельзя напрямую показать значение nullable-типа %s - разверните его сначала (\"?? запасное\" или явный каст)", s.Line, typ)
		}
		return &ir.ShowStmt{Value: value, Line: s.Line}, nil

	case *ast.VarDecl:
		var declared ir.Type
		var hasAnnotation bool
		if s.Type != "" {
			t, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, s.Type)
			if err != nil {
				return nil, fmt.Errorf("строка %d: %w", s.Line, err)
			}
			declared, hasAnnotation = t, true
		}
		var value ir.Expr
		var err error
		if hasAnnotation {
			value, err = coerceExpr(sc, s.Value, declared)
		} else {
			value, err = analyzeExpr(sc, s.Value)
		}
		if err != nil {
			return nil, err
		}
		if hasAnnotation {
			if typ := ir.TypeOf(value); !isAssignable(sc.entities, typ, declared) {
				return nil, fmt.Errorf("строка %d: переменная %q объявлена типом %s, но инициализирована значением типа %s", s.Line, s.Name, declared, typ)
			}
		}
		// До sc.declare - иначе при затенении (переменная %s уже с этим же
		// именем во внешней области) established читался бы уже с только
		// что объявленной (пустой) внутренней переменной вместо той, на
		// которую на самом деле ссылается value.
		newEstablished := establishedForNewBinding(sc, s.Name, value)
		if err := sc.declare(s.Name, ir.TypeOf(value)); err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		sc.setEstablished(s.Name, newEstablished)
		return &ir.VarDecl{Name: s.Name, Value: value, Line: s.Line}, nil

	case *ast.ConstDecl:
		// Полностью тот же код, что и VarDecl выше (одна и та же ir.VarDecl -
		// internal/ir не различает константу и переменную вообще, см.
		// package doc ast.ConstDecl), плюс markConst сразу после declare -
		// дальше любой Assign этому имени должен быть отвергнут (см. ниже).
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		newEstablished := establishedForNewBinding(sc, s.Name, value)
		if err := sc.declare(s.Name, ir.TypeOf(value)); err != nil {
			return nil, fmt.Errorf("строка %d: %w", s.Line, err)
		}
		sc.setEstablished(s.Name, newEstablished)
		sc.markConst(s.Name)
		return &ir.VarDecl{Name: s.Name, Value: value, Line: s.Line}, nil

	case *ast.Assign:
		declared, exists := sc.lookup(s.Name)
		if !exists {
			return nil, fmt.Errorf("строка %d: переменная %q не объявлена - используйте \"переменная\" для объявления", s.Line, s.Name)
		}
		if sc.isConst(s.Name) {
			return nil, fmt.Errorf("строка %d: %q объявлено как \"константа\" - нельзя присвоить новое значение", s.Line, s.Name)
		}
		value, err := coerceExpr(sc, s.Value, declared)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(value); !isAssignable(sc.entities, typ, declared) {
			return nil, fmt.Errorf("строка %d: переменной %q типа %s нельзя присвоить значение типа %s", s.Line, s.Name, declared, typ)
		}
		sc.setEstablished(s.Name, establishedForNewBinding(sc, s.Name, value))
		return &ir.Assign{Name: s.Name, Value: value, Line: s.Line}, nil

	case *ast.NullCoalesceAssign:
		return analyzeNullCoalesceAssignStmt(sc, s)

	case *ast.AssertStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: условие \"утверждать\" должно быть логическим, получено %s", s.Line, typ)
		}
		return &ir.AssertStmt{Cond: cond, Line: s.Line}, nil

	case *ast.IfStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: условие \"если\" должно быть логическим, получено %s", s.Line, typ)
		}
		// Слияние established после "если/иначе" - переменная доказанно
		// прошла состояние X дальше по коду только если прошла его НА ОБОИХ
		// путях выполнения (intersectEstablished) - ветки анализируются от
		// одного и того же снапшота (preIf), не одна поверх результата
		// другой, иначе Else "видел" бы эффект Then.
		// Сужение nullable-типа (nonNullState, internal/semantic/nullable.go)
		// тем же established-механизмом, что и обычный typestate: "если а !=
		// Ничто {...}" доказывает непустоту а внутри Then, "если а == Ничто
		// {...} иначе {...}" - симметрично внутри Else.
		nc, isNullCheck := cond.(*ir.NullCheckExpr)
		var narrowedName string
		if isNullCheck {
			if ident, ok := nc.Value.(*ir.Identifier); ok {
				narrowedName = ident.Name
			}
		}
		preIf := snapshotEstablished(sc.established)
		sc.push()
		if isNullCheck && nc.IsNot && narrowedName != "" {
			markNonNull(sc, narrowedName)
		}
		then, err := analyzeBlock(sc, s.Then)
		sc.pop()
		if err != nil {
			return nil, err
		}
		postThen := snapshotEstablished(sc.established)
		sc.established = preIf
		var elseBody []ir.Stmt
		postElse := preIf
		if s.Else != nil {
			sc.push()
			if isNullCheck && !nc.IsNot && narrowedName != "" {
				markNonNull(sc, narrowedName)
			}
			elseBody, err = analyzeBlock(sc, s.Else)
			sc.pop()
			if err != nil {
				return nil, err
			}
			postElse = snapshotEstablished(sc.established)
		}
		sc.established = intersectEstablished(postThen, postElse)
		return &ir.IfStmt{Cond: cond, Then: then, Else: elseBody, Line: s.Line}, nil

	case *ast.WhileStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, fmt.Errorf("строка %d: условие \"пока\" должно быть логическим, получено %s", s.Line, typ)
		}
		// "пока" может не выполниться ни разу - established, доказанное
		// внутри тела, не гарантировано ПОСЛЕ цикла (консервативный откат к
		// состоянию до цикла, а не fixed-point анализ по итерациям - см.
		// package doc ctx.established); внутри самого тела, в пределах
		// одного прохода, накопленные по ходу established всё ещё видны
		// (обычный sc.push/analyzeBlock/sc.pop).
		body, err := analyzeLoopBody(sc, "", ir.Type{}, s.Body, s.Line)
		if err != nil {
			return nil, err
		}
		return &ir.WhileStmt{Cond: cond, Body: body, Line: s.Line}, nil

	case *ast.ForStmt:
		from, err := analyzeExpr(sc, s.From)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(from); typ != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: граница диапазона после \"от\" должна быть целым числом, получено %s", s.Line, typ)
		}
		to, err := analyzeExpr(sc, s.To)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(to); typ != ir.TypeInt {
			return nil, fmt.Errorf("строка %d: граница диапазона после \"до\" должна быть целым числом, получено %s", s.Line, typ)
		}
		body, err := analyzeLoopBody(sc, s.Var, ir.TypeInt, s.Body, s.Line)
		if err != nil {
			return nil, err
		}
		return &ir.ForStmt{Var: s.Var, From: from, To: to, Body: body, Line: s.Line}, nil

	case *ast.ForEachStmt:
		collection, err := analyzeExpr(sc, s.Collection)
		if err != nil {
			return nil, err
		}
		typ := ir.TypeOf(collection)
		// "для <строка> в <Файл>" - потоковое построчное чтение (шаг 8), а
		// не индексированный доступ, как у Список<T> ниже - файл не
		// известной заранее длины. ElemType всегда Строка (сама строка
		// файла), сигнатуры Список<T> здесь ни при чём.
		if typ.Kind == ir.KindFile {
			body, err := analyzeLoopBody(sc, s.Var, ir.TypeString, s.Body, s.Line)
			if err != nil {
				return nil, err
			}
			return &ir.ForEachStmt{Var: s.Var, Collection: collection, ElemType: ir.TypeString, Body: body, Line: s.Line}, nil
		}
		sig := sc.entities[typ.Entity]
		if typ.Kind != ir.KindEntity || !sig.IsList {
			return nil, fmt.Errorf("строка %d: цикл \"для %s в ...\" ожидает переменную-Список<T> или Файл, получен тип %s", s.Line, s.Var, typ)
		}
		body, err := analyzeLoopBody(sc, s.Var, sig.ElemType, s.Body, s.Line)
		if err != nil {
			return nil, err
		}
		return &ir.ForEachStmt{Var: s.Var, Collection: collection, ElemType: sig.ElemType, Body: body, Line: s.Line}, nil

	case *ast.ReturnStmt:
		if s.Value == nil {
			if sc.hasReturn {
				return nil, fmt.Errorf("строка %d: функция с объявленным типом результата должна возвращать значение (\"вернуть <значение>\")", s.Line)
			}
			return &ir.ReturnStmt{Line: s.Line}, nil
		}
		if !sc.hasReturn {
			return nil, fmt.Errorf("строка %d: у функции не объявлен тип результата - используйте \"вернуть\" без значения", s.Line)
		}
		value, err := coerceExpr(sc, s.Value, sc.returnType)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(value); !isAssignable(sc.entities, typ, sc.returnType) {
			return nil, fmt.Errorf("строка %d: функция объявлена с типом результата %s, но возвращается значение типа %s", s.Line, sc.returnType, typ)
		}
		return &ir.ReturnStmt{Value: value, Line: s.Line}, nil

	case *ast.ExprStmt:
		if s.Await != nil {
			await, err := analyzeAwaitExpr(sc, s.Await)
			if err != nil {
				return nil, err
			}
			return &ir.ExprStmt{Await: await, Line: s.Line}, nil
		}
		if s.MethodCall != nil {
			call, err := analyzeMethodCallExpr(sc, s.MethodCall)
			if err != nil {
				return nil, err
			}
			return &ir.ExprStmt{MethodCall: call, Line: s.Line}, nil
		}
		if s.IncDec != nil {
			incDec, err := analyzeIncDecExpr(sc, s.IncDec)
			if err != nil {
				return nil, err
			}
			return &ir.ExprStmt{IncDec: incDec.(*ir.IncDecExpr), Line: s.Line}, nil
		}
		call, err := analyzeCallExpr(sc, s.Call)
		if err != nil {
			return nil, err
		}
		switch call := call.(type) {
		case *ir.CallExpr:
			return &ir.ExprStmt{Call: call, Line: s.Line}, nil
		case *ir.CallValueExpr:
			return &ir.ExprStmt{CallValue: call, Line: s.Line}, nil
		default:
			return nil, fmt.Errorf("внутренняя ошибка: неизвестный вид вызова %T", call)
		}

	case *ast.FieldAssign:
		if !receiverAddressable(s.Receiver) {
			return nil, fmt.Errorf("строка %d: получателем присваивания полю может быть только переменная, поле или \"этот\"", s.Line)
		}
		receiver, err := analyzeExpr(sc, s.Receiver)
		if err != nil {
			return nil, err
		}
		typ := ir.TypeOf(receiver)
		if typ.Kind != ir.KindEntity {
			return nil, fmt.Errorf("строка %d: нельзя присвоить полю %q - тип %s не сущность", s.Line, s.Field, typ)
		}
		sig := sc.entities[typ.Entity]

		// Нельзя присваивать значение константе сущности
		if cidx := sig.constIndex(s.Field); cidx != -1 {
			return nil, fmt.Errorf("строка %d: нельзя присвоить значение константе %q.%q", s.Line, typ.Entity, s.Field)
		}

		idx := sig.fieldIndex(s.Field)
		if idx == -1 {
			return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", s.Line, typ.Entity, s.Field)
		}
		if err := checkFieldVisibility(sc, typ.Entity, s.Field, sig.FieldVisibility[idx], s.Line); err != nil {
			return nil, err
		}
		value, err := coerceExpr(sc, s.Value, sig.FieldTypes[idx])
		if err != nil {
			return nil, err
		}
		if vt := ir.TypeOf(value); !isAssignable(sc.entities, vt, sig.FieldTypes[idx]) {
			return nil, fmt.Errorf("строка %d: полю %q типа %s нельзя присвоить значение типа %s", s.Line, s.Field, sig.FieldTypes[idx], vt)
		}
		return &ir.FieldAssign{Receiver: receiver, Field: s.Field, Value: value, Line: s.Line}, nil

	case *ast.BecomeStmt:
		receiver, err := analyzeExpr(sc, s.Receiver)
		if err != nil {
			return nil, err
		}
		typ := ir.TypeOf(receiver)
		if typ.Kind != ir.KindEntity {
			return nil, fmt.Errorf("строка %d: \"становится\" применимо только к сущности, получен тип %s", s.Line, typ)
		}
		sig := sc.entities[typ.Entity]
		if !sig.States[s.State] {
			return nil, fmt.Errorf("строка %d: у сущности %q не объявлено состояние %q", s.Line, typ.Entity, s.State)
		}
		// Фиксируем доказанный переход - только для отслеживаемых форм
		// получателя (простая переменная/"этот", см. package doc ctx.
		// established); получатель-поле сущности молча не отслеживается
		// (безопасный дефолт "ничего не доказано" дальше по коду).
		switch r := receiver.(type) {
		case *ir.Identifier:
			next := cloneStates(sc.getEstablished(r.Name))
			next[s.State] = true
			sc.setEstablished(r.Name, next)
		case *ir.ThisExpr:
			next := cloneStates(sc.thisEstablished)
			next[s.State] = true
			sc.thisEstablished = next
		}
		return &ir.BecomeStmt{Receiver: receiver, State: s.State}, nil

	case *ast.ParentCallStmt:
		if !sc.allowParentCall {
			return nil, fmt.Errorf("строка %d: \"родитель(...)\" допустимо только первой инструкцией тела конструктора", s.Line)
		}
		sc.allowParentCall = false
		baseName := sc.entities[sc.thisEntity].BaseName
		if baseName == "" {
			return nil, fmt.Errorf("строка %d: у сущности %q нет базовой сущности - \"родитель(...)\" вызывать нечего", s.Line, sc.thisEntity)
		}
		baseSig := sc.entities[baseName]
		if !baseSig.HasCtor {
			return nil, fmt.Errorf("строка %d: у базовой сущности %q нет собственного конструктора - вызывать нечего, присвойте унаследованные поля напрямую через \"этот.поле = ...\"", s.Line, baseName)
		}
		if len(s.Args) != len(baseSig.CtorParams) {
			return nil, fmt.Errorf("строка %d: конструктор %q ожидает аргументов: %d, передано: %d", s.Line, baseName, len(baseSig.CtorParams), len(s.Args))
		}
		args := make([]ir.Expr, len(s.Args))
		for i, a := range s.Args {
			arg, err := coerceExpr(sc, a, baseSig.CtorParams[i])
			if err != nil {
				return nil, err
			}
			if typ := ir.TypeOf(arg); !isAssignable(sc.entities, typ, baseSig.CtorParams[i]) {
				return nil, fmt.Errorf("строка %d: аргумент %d конструктора %q должен быть типа %s, получен %s", s.Line, i+1, baseName, baseSig.CtorParams[i], typ)
			}
			args[i] = arg
		}
		return &ir.ParentCallStmt{BaseEntity: baseName, Args: args, Line: s.Line}, nil

	case *ast.ThrowStmt:
		return analyzeThrowStmt(sc, s)

	case *ast.TryStmt:
		return analyzeTryStmt(sc, s)

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
