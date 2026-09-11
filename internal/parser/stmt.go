package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

func (p *Parser) parseBlock() ([]ast.Stmt, error) {
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	// Разделитель сразу после "{" необязателен - расстановка скобки на той
	// же строке или с переносом - вопрос стиля, а не грамматики.
	p.skipSeparators()

	savedPending := p.beginPendingScope()
	var stmts []ast.Stmt
	for p.cur().Type != lexer.RBRACE {
		if p.cur().Type == lexer.EOF {
			return nil, fmt.Errorf("строка %d: не найдена закрывающая }", p.cur().Line)
		}
		if p.cur().Type == lexer.DOC_COMMENT {
			// "/** ... */" внутри тела - не сразу перед объявлением
			// (consumeDoc уже забрал бы его в соответствующей точке
			// разбора, если бы был) - ведёт себя как обычный комментарий,
			// просто пропускается, ни к чему не привязываясь.
			p.advance()
			p.skipSeparators()
			continue
		}
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			// nil - "переменная X" без инициализации (pendingvar.go): не
			// порождает узел AST сама по себе, только регистрирует
			// отложенное объявление в этом блоке.
			stmts = append(stmts, stmt)
		}
		if p.cur().Type == lexer.RBRACE {
			break
		}
		if err := p.requireSeparator(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	if err := p.endPendingScope(savedPending); err != nil {
		return nil, err
	}
	return stmts, nil
}

// parseStatement разбирает одну инструкцию тела функции/метода (а также
// инструкцию верхнего уровня программы, см. ParseProgram). Модификаторы
// видимости ("приватный"/"публичный") здесь не ожидаются вообще - видимость
// имеет смысл только для константы/поля/метода внутри "сущность" (см.
// parseEntity) - попытка написать их перед обычной инструкцией падает в
// default ниже с понятной ошибкой "неожиданный токен".
func (p *Parser) parseStatement() (ast.Stmt, error) {
	switch p.cur().Type {
	case lexer.KwPrint:
		return p.parsePrintStmt()
	case lexer.KwShow:
		return p.parseShowStmt()
	case lexer.KwAssert:
		return p.parseAssertStmt()
	case lexer.KwVariable, lexer.KwStatic:
		return p.parseVarDecl()
	case lexer.KwConst:
		return p.parseConstDecl()
	case lexer.KwIf:
		return p.parseIfStmt()
	case lexer.KwSwitch:
		return p.parseSwitchStmt()
	case lexer.KwWhile:
		return p.parseWhileStmt()
	case lexer.KwFor:
		return p.parseForStmt()
	case lexer.KwBreak:
		tok := p.advance()
		depth, err := p.parseOptionalLoopDepth()
		if err != nil {
			return nil, err
		}
		return &ast.BreakStmt{Depth: depth, Line: tok.Line}, nil
	case lexer.KwContinue:
		tok := p.advance()
		depth, err := p.parseOptionalLoopDepth()
		if err != nil {
			return nil, err
		}
		return &ast.ContinueStmt{Depth: depth, Line: tok.Line}, nil
	case lexer.KwReturn:
		return p.parseReturnStmt()
	case lexer.KwParent:
		return p.parseParentCallStmt()
	case lexer.KwThrow:
		return p.parseThrowStmt()
	case lexer.KwAwait:
		return p.parseAwaitStmt()
	case lexer.KwTry:
		return p.parseTryStmt()
	case lexer.INCR, lexer.DECR:
		expr, err := p.parsePrefixIncDec()
		if err != nil {
			return nil, err
		}
		incDec := expr.(*ast.IncDecExpr)
		return &ast.ExprStmt{IncDec: incDec, Line: incDec.Line}, nil
	case lexer.STAR:
		// "*Тип имя = значение" (объявление через указатель-сахар, "Указатель<
		// Тип>" - tryParseTypedVarDecl) неотличимо от "*p = значение"
		// (разыменование, parseDerefAssignStmt) только по первому токену "*" -
		// разница появляется дальше (один IDENT до "=" - разыменование, два -
		// объявление), тем же принципом, что и в case IDENT ниже.
		if decl, ok, err := p.tryParseTypedVarDecl(); err != nil {
			return nil, err
		} else if ok {
			return decl, nil
		}
		return p.parseDerefAssignStmt()
	case lexer.QUESTION:
		// "?Тип имя = значение" (nullable-объявление без слова "переменная") -
		// у "?" нет никакого другого смысла в начале инструкции, поэтому здесь
		// либо tryParseTypedVarDecl совпадёт, либо разбор упадёт с обычной
		// ошибкой "неожиданный токен" ниже по стеку (сама tryParseTypedVarDecl
		// откатывает позицию перед этим).
		if decl, ok, err := p.tryParseTypedVarDecl(); err != nil {
			return nil, err
		} else if ok {
			return decl, nil
		}
		return nil, fmt.Errorf("строка %d, столбец %d: неожиданный токен %q в теле функции",
			p.cur().Line, p.cur().Col, p.cur().Value)
	case lexer.IDENT:
		if decl, ok, err := p.tryParseTypedVarDecl(); err != nil {
			return nil, err
		} else if ok {
			return decl, nil
		}
		if p.peekAt(1).Type == lexer.ASSIGN || isCompoundAssignTok(p.peekAt(1).Type) {
			return p.parseAssign()
		}
		if p.peekAt(1).Type == lexer.NULL_ASSIGN {
			return p.parseNullCoalesceAssign()
		}
		if p.peekAt(1).Type == lexer.LBRACKET {
			return p.parseIndexAssign()
		}
		if p.peekAt(1).Type == lexer.DOT || p.peekAt(1).Type == lexer.DBLCOLON {
			return p.parseDotStatement()
		}
		if p.peekAt(1).Type == lexer.KwBecomes {
			return p.parseBecomeStmt()
		}
		if p.peekAt(1).Type == lexer.INCR || p.peekAt(1).Type == lexer.DECR {
			name := p.advance()
			op := p.advance()
			incDec := &ast.IncDecExpr{Name: name.Value, Op: op.Value, Prefix: false, Line: name.Line}
			return &ast.ExprStmt{IncDec: incDec, Line: name.Line}, nil
		}
		if lemma, ok := p.gerundWords[p.cur().Value]; ok && p.peekAt(1).Type == lexer.LPAREN {
			return p.parseGerundStmt(lemma)
		}
		if p.peekAt(1).Type == lexer.LPAREN {
			call, err := p.parseCallExpr()
			if err != nil {
				return nil, err
			}
			return p.finishCallChainStmt(call)
		}
		if p.genericFuncs[p.cur().Value] && p.peekAt(1).Type == lexer.LT {
			call, err := p.parseGenericCallExpr()
			if err != nil {
				return nil, err
			}
			return p.finishCallChainStmt(call)
		}
		if params, ok := p.funcParams[p.cur().Value]; ok {
			call, err := p.parseNaturalCall(params)
			if err != nil {
				return nil, err
			}
			return p.parseNaturalCallContinuation(call)
		}
		if params, ok := p.startsNaturalMethodCall(); ok {
			call, err := p.parseNaturalMethodCall(params)
			if err != nil {
				return nil, err
			}
			return p.parseNaturalMethodCallContinuation(call)
		}
		return nil, fmt.Errorf("строка %d, столбец %d: неожиданный токен %q в теле функции",
			p.cur().Line, p.cur().Col, p.cur().Value)
	case lexer.KwThis:
		if p.peekAt(1).Type == lexer.DOT {
			return p.parseDotStatement()
		}
		if p.peekAt(1).Type == lexer.KwBecomes {
			return p.parseBecomeStmt()
		}
		if params, ok := p.startsNaturalMethodCall(); ok {
			call, err := p.parseNaturalMethodCall(params)
			if err != nil {
				return nil, err
			}
			return p.parseNaturalMethodCallContinuation(call)
		}
		return nil, fmt.Errorf("строка %d, столбец %d: \"этот\" в начале инструкции допустимо только с полем, методом или переходом состояния (\"этот.поле = ...\"/\"этот.метод(...)\"/\"этот становится ...\")",
			p.cur().Line, p.cur().Col)
	default:
		return nil, fmt.Errorf("строка %d, столбец %d: неожиданный токен %q в теле функции",
			p.cur().Line, p.cur().Col, p.cur().Value)
	}
}

// parseOptionalLoopDepth разбирает необязательный числовой аргумент после
// "прервать"/"продолжить" ("прервать 2" - прервать два уровня вложенности
// циклов, считая текущий) - PHP-стиль "break N"/"continue N", не именованная
// метка. Без аргумента (следующий токен - не INT, обычно конец инструкции)
// возвращает 1 - ближайший охватывающий цикл, как и раньше. Сама глубина
// (не выходит ли она за пределы реальной вложенности циклов в этой точке)
// проверяется позже, семантическим анализом (sc.loopDepth) - здесь только
// синтаксис.
func (p *Parser) parseOptionalLoopDepth() (int, error) {
	if p.cur().Type != lexer.INT {
		return 1, nil
	}
	tok := p.advance()
	v, err := parseIntTokenValue(tok.Value)
	if err != nil {
		return 0, fmt.Errorf("строка %d, столбец %d: некорректный целочисленный литерал %q", tok.Line, tok.Col, tok.Value)
	}
	if v < 1 {
		return 0, fmt.Errorf("строка %d: глубина вложенности цикла должна быть не меньше 1, получено %d", tok.Line, v)
	}
	return int(v), nil
}

// parseDotStatement разбирает инструкцию, начинающуюся с получателя (IDENT
// или "этот"), за которым следует "." - присваивание полю (receiver.поле =
// выражение, в том числе вложенное - "x.a.b = значение", получатель
// присваивания тогда "x.a" целиком, см. finishFieldAssign), вызов метода
// как самостоятельная инструкция (receiver.метод(...)) - в том числе
// цепочкой (receiver.метод1().метод2()) - или присваивание полю ВЛОЖЕННОЙ
// сущности через цепочку методов ("x.метод().поле = значение" - недопустимо,
// как и раньше: получатель присваивания полю обязан быть переменной/полем/
// "этот", не результатом вызова - см. FieldAssign в internal/semantic).
// Позиция парсера на входе - на получателе.
// finishCallChainStmt - как parseDotStatement, но для инструкции,
// начинающейся не с получателя (IDENT/"этот"), а с уже разобранного вызова
// СВОБОДНОЙ функции (`имя(...)`/generic-вызов `имя<Тип>(...)`) - результат
// такого вызова тоже годится базой для цепочки "."/"::"
// (`создать_логгер().ошибка(...)`, `создать().а().поле = значение`), тем
// же принципом, что и у результата вызова метода (см.
// analyzeMethodCallExpr/analyzeFieldAssign, internal/semantic). Без
// продолжения - просто вызов как самостоятельная инструкция, как и раньше.
func (p *Parser) finishCallChainStmt(call *ast.CallExpr) (ast.Stmt, error) {
	if p.cur().Type != lexer.DOT && p.cur().Type != lexer.DBLCOLON {
		return &ast.ExprStmt{Call: call, Line: call.Line}, nil
	}
	startLine, startCol := p.cur().Line, p.cur().Col
	expr, err := p.parsePostfixTail(call)
	if err != nil {
		return nil, err
	}
	switch e := expr.(type) {
	case *ast.MethodCallExpr:
		return &ast.ExprStmt{MethodCall: e, Line: e.Line}, nil
	case *ast.FieldAccess:
		opTok := p.cur()
		if opTok.Type == lexer.ASSIGN || isCompoundAssignTok(opTok.Type) {
			return p.finishFieldAssign(e.Receiver, e.Field, e.Static, e.Line)
		}
	}
	return nil, fmt.Errorf("строка %d, столбец %d: доступ к полю сам по себе не является инструкцией - нужен вызов метода или присваивание",
		startLine, startCol)
}

func (p *Parser) parseDotStatement() (ast.Stmt, error) {
	if p.peekAt(3).Type == lexer.ASSIGN || isCompoundAssignTok(p.peekAt(3).Type) {
		return p.parseFieldAssign()
	}
	start := p.cur()
	expr, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	switch e := expr.(type) {
	case *ast.MethodCallExpr:
		return &ast.ExprStmt{MethodCall: e, Line: e.Line}, nil
	case *ast.FieldAccess:
		opTok := p.cur()
		if opTok.Type == lexer.ASSIGN || isCompoundAssignTok(opTok.Type) {
			return p.finishFieldAssign(e.Receiver, e.Field, e.Static, e.Line)
		}
	}
	return nil, fmt.Errorf("строка %d, столбец %d: доступ к полю сам по себе не является инструкцией - нужен вызов метода или присваивание",
		start.Line, start.Col)
}

func (p *Parser) parsePrintStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwPrint)
	if err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("после \"вывести\" ожидалось выражение: %w", err)
	}
	return &ast.PrintStmt{Value: value, Line: kw.Line}, nil
}

func (p *Parser) parseShowStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwShow)
	if err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("после \"показать\" ожидалось выражение: %w", err)
	}
	return &ast.ShowStmt{Value: value, Line: kw.Line}, nil
}

func (p *Parser) parseAssertStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwAssert)
	if err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("после \"утверждать\" ожидалось выражение: %w", err)
	}
	// "утверждать <условие>, <сообщение>" - необязательное второе
	// выражение (внятные сообщения о несовпадении),
	// разбирается ровно как второй аргумент вызова - через ту же запятую.
	var message ast.Expr
	if p.cur().Type == lexer.COMMA {
		p.advance()
		message, err = p.parseExpr()
		if err != nil {
			return nil, fmt.Errorf("после \"утверждать <условие>,\" ожидалось выражение сообщения: %w", err)
		}
	}
	return &ast.AssertStmt{Cond: cond, Message: message, Line: kw.Line}, nil
}

// parseVarDecl разбирает "переменная <имя> = <выражение>" - тип выводится
// из Value, как раньше - либо, начиная с этой сессии, "переменная <имя>: <Тип>
// = <выражение>" с явной аннотацией типа (нужна там, куда тип неоткуда
// вывести из самого Value: "Ничто" у nullable-переменной, пустой литерал
// списка "[]" - см. ast.VarDecl.Type).
func (p *Parser) parseVarDecl() (ast.Stmt, error) {
	isStatic := false
	if p.cur().Type == lexer.KwStatic {
		tok := p.advance()
		// "переменная" - существительное женского рода, модификатор
		// согласуется с НИМ, не с именем объявляемой переменной (в отличие
		// от "статический"/"приватный" у поля сущности) - единственная
		// правильная форма всегда "статическая".
		if tok.Value != "статическая" {
			return nil, fmt.Errorf("строка %d, столбец %d: перед \"переменная\" нужна форма \"статическая\", получено %q", tok.Line, tok.Col, tok.Value)
		}
		isStatic = true
	}
	kw, err := p.expect(lexer.KwVariable)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"переменная\" ожидалось имя переменной: %w", err)
	}
	var typeName string
	if p.cur().Type == lexer.COLON {
		p.advance()
		typeName, err = p.parseTypeName()
		if err != nil {
			return nil, fmt.Errorf("в аннотации типа переменной %q: %w", name.Value, err)
		}
	}
	if p.cur().Type != lexer.ASSIGN && typeName == "" {
		if isStatic {
			return nil, fmt.Errorf("строка %d, столбец %d: \"статическая переменная\" должна содержать инициализацию (\"=\") - без неё непонятно, каким значением она заполнится ровно один раз", p.cur().Line, p.cur().Col)
		}
		// Отложенное объявление (pendingvar.go, "переменная X" без "=") -
		// настоящий ast.VarDecl появится позже, в точке первого
		// присваивания X в этом же блоке. Типизированное "переменная X:
		// Тип" без значения этим не покрывается - остаётся ошибкой ниже,
		// как и раньше (тип без значения означал бы отдельный вопрос
		// zero-значений, который эта фича не решает).
		p.registerPendingVarDecl(name)
		return nil, nil
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, fmt.Errorf("объявление переменной %q должно содержать инициализацию (\"=\"): %w", name.Value, err)
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в инициализации переменной %q: %w", name.Value, err)
	}
	p.registerNoun(name.Value)
	if typeName != "" {
		p.registerVarEntityType(name.Value, typeName)
	}
	return &ast.VarDecl{Name: name.Value, Type: typeName, Value: value, IsStatic: isStatic, Line: kw.Line}, nil
}

// parseConstDecl - "константа <имя> = <значение>" (шаг 12) - синтаксически
// идентично parseVarDecl (та же обязательная инициализация, та же
// регистрация имени для падежного согласования) - разница (запрет
// повторного присваивания) проверяется в internal/semantic, не здесь.
// Модификатор видимости сюда не входит - его читает вызывающая сторона
// (parseEntity) до вызова этой функции, см. checkConstVisibilityAgreement.
func (p *Parser) parseConstDecl() (*ast.ConstDecl, error) {
	kw, err := p.expect(lexer.KwConst)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"константа\" ожидалось имя константы: %w", err)
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, fmt.Errorf("объявление константы %q должно содержать инициализацию (\"=\"): %w", name.Value, err)
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в инициализации константы %q: %w", name.Value, err)
	}
	p.registerNoun(name.Value)
	return &ast.ConstDecl{Name: name.Value, Value: value, Line: kw.Line}, nil
}

// compoundAssignOps - составные операторы присваивания ("+=" и т. п.) ->
// базовый бинарный оператор, которым парсер разворачивает их в обычный
// "<имя> = <имя> <оператор> <выражение>" (ast.Assign/ast.FieldAssign с
// ast.BinaryExpr внутри) - никакого отдельного узла AST/IR не заводится,
// вся остальная семантика/кодогенерация уже умеет работать с обычным
// присваиванием и бинарной арифметикой (в т.ч. "+" для Строка -
// конкатенация, поэтому "+=" работает и для строк без отдельного случая).
var compoundAssignOps = map[lexer.TokenType]string{
	lexer.PLUS_ASSIGN:    "+",
	lexer.MINUS_ASSIGN:   "-",
	lexer.STAR_ASSIGN:    "*",
	lexer.SLASH_ASSIGN:   "/",
	lexer.PERCENT_ASSIGN: "%",
}

func isCompoundAssignTok(t lexer.TokenType) bool {
	_, ok := compoundAssignOps[t]
	return ok
}

// parseDerefAssignStmt разбирает "*указатель = значение" - позиция парсера
// на входе - на первой "*". Указатель - обычное унарное выражение
// (parseUnary), поэтому "**p = значение" (запись через один уровень
// разыменования указателя на указатель) работает без отдельного случая -
// Pointer получает "*p" (DerefExpr), не голое "p".
func (p *Parser) parseDerefAssignStmt() (ast.Stmt, error) {
	star := p.advance() // "*"
	pointer, err := p.parseUnary()
	if err != nil {
		return nil, fmt.Errorf("после \"*\" ожидался указатель: %w", err)
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, fmt.Errorf("после \"*<указатель>\" ожидалось \"=\": %w", err)
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в присваивании через указатель: %w", err)
	}
	return &ast.DerefAssignStmt{Pointer: pointer, Value: value, Line: star.Line}, nil
}

func (p *Parser) parseAssign() (ast.Stmt, error) {
	name := p.advance()
	opTok := p.cur()
	if opTok.Type != lexer.ASSIGN && !isCompoundAssignTok(opTok.Type) {
		return nil, fmt.Errorf("строка %d, столбец %d: ожидался \"=\" в присваивании %q, получено %q", opTok.Line, opTok.Col, name.Value, opTok.Value)
	}
	p.advance()
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в присваивании %q: %w", name.Value, err)
	}
	if baseOp, ok := compoundAssignOps[opTok.Type]; ok {
		// Составное присваивание всегда читает старое значение name -
		// отложенное объявление (pendingvar.go) им заведомо не разрешается
		// (у него ещё нет значения, которое можно было бы прочитать),
		// resolveAssignTarget здесь не нужен.
		value = &ast.BinaryExpr{Op: baseOp, Left: &ast.Identifier{Name: name.Value, Line: name.Line}, Right: value, Line: opTok.Line}
		return &ast.Assign{Name: name.Value, Value: value, Line: name.Line}, nil
	}
	return p.resolveAssignTarget(name, value), nil
}

// parseNullCoalesceAssign разбирает "а ?= б" (null-coalescing присваивание,
// только простое имя переменной - как у parseAssign, не поле сущности).
func (p *Parser) parseNullCoalesceAssign() (ast.Stmt, error) {
	name := p.advance()
	p.advance() // "?="
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в присваивании %q: %w", name.Value, err)
	}
	return &ast.NullCoalesceAssign{Name: name.Value, Value: value, Line: name.Line}, nil
}

// parseIndexAssign разбирает "receiver[индекс] = значение" - сахар над
// "receiver.установить(индекс, значение)" (genericMethodSigs - "установить"
// есть у Список/Словарь в одинаковой форме "позиция/ключ, значение") - сразу
// строит ast.MethodCallExpr, отдельного AST-узла для этой формы не заводится
// вообще. Receiver - ровно IDENT (та же граница, что и у parseFieldAssign
// ниже) - вложенные цепочки ("матрица[i][j] = x") не поддержаны в этом
// срезе: "получить строку, затем установить в неё" уже работает как есть
// через отдельные .получить/.установить (Список - ссылочная семантика
// элемента, строка остаётся тем же объектом).
func (p *Parser) parseIndexAssign() (ast.Stmt, error) {
	receiverTok := p.advance() // IDENT
	receiver := &ast.Identifier{Name: receiverTok.Value, Line: receiverTok.Line}
	p.advance() // "["
	index, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в индексе \"[...]\": %w", err)
	}
	if _, err := p.expect(lexer.RBRACKET); err != nil {
		return nil, fmt.Errorf("после индекса ожидалось \"]\": %w", err)
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, fmt.Errorf("после %q[...] ожидалось \"=\" (индексация сама по себе не инструкция): %w", receiverTok.Value, err)
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в присваивании по индексу: %w", err)
	}
	call := &ast.MethodCallExpr{Receiver: receiver, Method: "установить", Args: []ast.Expr{index, value}, Line: receiverTok.Line}
	return &ast.ExprStmt{MethodCall: call, Line: receiverTok.Line}, nil
}

// parseFieldAssign разбирает "receiver.поле = выражение" - receiver РОВНО
// один уровень (IDENT переменной или "этот"), без вложенности - самый
// частый случай, отдельный быстрый путь без общей parsePostfix. Позиция
// парсера на входе - на получателе. Присваивание в поле ВЛОЖЕННОЙ сущности
// (`x.a.b = значение`) идёт другим путём - см. parseDotStatement,
// finishFieldAssign.
func (p *Parser) parseFieldAssign() (ast.Stmt, error) {
	receiverTok := p.advance() // IDENT получателя или "этот"
	var receiver ast.Expr
	if receiverTok.Type == lexer.KwThis {
		if !p.inMethodBody {
			return nil, fmt.Errorf("строка %d, столбец %d: \"этот\" валиден только внутри тела метода", receiverTok.Line, receiverTok.Col)
		}
		receiver = &ast.ThisExpr{Line: receiverTok.Line}
	} else {
		receiver = &ast.Identifier{Name: receiverTok.Value, Line: receiverTok.Line}
	}
	static := p.cur().Type == lexer.DBLCOLON
	p.advance() // "." или "::"
	field, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \".\" ожидалось имя поля: %w", err)
	}
	return p.finishFieldAssign(receiver, field.Value, static, receiverTok.Line)
}

// finishFieldAssign разбирает хвост присваивания полю - "= выражение" (или
// составной оператор, "+=" и т. п.) - после того как получатель и имя поля
// уже разобраны. Общий для parseFieldAssign (получатель - ровно один
// уровень) и parseDotStatement (получатель - результат parsePostfix,
// возможно вложенный - `x.a.b = значение`: `receiver` здесь - уже
// `x.a` целиком, `field` - "b").
func (p *Parser) finishFieldAssign(receiver ast.Expr, field string, static bool, line int) (ast.Stmt, error) {
	opTok := p.cur()
	if opTok.Type != lexer.ASSIGN && !isCompoundAssignTok(opTok.Type) {
		return nil, fmt.Errorf("строка %d, столбец %d: ожидался \"=\" в присваивании полю %q, получено %q", opTok.Line, opTok.Col, field, opTok.Value)
	}
	p.advance()
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в присваивании полю %q: %w", field, err)
	}
	if baseOp, ok := compoundAssignOps[opTok.Type]; ok {
		value = &ast.BinaryExpr{Op: baseOp, Left: &ast.FieldAccess{Receiver: receiver, Field: field, Static: static, Line: line}, Right: value, Line: opTok.Line}
	}
	return &ast.FieldAssign{Receiver: receiver, Field: field, Static: static, Value: value, Line: line}, nil
}

func (p *Parser) parseIfStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwIf)
	if err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("после \"если\" ожидалось условие: %w", err)
	}
	p.skipNewlines()
	thenBlock, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	stmt := &ast.IfStmt{Cond: cond, Then: thenBlock, Line: kw.Line}

	if p.peekPastNewlines().Type == lexer.KwElse {
		p.skipNewlines()
		p.advance() // "иначе"
		p.skipNewlines()
		elseBlock, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		stmt.Else = elseBlock
	}
	return stmt, nil
}

func (p *Parser) parseWhileStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwWhile)
	if err != nil {
		return nil, err
	}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("после \"пока\" ожидалось условие: %w", err)
	}
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.WhileStmt{Cond: cond, Body: body, Line: kw.Line}, nil
}

// parseForStmt разбирает одну из трёх форм "для" - различаются по тому, что
// следует за именем переменной цикла: "в" (parseForEachStmt), "от"
// (диапазон, ниже) или что угодно ещё (общий C-подобный цикл,
// parseCStyleForStmt) - определяется заглядыванием на 2 токена вперёд
// (IDENT + "в"/"от"), НЕ потребляя имя заранее, иначе для C-подобной формы
// пришлось бы возвращать уже считанный токен имени назад.
func (p *Parser) parseForStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwFor)
	if err != nil {
		return nil, err
	}
	if p.cur().Type == lexer.IDENT && (p.peekAt(1).Type == lexer.KwIn || p.peekAt(1).Type == lexer.KwFrom) {
		name := p.advance()
		if p.cur().Type == lexer.KwIn {
			return p.parseForEachStmt(kw, name)
		}
		p.advance() // "от"
		from, err := p.parseExpr()
		if err != nil {
			return nil, fmt.Errorf("после \"от\" ожидалось выражение: %w", err)
		}
		if _, err := p.expect(lexer.KwTo); err != nil {
			return nil, fmt.Errorf("после начала диапазона ожидалось \"до\": %w", err)
		}
		to, err := p.parseExpr()
		if err != nil {
			return nil, fmt.Errorf("после \"до\" ожидалось выражение: %w", err)
		}
		p.registerNoun(name.Value)
		p.skipNewlines()
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		return &ast.ForStmt{Var: name.Value, From: from, To: to, Body: body, Line: kw.Line}, nil
	}
	return p.parseCStyleForStmt(kw)
}

// parseCStyleForStmt разбирает общий цикл (ни диапазон "от...до", ни
// переход "в" по коллекции - см. parseForStmt):
//
//	для <init>; <условие>; <шаг> { <тело> }
//
// Без скобок вокруг трёх секций (в Rech нигде больше нет "(...)" вокруг
// условия - "если x {", "пока x {") - ";" здесь ровно тот же разделитель
// инструкций, что уже существует в языке (docs/SPEC.md), не новая
// пунктуация. Все три секции необязательны, независимо друг от друга, тем
// же принципом, что и в C - "для ;; { ... }" (все три пустые) бесконечен
// (см. package doc ast.CStyleForStmt). Позиция парсера на входе - сразу
// после "для" (kw уже разобран).
func (p *Parser) parseCStyleForStmt(kw lexer.Token) (ast.Stmt, error) {
	var init *ast.VarDecl
	if p.cur().Type != lexer.SEMICOLON {
		var err error
		init, err = p.parseForInitStmt()
		if err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.SEMICOLON); err != nil {
		return nil, fmt.Errorf("после инициализации цикла \"для\" ожидалось \";\": %w", err)
	}
	var cond ast.Expr
	if p.cur().Type != lexer.SEMICOLON {
		var err error
		cond, err = p.parseExpr()
		if err != nil {
			return nil, fmt.Errorf("после \";\" ожидалось условие цикла \"для\": %w", err)
		}
	}
	if _, err := p.expect(lexer.SEMICOLON); err != nil {
		return nil, fmt.Errorf("после условия цикла \"для\" ожидалось \";\": %w", err)
	}
	var post ast.Stmt
	if p.cur().Type != lexer.LBRACE {
		var err error
		post, err = p.parseStatement()
		if err != nil {
			return nil, fmt.Errorf("после \";\" ожидалась инструкция шага цикла \"для\": %w", err)
		}
	}
	if init != nil {
		p.registerNoun(init.Name)
	}
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.CStyleForStmt{Init: init, Cond: cond, Post: post, Body: body, Line: kw.Line}, nil
}

// parseForInitStmt разбирает секцию инициализации общего "для" (см.
// parseCStyleForStmt) - "переменная <имя> = <выражение>" через уже
// существующий parseVarDecl, либо голое "<имя> = <выражение>", построенное
// вручную как *ast.VarDecl (не *ast.Assign) - переменная цикла ВСЕГДА
// объявляется заново в области видимости цикла, тем же принципом, что и
// ForStmt.Var/ForEachStmt.Var, которым тоже не нужно слово "переменная".
func (p *Parser) parseForInitStmt() (*ast.VarDecl, error) {
	if p.cur().Type == lexer.KwVariable {
		decl, err := p.parseVarDecl()
		if err != nil {
			return nil, err
		}
		vd, ok := decl.(*ast.VarDecl)
		if !ok {
			return nil, fmt.Errorf("строка %d: инициализация цикла \"для\" должна содержать значение (\"=\")", p.cur().Line)
		}
		return vd, nil
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"для\" ожидалась инициализация переменной цикла: %w", err)
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, fmt.Errorf("после имени переменной цикла ожидалось \"=\": %w", err)
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("после \"=\" ожидалось начальное значение переменной цикла: %w", err)
	}
	p.registerNoun(name.Value)
	return &ast.VarDecl{Name: name.Value, Value: value, Line: name.Line}, nil
}

// parseForEachStmt разбирает цикл по коллекции (шаг 4.2.b):
//
//	для <имя> в <коллекция:P> { <тело> }
//
// Позиция парсера на входе - на "в" (kw и name уже разобраны parseForStmt).
// Слово после "в" - существительное в предложном падеже (согласование с
// предлогом, живее, чем голая словарная форма "в список"), резолвится в
// переменную через resolvePrepositionalNoun - тот же механизм падежного
// сопоставления, что и у аргументов естественного вызова (шаг 3.4), а не
// совпадение текста.
func (p *Parser) parseForEachStmt(kw lexer.Token, name lexer.Token) (ast.Stmt, error) {
	p.advance() // "в"
	collTok, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"в\" в цикле по коллекции ожидалось название коллекции (в предложном падеже): %w", err)
	}
	lemma, err := p.resolvePrepositionalNoun(collTok)
	if err != nil {
		return nil, err
	}
	p.registerNoun(name.Value)
	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.ForEachStmt{
		Var:        name.Value,
		Collection: &ast.Identifier{Name: lemma, Line: collTok.Line},
		Body:       body,
		Line:       kw.Line,
	}, nil
}

// resolvePrepositionalNoun резолвит словоформу tok (существительное в
// предложном падеже после предлога "в") в лемму уже объявленной переменной
// через p.dict - тем же принципом, что и падежное сопоставление аргументов
// естественного вызова (parseNaturalCall): словоформа, а не буквальное имя
// переменной, поэтому "список" в "для x в списке" склоняется по-русски.
//
// Словарь (gomorphy/OpenCorpora плюс курируемые исключения, package doc
// internal/morph) не знает АБСОЛЮТНО каждое существительное - заимствованные
// слова вроде "сервис" не распознаются в этой позиции вовсе (найдено на
// практике -). Без словоформы вообще ни одна из его
// проверок сработать не может, значит писать коллекцию в предложном падеже
// здесь принципиально нельзя - именно поэтому p.dict.Analyze(tok.Value),
// вернувший ПУСТО (слово не опознано морфологией целиком, ни в каком
// падеже), не считается ошибкой: tok.Value принимается КАК ЕСТЬ
// (именительный падеж, буквальное имя переменной - "для x в сервисы"),
// вместо того чтобы требовать от программиста склонения, которое движок не
// может проверить. Слово, которое РАСПОЗНАНО (хоть в каком-то падеже), но
// НЕ в предложном - это, наоборот, весомый признак настоящей опечатки
// (не "словарь не знает", а "неверная форма") - остаётся ошибкой ниже, как
// и раньше.
func (p *Parser) resolvePrepositionalNoun(tok lexer.Token) (string, error) {
	analyses := p.dict.Analyze(tok.Value)
	if len(analyses) == 0 {
		return tok.Value, nil
	}
	lemma := ""
	for _, a := range analyses {
		if a.POS != morph.Noun || a.Tag.Case != morph.Prepositional {
			continue
		}
		if lemma != "" && lemma != a.Lemma {
			return "", fmt.Errorf("строка %d, столбец %d: словоформа %q неоднозначна как коллекция цикла - подходит нескольким переменным",
				tok.Line, tok.Col, tok.Value)
		}
		lemma = a.Lemma
	}
	if lemma == "" {
		return "", fmt.Errorf("строка %d, столбец %d: %q не распознано как существительное в предложном падеже (ожидалась переменная-коллекция после \"в\")",
			tok.Line, tok.Col, tok.Value)
	}
	return lemma, nil
}

func (p *Parser) parseReturnStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwReturn)
	if err != nil {
		return nil, err
	}
	switch p.cur().Type {
	case lexer.NEWLINE, lexer.SEMICOLON, lexer.RBRACE, lexer.EOF:
		return &ast.ReturnStmt{Line: kw.Line}, nil
	default:
		value, err := p.parseExpr()
		if err != nil {
			return nil, fmt.Errorf("после \"вернуть\" ожидалось выражение: %w", err)
		}
		return &ast.ReturnStmt{Value: value, Line: kw.Line}, nil
	}
}

// parseCallExpr разбирает "(" [ expr { "," expr } ] ")" вслед за именем
// функции. Позиция парсера на входе - на IDENT (имени функции).
