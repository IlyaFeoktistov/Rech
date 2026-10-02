package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// blockDiverges - true, если body гарантированно не проваливается дальше
// (любой путь исполнения оканчивается "вернуть"/"выбросить" - "вернуть" без
// значения тоже расходится, в отличие от проверки "все пути возвращают
// значение", см. internal/semantic/flow.go). Используется ТОЛЬКО для
// сужения "?T" (established, IfStmt ниже) - "если а == Ничто { <расходится>
// }" БЕЗ явного "иначе" должно доказывать непустоту "а" для остального кода
// ПОСЛЕ "если", ровно как доказало бы "если а == Ничто {...} иначе
// {...использовать а...}" (НАЙДЕННОЕ И ИСПРАВЛЕННОЕ ограничение). Узкий
// анализ последней инструкции блока (и рекурсивно её веток), не полный CFG -
// в отличие от analyzeFlow/internal/ir/cfg (проверка "все пути возвращают
// значение"/недостижимый код), у этой конкретной задачи (сужение типа
// "?T" в established) нет требования распознавать НЕ последнюю инструкцию
// блока, поэтому полноценный граф здесь избыточен - оставлено как есть.
func blockDiverges(body []ir.Stmt) bool {
	if len(body) == 0 {
		return false
	}
	switch last := body[len(body)-1].(type) {
	case *ir.ReturnStmt:
		return true
	case *ir.ThrowStmt:
		return true
	case *ir.BreakStmt, *ir.ContinueStmt:
		// НАЙДЕННЫЙ по ходу examples/проекты/чат_сервер недочёт (docs/
		//): "если !соединение { продолжить }" не сужало
		// "соединение" для кода ПОСЛЕ "если" в том же теле цикла -
		// "прервать"/"продолжить" не давали упасть дальше НИЧУТЬ не хуже
		// "вернуть"/"выбросить" (код текстово после них в ЭТОМ теле
		// доказанно не достижим этим путём), просто не были в списке.
		// Оба валидны только внутри цикла (sc.loopDepth, проверено раньше
		// по конвейеру) - к моменту, когда эта функция вообще видит такую
		// инструкцию последней в блоке, она уже гарантированно корректна.
		return true
	case *ir.TryStmt:
		if len(last.Finally) > 0 && blockDiverges(last.Finally) {
			return true
		}
		if last.NoCatch {
			return blockDiverges(last.Body)
		}
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

// loopVar - переменная, которую цикл объявляет в области видимости тела
// (analyzeLoopBody).
type loopVar struct {
	name string
	typ  ir.Type
}

// analyzeLoopBody запускает тело цикла (пока/для/для-в) в новой области
// видимости и откатывает established к состоянию ДО цикла после выхода -
// цикл может не выполниться ни разу, поэтому established, доказанное
// только внутри тела, не гарантировано после цикла (см. package doc
// ctx.established). vars - переменные цикла (у "пока" их нет, у "для x в"
// одна, у "для к, з в" две), объявляются в той же новой области ДО
// анализа тела. Общая для WhileStmt/ForStmt/ForEachStmt.
func analyzeLoopBody(sc *ctx, body []ast.Stmt, line int, vars ...loopVar) ([]ir.Stmt, error) {
	preLoop := snapshotEstablished(sc.established)
	sc.push()
	sc.loopDepth++
	for _, v := range vars {
		if err := sc.declare(v.name, v.typ); err != nil {
			sc.loopDepth--
			sc.pop()
			return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: line})
		}
	}
	analyzed, err := analyzeBlock(sc, body)
	sc.loopDepth--
	sc.pop()
	if err != nil {
		return nil, err
	}
	sc.established = preLoop
	return analyzed, nil
}

// checkLoopJumpDepth проверяет числовой аргумент "прервать N"/"продолжить N"
// (PHP-стиль - не именованная метка, число уровней вложенности циклов,
// считая текущий) - N обязан укладываться в РЕАЛЬНУЮ вложенность циклов в
// этой точке (sc.loopDepth, уже >0 - проверено вызывающим кодом). word -
// "прервать"/"продолжить", только для текста ошибки.
func checkLoopJumpDepth(sc *ctx, word string, depth, line int) error {
	if depth > sc.loopDepth {
		return errAtLine(line, CodeStmtCheck, "\"%s %d\" - глубина вложенности циклов в этой точке только %d", word, depth, sc.loopDepth)
	}
	return nil
}

func analyzeStmt(sc *ctx, s ast.Stmt) (ir.Stmt, error) {
	switch s := s.(type) {
	case *ast.PrintStmt:
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		value = maybeUnwrapNarrowed(sc, value)
		typ := ir.TypeOf(value)
		if typ.Kind == ir.KindNullable {
			return nil, errAtLine(s.Line, CodeStmtCheck, "нельзя напрямую вывести значение nullable-типа %s - разверните его сначала (\"?? запасное\" или явный каст)", typ)
		}
		if wrapped := wrapStringerPrint(sc, value, typ); wrapped != nil {
			value = wrapped
		}
		return &ir.PrintStmt{Value: value, Line: s.Line}, nil

	case *ast.ShowStmt:
		value, err := analyzeExpr(sc, s.Value)
		if err != nil {
			return nil, err
		}
		value = maybeUnwrapNarrowed(sc, value)
		if typ := ir.TypeOf(value); typ.Kind == ir.KindNullable {
			return nil, errAtLine(s.Line, CodeStmtCheck, "нельзя напрямую показать значение nullable-типа %s - разверните его сначала (\"?? запасное\" или явный каст)", typ)
		}
		return &ir.ShowStmt{Value: value, Line: s.Line}, nil

	case *ast.VarDecl:
		var declared ir.Type
		var hasAnnotation bool
		if s.Type == bareCallableType {
			if s.Value == nil {
				return nil, errAtLine(s.Line, CodeStmtCheck, "у переменной %q без значения нужна полная сигнатура типа: \"Вызываемый(<параметры>): <результат> %s\"", s.Name, s.Name)
			}
			return analyzeBareCallableVarDecl(sc, s)
		}
		if s.Type != "" {
			t, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, s.Type)
			if err != nil {
				return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: s.Line})
			}
			declared, hasAnnotation = t, true
		}
		if s.Value == nil {
			return analyzeBareVarDecl(sc, s, declared)
		}
		var value ir.Expr
		var err error
		if hasAnnotation {
			value, err = coerceExpr(sc, s.Value, declared)
		} else {
			if err := uninferableVarError(s); err != nil {
				return nil, err
			}
			value, err = analyzeExpr(sc, s.Value)
		}
		if err != nil {
			return nil, err
		}
		if hasAnnotation {
			if typ := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, typ, declared) {
				return nil, errAtLine(s.Line, CodeStmtCheck, "переменная %q объявлена типом %s, но инициализирована значением типа %s", s.Name, declared, typ)
			}
		}
		// До sc.declare - иначе при затенении (переменная %s уже с этим же
		// именем во внешней области) established читался бы уже с только
		// что объявленной (пустой) внутренней переменной вместо той, на
		// которую на самом деле ссылается value.
		newEstablished := establishedForNewBinding(sc, s.Name, value)
		// Тип переменной - аннотация, если она есть: "Фигура ф = новый
		// Квадрат()" - переменная интерфейса, ей можно присвоить любую
		// другую фигуру, а поля Квадрата через неё не видны.
		varType := ir.TypeOf(value)
		if hasAnnotation {
			varType = declared
		}
		if err := sc.declare(s.Name, varType); err != nil {
			return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: s.Line})
		}
		sc.setEstablished(s.Name, newEstablished)
		var staticID int
		if s.IsStatic {
			sc.a.nextStaticVarID++
			staticID = sc.a.nextStaticVarID
		}
		decl := &ir.VarDecl{Name: s.Name, Value: value, IsStatic: s.IsStatic, StaticID: staticID, Line: s.Line, Doc: s.Doc}
		if hasAnnotation {
			decl.DeclType = declared
		}
		return decl, nil

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
			return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: s.Line})
		}
		sc.setEstablished(s.Name, newEstablished)
		sc.markConst(s.Name)
		return &ir.VarDecl{Name: s.Name, Value: value, Line: s.Line, IsConst: true, Doc: s.Doc}, nil

	case *ast.Assign:
		declared, exists := sc.lookup(s.Name)
		if !exists {
			return nil, errAtLine(s.Line, CodeNameResolution, "переменная %q не объявлена - объявите её: \"переменная %s = ...\" или \"<Тип> %s = ...\"", s.Name, s.Name, s.Name)
		}
		if sc.isConst(s.Name) {
			return nil, errAtLine(s.Line, CodeStmtCheck, "%q объявлено как \"константа\" - нельзя присвоить новое значение", s.Name)
		}
		value, err := coerceExpr(sc, s.Value, declared)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, typ, declared) {
			return nil, errAtLine(s.Line, CodeStmtCheck, "переменной %q типа %s нельзя присвоить значение типа %s", s.Name, declared, typ)
		}
		sc.setEstablished(s.Name, sc.withAssigned(s.Name, establishedForNewBinding(sc, s.Name, value)))
		// Захвачено лямбдой по ссылке - запись прозрачно идёт через
		// указатель (DerefAssignStmt), не напрямую - см. package doc
		// ir.CaptureVar/case *ast.Identifier выше (expr.go).
		if ptrType, isByRef := sc.byRefPointerType(s.Name); isByRef {
			return &ir.DerefAssignStmt{Pointer: &ir.Identifier{Name: s.Name, Type: ptrType}, Value: value, Line: s.Line}, nil
		}
		return &ir.Assign{Name: s.Name, Value: value, Line: s.Line}, nil

	case *ast.NullCoalesceAssign:
		return analyzeNullCoalesceAssignStmt(sc, s)

	case *ast.AssertStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, errAtLine(s.Line, CodeStmtCheck, "условие \"утверждать\" должно быть логическим, получено %s", typ)
		}
		var message ir.Expr
		if s.Message != nil {
			message, err = analyzeExpr(sc, s.Message)
			if err != nil {
				return nil, err
			}
			if typ := ir.TypeOf(message); typ != ir.TypeString {
				return nil, errAtLine(s.Line, CodeStmtCheck, "сообщение \"утверждать <условие>, <сообщение>\" должно быть Строка, получено %s", typ)
			}
		}
		return &ir.AssertStmt{Cond: cond, Message: message, Line: s.Line}, nil

	case *ast.IfStmt:
		cond, err := analyzeExpr(sc, s.Cond)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(cond); typ != ir.TypeBool {
			return nil, errAtLine(s.Line, CodeStmtCheck, "условие \"если\" должно быть логическим, получено %s", typ)
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
		// использовать а...}" (НАЙДЕННОЕ И
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
			return nil, errAtLine(s.Line, CodeStmtCheck, "условие \"пока\" должно быть логическим, получено %s", typ)
		}
		// Сужение nullable-типа в условии "пока а != Ничто {...}" - ТЕМ ЖЕ
		// принципом, что и "если а != Ничто {...}" (established, см. её же
		// комментарий у case *ast.IfStmt) - НАЙДЕННОЕ ограничение снято:
		// условие "пока" проверяется заново
		// ПЕРЕД КАЖДЫМ проходом тела, значит "а != Ничто" доказанно истинно
		// в начале ЛЮБОГО прохода, не только первого - никакого fixed-point
		// анализа по обратному ребру цикла не требуется, это прямое
		// следствие семантики "пока" (тело физически не может начать
		// выполняться, если условие в этот момент ложно) - та же гарантия,
		// что и у "если", просто проверяемая заново на каждом входе, а не
		// один раз.
		nc, isNullCheck := cond.(*ir.NullCheckExpr)
		var narrowedName string
		if isNullCheck {
			if ident, ok := nc.Value.(*ir.Identifier); ok {
				narrowedName = ident.Name
			}
		}
		// preWhile снят ДО markNonNull - established после цикла
		// восстанавливается К НЕМУ целиком (см. ниже) - сужение НЕ
		// переживает сам цикл: после выхода "а" вполне может быть Ничто
		// (именно оно и остановило "пока").
		preWhile := snapshotEstablished(sc.established)
		if isNullCheck && nc.IsNot && narrowedName != "" {
			markNonNull(sc, narrowedName)
		}
		// "пока" может не выполниться ни разу - established, доказанное
		// внутри тела (в т.ч. само сужение выше), не гарантировано ПОСЛЕ
		// цикла - откат к preWhile ниже, консервативный, не fixed-point
		// анализ по итерациям (см. package doc ctx.established); внутри
		// самого тела, в пределах одного прохода, накопленные по ходу
		// established всё ещё видны (обычный sc.push/analyzeBlock/sc.pop,
		// см. analyzeLoopBody).
		body, err := analyzeLoopBody(sc, s.Body, s.Line)
		sc.established = preWhile
		if err != nil {
			return nil, err
		}
		return &ir.WhileStmt{Cond: cond, Body: body, Line: s.Line}, nil

	case *ast.ForStmt:
		varType, err := rangeVarType(sc, s)
		if err != nil {
			return nil, err
		}
		from, err := rangeBound(sc, s.From, varType, "от", s.Line)
		if err != nil {
			return nil, err
		}
		to, err := rangeBound(sc, s.To, varType, "до", s.Line)
		if err != nil {
			return nil, err
		}
		body, err := analyzeLoopBody(sc, s.Body, s.Line, loopVar{s.Var, varType})
		if err != nil {
			return nil, err
		}
		return &ir.ForStmt{Var: s.Var, VarType: varType, From: from, To: to, Body: body, Line: s.Line}, nil

	case *ast.ForEachStmt:
		return analyzeForEachStmt(sc, s)

	case *ast.CStyleForStmt:
		// Своя область видимости (не analyzeLoopBody - переменной цикла
		// здесь ЗАРАНЕЕ не известен тип, в отличие от ForStmt/ForEachStmt:
		// его выводит analyzeStmt на самом Init, тем же путём, что и у
		// обычного VarDecl) - охватывает Init/Cond/Post/Body целиком, ровно
		// как в C ("для" вводит собственную область видимости, docs/
		// SPEC.md). Init/Cond/Post независимо могут отсутствовать (nil) -
		// "для ;; { ... }" бесконечен (package doc ast.CStyleForStmt) -
		// каждая секция анализируется, только если она есть.
		preLoop := snapshotEstablished(sc.established)
		sc.push()
		sc.loopDepth++
		var init *ir.VarDecl
		if s.Init != nil {
			initIR, err := analyzeStmt(sc, s.Init)
			if err != nil {
				sc.loopDepth--
				sc.pop()
				return nil, err
			}
			var ok bool
			init, ok = initIR.(*ir.VarDecl)
			if !ok {
				sc.loopDepth--
				sc.pop()
				return nil, errAtLine(s.Line, CodeStmtCheck, "инициализация цикла \"для\" должна объявлять переменную")
			}
		}
		var cond ir.Expr
		if s.Cond != nil {
			var err error
			cond, err = analyzeExpr(sc, s.Cond)
			if err != nil {
				sc.loopDepth--
				sc.pop()
				return nil, err
			}
			if typ := ir.TypeOf(cond); typ != ir.TypeBool {
				sc.loopDepth--
				sc.pop()
				return nil, errAtLine(s.Line, CodeStmtCheck, "условие цикла \"для\" должно быть логическим, получено %s", typ)
			}
		}
		var post ir.Stmt
		if s.Post != nil {
			var err error
			post, err = analyzeStmt(sc, s.Post)
			if err != nil {
				sc.loopDepth--
				sc.pop()
				return nil, err
			}
		}
		body, err := analyzeBlock(sc, s.Body)
		sc.loopDepth--
		sc.pop()
		if err != nil {
			return nil, err
		}
		sc.established = preLoop
		return &ir.CStyleForStmt{Init: init, Cond: cond, Post: post, Body: body, Line: s.Line}, nil

	case *ast.BreakStmt:
		if sc.loopDepth == 0 {
			return nil, errAtLine(s.Line, CodeStmtCheck, "\"прервать\" допустимо только внутри цикла")
		}
		if err := checkLoopJumpDepth(sc, "прервать", s.Depth, s.Line); err != nil {
			return nil, err
		}
		return &ir.BreakStmt{Depth: s.Depth, Line: s.Line}, nil

	case *ast.ContinueStmt:
		if sc.loopDepth == 0 {
			return nil, errAtLine(s.Line, CodeStmtCheck, "\"продолжить\" допустимо только внутри цикла")
		}
		if err := checkLoopJumpDepth(sc, "продолжить", s.Depth, s.Line); err != nil {
			return nil, err
		}
		return &ir.ContinueStmt{Depth: s.Depth, Line: s.Line}, nil

	case *ast.ReturnStmt:
		if s.Value == nil {
			if sc.hasReturn {
				return nil, errAtLine(s.Line, CodeStmtCheck, "функция с объявленным типом результата должна возвращать значение (\"вернуть <значение>\")")
			}
			return &ir.ReturnStmt{Line: s.Line}, nil
		}
		if sc.inCtor {
			return nil, errAtLine(s.Line, CodeNameResolution, "не возвращает значения - используйте \"вернуть\" без значения")
		}
		if !sc.hasReturn {
			return nil, errAtLine(s.Line, CodeNameResolution, "у функции не объявлен тип результата - используйте \"вернуть\" без значения")
		}
		value, err := coerceExpr(sc, s.Value, sc.returnType)
		if err != nil {
			return nil, err
		}
		if typ := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, typ, sc.returnType) {
			return nil, errAtLine(s.Line, CodeStmtCheck, "функция объявлена с типом результата %s, но возвращается значение типа %s", sc.returnType, typ)
		}
		return &ir.ReturnStmt{Value: value, Line: s.Line}, nil

	case *ast.ExprStmt:
		if s.CallValue != nil {
			call, err := analyzeCallResultCall(sc, s.CallValue)
			if err != nil {
				return nil, err
			}
			return &ir.ExprStmt{CallValue: call, Line: s.Line}, nil
		}
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
			if desugared, handled, derr := maybeDesugarHandleMethod(sc, s.MethodCall); handled {
				if derr != nil {
					return nil, derr
				}
				switch call := desugared.(type) {
				case *ir.CallExpr:
					return &ir.ExprStmt{Call: call, Line: s.Line}, nil
				case *ir.CallValueExpr:
					return &ir.ExprStmt{CallValue: call, Line: s.Line}, nil
				default:
					return nil, errNoPos(diagnostics.CodeInternal, "внутренняя ошибка: неизвестный вид вызова %T", call)
				}
			}
			receiver, err := analyzeMethodReceiver(sc, s.MethodCall)
			if err != nil {
				return nil, err
			}
			if ir.TypeOf(receiver).Kind == ir.KindPointer && s.MethodCall.Method == "установить" {
				return analyzePointerStore(sc, receiver, s.MethodCall)
			}
			if ir.TypeOf(receiver).Kind == ir.KindArray && s.MethodCall.Method == "установить" {
				return analyzeArrayStore(sc, receiver, s.MethodCall)
			}
			call, err := analyzeMethodCallOn(sc, receiver, s.MethodCall)
			if err != nil {
				return nil, err
			}
			switch call := call.(type) {
			case *ir.MethodCallExpr:
				return &ir.ExprStmt{MethodCall: call, Line: s.Line}, nil
			case *ir.CallValueExpr:
				return &ir.ExprStmt{CallValue: call, Line: s.Line}, nil
			default:
				return nil, errNoPos(diagnostics.CodeInternal, "внутренняя ошибка: неизвестный вид вызова %T", call)
			}
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
			return nil, errNoPos(diagnostics.CodeInternal, "внутренняя ошибка: неизвестный вид вызова %T", call)
		}

	case *ast.FieldAssign:
		if s.Static {
			return analyzeStaticFieldAssign(sc, s)
		}
		if base, levels, ok := elementFieldTarget(sc, s.Receiver); ok {
			return analyzeElementFieldAssign(sc, s, base, levels)
		}
		// "п.a = 1" у структуры без значения - запись поля, не чтение "п"
		// (bare_struct_fields.go).
		var bareName string
		var receiver ir.Expr
		if id, typ, bsig, ok := bareStruct(sc, s.Receiver); ok {
			if !bsig.IsStruct {
				return nil, errAtLine(s.Line, CodeStmtCheck, "%q - сущность без значения: сначала создайте её (%s = %s %s(...)), поля появляются вместе с объектом", id.Name, id.Name, newWordFor(typ.Entity), typ.Entity)
			}
			bareName, receiver = id.Name, &ir.Identifier{Name: id.Name, Type: typ}
		} else {
			var err error
			receiver, err = analyzeExpr(sc, s.Receiver)
			if err != nil {
				return nil, err
			}
		}
		typ := ir.TypeOf(receiver)
		if typ.Kind != ir.KindEntity {
			return nil, errAtLine(s.Line, CodeStmtCheck, "нельзя присвоить полю %q - тип %s не сущность", s.Field, typ)
		}
		sig := sc.entities[typ.Entity]
		if sig.IsStruct && isCallResult(s.Receiver) {
			// Структура - значение: результат вызова - копия, запись в её
			// поле никуда не попадёт.
			return nil, errAtLine(s.Line, CodeStmtCheck, "поле %q присваивается копии структуры %s - результату вызова: изменение потеряется", s.Field, typ.Entity).
				WithHint("сохраните её в переменную, измените поле и запишите обратно")
		}
		// Получатель НЕ обязан быть адресуемым на уровне семантики - как и у
		// analyzeMethodCallExpr, cbackend сам материализует временную
		// переменную для получателя-"структуры" без постоянного адреса (см.
		// generateIRAddressableExpr/materializeIfNeeded). Получателем годится
		// результат ЛЮБОГО вызова, сколь угодно вложенно -
		// `x.а().б().поле = значение`.

		// Нельзя присваивать значение константе сущности
		if cidx := sig.constIndex(s.Field); cidx != -1 {
			return nil, errAtLine(s.Line, CodeStmtCheck, "нельзя присвоить значение константе %q.%q", typ.Entity, s.Field)
		}

		idx := sig.fieldIndex(s.Field)
		if idx == -1 {
			if sig.staticFieldIndex(s.Field) != -1 {
				return nil, errAtLine(s.Line, CodeStmtCheck, "%q - статическое поле сущности %q, присваивать только через \"%s::%s\", не через экземпляр", s.Field, typ.Entity, typ.Entity, s.Field)
			}
			d := errAtLine(s.Line, CodeNameResolution, "у сущности %q нет поля %q", typ.Entity, s.Field)
			if hint := suggestNameHint(s.Field, sig.FieldNames); hint != "" {
				d.WithHint("%s", hint)
			}
			return nil, d
		}
		if err := checkFieldVisibility(sc, typ.Entity, s.Field, sig.FieldVisibility[idx], s.Line); err != nil {
			return nil, err
		}
		value, err := coerceExpr(sc, s.Value, sig.FieldTypes[idx])
		if err != nil {
			return nil, err
		}
		if vt := ir.TypeOf(value); !isAssignable(sc.entities, sc.interfaces, vt, sig.FieldTypes[idx]) {
			return nil, errAtLine(s.Line, CodeStmtCheck, "полю %q типа %s нельзя присвоить значение типа %s", s.Field, sig.FieldTypes[idx], vt)
		}
		if bareName != "" {
			sc.markFieldAssigned(bareName, s.Field, sig)
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
			return nil, errAtLine(s.Line, CodeStmtCheck, "\"становится\" применимо только к сущности, получен тип %s", typ)
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
				return nil, errAtLine(s.Line, CodeStmtCheck, "у сущности %q нет состояния, записанного как %q - \"становится\" требует полную форму, согласованную по роду и творительным падежом (%q)", typ.Entity, s.State, hint)
			}
			return nil, errAtLine(s.Line, CodeNameResolution, "у сущности %q не объявлено состояние %q", typ.Entity, s.State)
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
			return nil, errAtLine(s.Line, CodeStmtCheck, "\"родитель(...)\" допустимо только первой инструкцией тела конструктора")
		}
		sc.allowParentCall = false
		baseName := sc.entities[sc.thisEntity].BaseName
		if baseName == "" {
			return nil, errAtLine(s.Line, CodeStmtCheck, "у сущности %q нет базовой сущности - \"родитель(...)\" вызывать нечего", sc.thisEntity)
		}
		baseSig := sc.entities[baseName]
		if !baseSig.HasCtor {
			return nil, errAtLine(s.Line, CodeStmtCheck, "у базовой сущности %q нет собственного конструктора - вызывать нечего, присвойте унаследованные поля напрямую через \"этот.поле = ...\"", baseName)
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
		return nil, errNoPos(diagnostics.CodeInternal, "инструкция %T не поддерживается", s)
	}
}

// receiverAddressable - получатели, у которых в C-пути есть постоянный
// адрес (переменная/"этот"/доступ к полю), без материализации во
// временную переменную. Используется только для безопасной индексации
// ("receiver[index] = значение", index.go) - там вложенные цепочки
// сознательно не поддержаны (см. package doc parseIndexAssign), поэтому
// проверка жёсткая. Вызов метода и присваивание полю (analyzeMethodCallExpr/
// analyzeFieldAssign) больше НЕ используют эту функцию как ограничение -
// получатель там любой, cbackend сам материализует временную переменную,
// когда получатель ("структура") не адресуем напрямую (см.
// generateIRAddressableExpr/materializeIfNeeded,
// internal/backend/cbackend/ir_generator.go).
func receiverAddressable(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Identifier, *ast.ThisExpr, *ast.FieldAccess:
		return true
	case *ast.DerefExpr:
		// "(*п)" - то же хранилище, что держит п.
		return receiverAddressable(v.Value)
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
