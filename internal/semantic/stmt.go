package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// endsWithReturnValue - true, если body гарантированно заканчивается
// "вернуть <значение>": либо это сама последняя инструкция, либо
// последняя инструкция - "попытаться {...} поймать (...) {...}", У КОТОРОГО
// заканчивается возвратом значения И тело попытки, И тело обработчика (obe
// ветки покрывают все пути исполнения - других способов try/catch
// завершиться, кроме исключения ВНУТРИ catch, здесь нет). Не анализ всех
// путей исполнения программы в общем виде (если/иначе не разворачивается) -
// узкое расширение именно под try/catch, единственную конструкцию, для
// которой раньше приходилось городить накопление в переменную ради этой
// проверки.
func endsWithReturnValue(body []ir.Stmt) bool {
	if len(body) == 0 {
		return false
	}
	switch last := body[len(body)-1].(type) {
	case *ir.ReturnStmt:
		return last.Value != nil
	case *ir.TryStmt:
		return endsWithReturnValue(last.Body) && endsWithReturnValue(last.CatchBody)
	default:
		return false
	}
}

// blockDiverges - true, если body гарантированно не проваливается дальше
// (любой путь исполнения оканчивается "вернуть"/"выбросить", а не просто
// "вернуть <значение>", как у endsWithReturnValue выше - "вернуть" без
// значения тоже расходится). Используется ТОЛЬКО для сужения "?T"
// (established, IfStmt ниже) - "если а == Ничто { <расходится> }" БЕЗ
// явного "иначе" должно доказывать непустоту "а" для остального кода
// ПОСЛЕ "если", ровно как доказало бы "если а == Ничто {...} иначе
// {...использовать а...}" (docs/ROADMAP.md, Шаг 41/44, НАЙДЕННОЕ И
// ИСПРАВЛЕННОЕ ограничение). Как и endsWithReturnValue - узкий анализ
// последней инструкции блока (и рекурсивно её веток), не полный CFG.
func blockDiverges(body []ir.Stmt) bool {
	if len(body) == 0 {
		return false
	}
	switch last := body[len(body)-1].(type) {
	case *ir.ReturnStmt:
		return true
	case *ir.ThrowStmt:
		return true
	case *ir.TryStmt:
		return blockDiverges(last.Body) && blockDiverges(last.CatchBody)
	case *ir.IfStmt:
		return last.Else != nil && blockDiverges(last.Then) && blockDiverges(last.Else)
	default:
		return false
	}
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
			if typ := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, typ, declared) {
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
		var staticID int
		if s.IsStatic {
			sc.a.nextStaticVarID++
			staticID = sc.a.nextStaticVarID
		}
		return &ir.VarDecl{Name: s.Name, Value: value, IsStatic: s.IsStatic, StaticID: staticID, Line: s.Line}, nil

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
		if typ := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, typ, declared) {
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
		// другой, иначе Else "видел" бы эффект Then. ИСКЛЮЧЕНИЕ - ветка,
		// которая гарантированно расходится (blockDiverges, "вернуть"/
		// "выбросить" - код после "если" физически не может быть достигнут
		// ЭТИМ путём): established после "если" наследуется ЦЕЛИКОМ от
		// противоположной ветки, без пересечения - иначе она "забывала" бы
		// собственное сужение. Это же правило распространяется на
		// НЕЯВНУЮ пустую "иначе" (s.Else == nil) - "если а == Ничто {
		// вернуть }" без явного "иначе" доказывает непустоту "а" дальше по
		// коду ТЕМ ЖЕ путём, что и "если а == Ничто {...} иначе {...
		// использовать а...}" (docs/ROADMAP.md, Шаг 41/44, НАЙДЕННОЕ И
		// ИСПРАВЛЕННОЕ ограничение) - narrowing для "иначе" вычисляется
		// всегда, даже когда самого тела "иначе" в исходнике нет.
		//
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
		thenDiverges := blockDiverges(then)
		sc.pop()
		if err != nil {
			return nil, err
		}
		postThen := snapshotEstablished(sc.established)
		sc.established = preIf

		var elseBody []ir.Stmt
		sc.push()
		if isNullCheck && !nc.IsNot && narrowedName != "" {
			markNonNull(sc, narrowedName)
		}
		if s.Else != nil {
			elseBody, err = analyzeBlock(sc, s.Else)
			if err != nil {
				sc.pop()
				return nil, err
			}
		}
		elseDiverges := s.Else != nil && blockDiverges(elseBody)
		postElse := snapshotEstablished(sc.established)
		sc.pop()

		switch {
		case thenDiverges && !elseDiverges:
			sc.established = postElse
		case elseDiverges && !thenDiverges:
			sc.established = postThen
		default:
			sc.established = intersectEstablished(postThen, postElse)
		}
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
		if typ := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, typ, sc.returnType) {
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
			if s.MethodCall.Static {
				call, err := analyzeStaticMethodCallExpr(sc, s.MethodCall)
				if err != nil {
					return nil, err
				}
				return &ir.ExprStmt{StaticMethodCall: call, Line: s.Line}, nil
			}
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
		if s.Static {
			return analyzeStaticFieldAssign(sc, s)
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
		// Как и у analyzeMethodCallExpr - адресуемость получателя
		// (переменная/поле/"этот") нужна только для "структура" (значение,
		// нет постоянного адреса для "&" в cbackend). Ссылочная сущность -
		// уже указатель сама по себе, получателем годится результат ЛЮБОГО
		// вызова (`x.метод().поле = значение`, сколь угодно вложенно -
		// `x.а().б().поле = значение`).
		if !receiverAddressable(s.Receiver) && sig.IsStruct {
			return nil, fmt.Errorf("строка %d: получателем присваивания полю структуры %q может быть только переменная, поле или \"этот\" - результат вызова (значение, не ссылка) не адресуем", s.Line, typ.Entity)
		}

		// Нельзя присваивать значение константе сущности
		if cidx := sig.constIndex(s.Field); cidx != -1 {
			return nil, fmt.Errorf("строка %d: нельзя присвоить значение константе %q.%q", s.Line, typ.Entity, s.Field)
		}

		idx := sig.fieldIndex(s.Field)
		if idx == -1 {
			if sig.staticFieldIndex(s.Field) != -1 {
				return nil, fmt.Errorf("строка %d: %q - статическое поле сущности %q, присваивать только через \"%s::%s\", не через экземпляр", s.Line, s.Field, typ.Entity, typ.Entity, s.Field)
			}
			return nil, fmt.Errorf("строка %d: у сущности %q нет поля %q", s.Line, typ.Entity, s.Field)
		}
		if err := checkFieldVisibility(sc, typ.Entity, s.Field, sig.FieldVisibility[idx], s.Line); err != nil {
			return nil, err
		}
		value, err := coerceExpr(sc, s.Value, sig.FieldTypes[idx])
		if err != nil {
			return nil, err
		}
		if vt := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, vt, sig.FieldTypes[idx]) {
			return nil, fmt.Errorf("строка %d: полю %q типа %s нельзя присвоить значение типа %s", s.Line, s.Field, sig.FieldTypes[idx], vt)
		}
		return &ir.FieldAssign{Receiver: receiver, Field: s.Field, Value: value, Line: s.Line}, nil

	case *ast.DerefAssignStmt:
		return analyzeDerefAssignStmt(sc, s)

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
		// sig.States - ключ полная форма, согласованная по роду/числу И
		// творительным падежом (см. package doc entitySig.States) - s.State
		// несёт ровно то, что написано после "становится", не краткую форму.
		// base - краткая форма, канонический идентификатор состояния дальше
		// по коду (established/ir.BecomeStmt.State).
		base, ok := sig.States[s.State]
		if !ok {
			if hint := suggestStateForm(sig.States, s.State); hint != "" {
				return nil, fmt.Errorf("строка %d: у сущности %q нет состояния, записанного как %q - \"становится\" требует полную форму, согласованную по роду и творительным падежом (%q)", s.Line, typ.Entity, s.State, hint)
			}
			return nil, fmt.Errorf("строка %d: у сущности %q не объявлено состояние %q", s.Line, typ.Entity, s.State)
		}
		// Фиксируем доказанный переход - только для отслеживаемых форм
		// получателя (простая переменная/"этот", см. package doc ctx.
		// established); получатель-поле сущности молча не отслеживается
		// (безопасный дефолт "ничего не доказано" дальше по коду).
		switch r := receiver.(type) {
		case *ir.Identifier:
			next := cloneStates(sc.getEstablished(r.Name))
			next[base] = true
			sc.setEstablished(r.Name, next)
		case *ir.ThisExpr:
			next := cloneStates(sc.thisEstablished)
			next[base] = true
			sc.thisEstablished = next
		}
		return &ir.BecomeStmt{Receiver: receiver, State: base}, nil

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
		baseCtorSig := funcSig{Params: baseSig.CtorParams, ParamDefaults: baseSig.CtorParamDefaults}
		args, err := checkCallArgs(sc, s.Args, baseCtorSig, s.Line, fmt.Sprintf("конструктор %q", baseName), fmt.Sprintf("конструктора %q", baseName))
		if err != nil {
			return nil, err
		}
		return &ir.ParentCallStmt{BaseEntity: baseName, Args: args, Line: s.Line}, nil

	case *ast.ThrowStmt:
		return analyzeThrowStmt(sc, s)

	case *ast.TryStmt:
		return analyzeTryStmt(sc, s)

	case *ast.GerundStmt:
		return analyzeGerundStmt(sc, s)

	case *ast.SwitchStmt:
		return analyzeSwitchStmt(sc, s)

	default:
		return nil, fmt.Errorf("инструкция %T не поддерживается", s)
	}
}

// receiverAddressable - получатели, у которых в C-пути есть постоянный
// адрес (переменная/"этот"/доступ к полю) - результат вызова функции/
// конструктора/метода сюда не входит (временное значение). Для
// присваивания полю/индексации (FieldAssign, index.go) - жёсткое условие:
// присвоить полю временного значения бессмысленно в любом случае. Для
// вызова метода (analyzeMethodCallExpr) - только запасной признак: не
// подошёл сюда - ещё не ошибка, получатель-сущность БЕЗ IsStruct (ссылка)
// адресуемости не требует вовсе, явная ошибка Rech в реально неадресуемом
// случае лучше невнятной ошибки компилятора C.
func receiverAddressable(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Identifier, *ast.ThisExpr, *ast.FieldAccess:
		return true
	default:
		return false
	}
}

// suggestStateForm - written совпадает с краткой формой (значением
// entitySig.States, не ключом) какого-то состояния - типичная ошибка:
// программист написал краткую форму вместо полной, согласованной
// творительным падежом. Возвращает ожидаемую полную форму для сообщения об
// ошибке, "" - written не похоже ни на одну из известных кратких форм.
func suggestStateForm(states map[string]string, written string) string {
	for full, base := range states {
		if base == written {
			return full
		}
	}
	return ""
}
