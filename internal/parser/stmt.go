package parser

import (
	"slices"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
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

	var stmts []ast.Stmt
	for p.cur().Type != lexer.RBRACE {
		if p.cur().Type == lexer.EOF {
			return nil, errAtLine(p.cur().Line, CodeUnclosedBracket, "не найдена закрывающая }")
		}
		var stmt ast.Stmt
		var err error
		start := p.pos
		if p.cur().Type == lexer.DOC_COMMENT {
			stmt, err = p.parseDocumentedStatement()
		} else {
			stmt, err = p.parseStatement()
		}
		if err != nil {
			if !p.lenient {
				return nil, err
			}
			// LSP: сломанная инструкция пропускается, остальные инструкции
			// блока разбираются как обычно (см. recoverToNextStatement).
			p.errors = append(p.errors, err)
			p.recoverToNextStatement(start)
			p.skipSeparators()
			continue
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
			if !p.lenient {
				return nil, err
			}
			p.errors = append(p.errors, err)
			p.recoverToNextStatement(p.pos)
			p.skipSeparators()
		}
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
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
		return nil, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "неожиданный токен %q в теле функции", p.cur().Value)
	case lexer.IDENT:
		if decl, ok, err := p.tryParseTypedVarDecl(); err != nil {
			return nil, err
		} else if ok {
			return decl, nil
		}
		if next := p.peekPastNewlinesFrom(1); next.Type == lexer.ASSIGN || isCompoundAssignTok(next.Type) {
			// Перенос строки перед "=" ("X\n= значение") допустим - см.
			// комментарий в parseAssign. Неоднозначности с обычным
			// использованием X как значения на следующей строке нет: "X"
			// сам по себе (без "=" сразу за ним, хоть бы и через перенос)
			// не разбирается вообще ни как одна из инструкций в этом
			// switch, ни как отдельная - если следующий значимый токен НЕ
			// "=", этот if просто не совпадёт, и разбор продолжится ниже
			// как раньше.
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
		if (p.genericFuncs[p.cur().Value] && p.peekAt(1).Type == lexer.LT) || p.looksLikeGenericCall() {
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
		return nil, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "неожиданный токен %q в теле функции", p.cur().Value)
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
		return nil, errAt(p.cur().Line, p.cur().Col, CodeContextRestrict, "\"этот\" в начале инструкции допустимо только с полем, методом или переходом состояния (\"этот.поле = ...\"/\"этот.метод(...)\"/\"этот становится ...\")")
	default:
		return nil, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "неожиданный токен %q в теле функции", p.cur().Value)
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
		return 0, errAt(tok.Line, tok.Col, CodeInvalidLiteral, "некорректный целочисленный литерал %q", tok.Value)
	}
	if v < 1 {
		return 0, errAtLine(tok.Line, CodeStmtRestrict, "глубина вложенности цикла должна быть не меньше 1, получено %d", v)
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
	callsResult := p.cur().Type == lexer.LPAREN && !p.cur().SpaceBefore
	if p.cur().Type != lexer.DOT && p.cur().Type != lexer.DBLCOLON && !callsResult {
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
	case *ast.CallValueExpr:
		return &ast.ExprStmt{CallValue: e, Line: e.Line}, nil
	case *ast.FieldAccess:
		opTok := p.cur()
		if opTok.Type == lexer.ASSIGN || isCompoundAssignTok(opTok.Type) {
			return p.finishFieldAssign(e.Receiver, e.Field, e.Static, e.Line)
		}
		if isIncDecTok(opTok.Type) {
			return p.finishFieldIncDec(e.Receiver, e.Field, e.Static, e.Line)
		}
	}
	return nil, errAt(startLine, startCol, CodeStmtRestrict, "доступ к полю сам по себе не является инструкцией - нужен вызов метода или присваивание")
}

// isIncDecTok - true у "++"/"--" (см. finishFieldIncDec).
func isIncDecTok(t lexer.TokenType) bool {
	return t == lexer.INCR || t == lexer.DECR
}

func (p *Parser) parseDotStatement() (ast.Stmt, error) {
	if p.peekAt(3).Type == lexer.ASSIGN || isCompoundAssignTok(p.peekAt(3).Type) {
		return p.parseFieldAssign()
	}
	if isIncDecTok(p.peekAt(3).Type) {
		return p.parseFieldIncDec()
	}
	start := p.cur()
	expr, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	switch e := expr.(type) {
	case *ast.MethodCallExpr:
		return &ast.ExprStmt{MethodCall: e, Line: e.Line}, nil
	case *ast.CallValueExpr:
		return &ast.ExprStmt{CallValue: e, Line: e.Line}, nil
	case *ast.FieldAccess:
		opTok := p.cur()
		if opTok.Type == lexer.ASSIGN || isCompoundAssignTok(opTok.Type) {
			return p.finishFieldAssign(e.Receiver, e.Field, e.Static, e.Line)
		}
		if isIncDecTok(opTok.Type) {
			return p.finishFieldIncDec(e.Receiver, e.Field, e.Static, e.Line)
		}
	}
	return nil, errAt(start.Line, start.Col, CodeStmtRestrict, "доступ к полю сам по себе не является инструкцией - нужен вызов метода или присваивание")
}

func (p *Parser) parsePrintStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwPrint)
	if err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"вывести\" ожидалось выражение")
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
		return nil, diagnostics.Wrap(err, "после \"показать\" ожидалось выражение")
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
		return nil, diagnostics.Wrap(err, "после \"утверждать\" ожидалось выражение")
	}
	// "утверждать <условие>, <сообщение>" - необязательное второе
	// выражение (внятные сообщения о несовпадении),
	// разбирается ровно как второй аргумент вызова - через ту же запятую.
	// Перенос строки вокруг "," допустим - тем же приёмом (skipNewlines),
	// что и у любого другого списка через запятую в парсере (parseArgList,
	// parseUseDecl и т. д.) - НАЙДЕНО пользователем вживую: длинное условие
	// с длинным сообщением об ошибке было вынуждено лежать одной строкой,
	// хотя весь остальной парсер такие переносы уже разрешает. peekPastNewlines,
	// не skipNewlines, ДО решения "запятая точно будет" - если её не
	// окажется (сообщения нет), эти же переносы строк - разделитель ЭТОЙ
	// инструкции от следующей (requireSeparator, package doc), их нельзя
	// съесть заранее, не зная, найдётся ли запятая.
	var message ast.Expr
	if p.peekPastNewlines().Type == lexer.COMMA {
		p.skipNewlines()
		p.advance() // ","
		p.skipNewlines()
		message, err = p.parseExpr()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \"утверждать <условие>,\" ожидалось выражение сообщения")
		}
	}
	return &ast.AssertStmt{Cond: cond, Message: message, Line: kw.Line}, nil
}

// parseVarDecl разбирает "[статическая] переменная <имя> = <выражение>" -
// тип выводится из значения (если вывести нельзя - ошибка семантики с
// просьбой указать тип). Явный тип - другая форма объявления: "<Тип> <имя>
// [= <выражение>]" (tryParseTypedVarDecl), поэтому ни "переменная <имя>:
// <Тип>", ни "переменная <имя>" без значения не бывает.
func (p *Parser) parseVarDecl() (ast.Stmt, error) {
	isStatic := false
	if p.cur().Type == lexer.KwStatic {
		tok := p.advance()
		// "переменная" - существительное женского рода, модификатор
		// согласуется с НИМ - единственная правильная форма "статическая".
		if tok.Value != "статическая" {
			return nil, errAt(tok.Line, tok.Col, CodeStmtRestrict, "перед \"переменная\" нужна форма \"статическая\", получено %q", tok.Value)
		}
		isStatic = true
	}
	kw, err := p.expect(lexer.KwVariable)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"переменная\" ожидалось имя переменной")
	}
	if p.cur().Type == lexer.COLON {
		return nil, p.typedVariableKeywordError(name)
	}
	// Перенос строки перед "=" ("переменная X\n= значение") допустим, только
	// если за ним ДЕЙСТВИТЕЛЬНО следует "=".
	if p.cur().Type == lexer.NEWLINE && p.peekPastNewlines().Type == lexer.ASSIGN {
		p.skipNewlines()
	}
	if p.cur().Type != lexer.ASSIGN {
		if isStatic {
			return nil, errAt(p.cur().Line, p.cur().Col, CodeStmtRestrict, "\"статическая переменная\" должна содержать инициализацию (\"=\") - без неё непонятно, каким значением она заполнится ровно один раз")
		}
		return nil, errAt(name.Line, name.Col, CodeStmtRestrict, "у \"переменная %s\" без значения не вычислить тип - укажите тип явно", name.Value).
			WithHint("<Тип> %s, например: Целое %s", name.Value, name.Value)
	}
	p.advance()      // "="
	p.skipNewlines() // "=\nзначение" - перенос сразу после "=" тоже незначим.
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в инициализации переменной %q", name.Value)
	}
	p.registerNoun(name.Value)
	return &ast.VarDecl{Name: name.Value, Value: value, IsStatic: isStatic, Line: kw.Line}, nil
}

// typedVariableKeywordError - "переменная <имя>: <Тип>": "переменная"
// выводит тип сам, явный тип пишется первым. Позиция - на ":".
func (p *Parser) typedVariableKeywordError(name lexer.Token) error {
	colon := p.cur()
	save, saveGT := p.pos, p.pendingGT
	defer func() { p.pos, p.pendingGT = save, saveGT }()
	p.advance()
	typeName, err := p.parseTypeName()
	if err != nil {
		typeName = "<Тип>"
	}
	value := ""
	if p.cur().Type == lexer.ASSIGN {
		value = " = ..."
	}
	return errAt(colon.Line, colon.Col, CodeStmtRestrict, "\"переменная\" выводит тип из значения - объявление с явным типом пишется \"Тип имя\"").
		WithHint("%s %s%s", typeName, name.Value, value)
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
		return nil, diagnostics.Wrap(err, "после \"константа\" ожидалось имя константы")
	}
	if p.cur().Type == lexer.NEWLINE && p.peekPastNewlines().Type == lexer.ASSIGN {
		p.skipNewlines()
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, diagnostics.Wrap(err, "объявление константы %q должно содержать инициализацию (\"=\")", name.Value)
	}
	p.skipNewlines()
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в инициализации константы %q", name.Value)
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
		return nil, diagnostics.Wrap(err, "после \"*\" ожидался указатель")
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, diagnostics.Wrap(err, "после \"*<указатель>\" ожидалось \"=\"")
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в присваивании через указатель")
	}
	return &ast.DerefAssignStmt{Pointer: pointer, Value: value, Line: star.Line}, nil
}

func (p *Parser) parseAssign() (ast.Stmt, error) {
	name := p.advance()
	// Перенос строки перед "="/составным оператором ("X\n= значение") -
	// дошли сюда только потому, что вызывающая сторона (диспетчер
	// parseStatement) уже подтвердила через peekPastNewlinesFrom, что за
	// переносом действительно следует "=" - здесь его остаётся только
	// пропустить.
	p.skipNewlines()
	opTok := p.cur()
	if opTok.Type != lexer.ASSIGN && !isCompoundAssignTok(opTok.Type) {
		return nil, errAt(opTok.Line, opTok.Col, CodeUnexpectedToken, "ожидался \"=\" в присваивании %q, получено %q", name.Value, opTok.Value)
	}
	p.advance()
	p.skipNewlines() // "=\nзначение" - перенос сразу после "=" тоже незначим.
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в присваивании %q", name.Value)
	}
	if baseOp, ok := compoundAssignOps[opTok.Type]; ok {
		value = &ast.BinaryExpr{Op: baseOp, Left: &ast.Identifier{Name: name.Value, Line: name.Line}, Right: value, Line: opTok.Line}
	}
	return &ast.Assign{Name: name.Value, Value: value, Line: name.Line}, nil
}

// parseNullCoalesceAssign разбирает "а ?= б" (null-coalescing присваивание,
// только простое имя переменной - как у parseAssign, не поле сущности).
func (p *Parser) parseNullCoalesceAssign() (ast.Stmt, error) {
	name := p.advance()
	p.advance() // "?="
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в присваивании %q", name.Value)
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
		return nil, diagnostics.Wrap(err, "в индексе \"[...]\"")
	}
	if _, err := p.expect(lexer.RBRACKET); err != nil {
		return nil, diagnostics.Wrap(err, "после индекса ожидалось \"]\"")
	}
	if p.cur().Type == lexer.DOT && p.peekAt(1).Type == lexer.IDENT {
		// "сп[i].поле = значение" - запись поля элемента (semantic,
		// element_field_assign.go).
		p.advance() // "."
		field := p.advance()
		elem := &ast.IndexExpr{Receiver: receiver, Index: index, Line: receiverTok.Line}
		if isIncDecTok(p.cur().Type) {
			return p.finishFieldIncDec(elem, field.Value, false, receiverTok.Line)
		}
		return p.finishFieldAssign(elem, field.Value, false, receiverTok.Line)
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, diagnostics.Wrap(err, "после %q[...] ожидалось \"=\" (индексация сама по себе не инструкция)", receiverTok.Value)
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в присваивании по индексу")
	}
	call := &ast.MethodCallExpr{Receiver: receiver, Method: "установить", Args: []ast.Expr{index, value}, Line: receiverTok.Line}
	return &ast.ExprStmt{MethodCall: call, Line: receiverTok.Line}, nil
}

// parseSimpleFieldReceiver разбирает "receiver.поле"/"receiver::поле" -
// receiver РОВНО один уровень (IDENT переменной или "этот"), без
// вложенности - общая голова parseFieldAssign/parseFieldIncDec (различаются
// только хвостом - что идёт ПОСЛЕ имени поля). Позиция парсера на входе -
// на получателе.
func (p *Parser) parseSimpleFieldReceiver() (receiver ast.Expr, field string, static bool, line int, err error) {
	receiverTok := p.advance() // IDENT получателя или "этот"
	if receiverTok.Type == lexer.KwThis {
		if !p.inMethodBody {
			return nil, "", false, 0, errAt(receiverTok.Line, receiverTok.Col, CodeContextRestrict, "\"этот\" валиден только внутри тела метода")
		}
		receiver = &ast.ThisExpr{Line: receiverTok.Line}
	} else {
		receiver = &ast.Identifier{Name: receiverTok.Value, Line: receiverTok.Line}
	}
	static = p.cur().Type == lexer.DBLCOLON
	p.advance() // "." или "::"
	fieldTok, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, "", false, 0, diagnostics.Wrap(err, "после \".\" ожидалось имя поля")
	}
	return receiver, fieldTok.Value, static, receiverTok.Line, nil
}

// parseFieldAssign разбирает "receiver.поле = выражение" - см. package doc
// parseSimpleFieldReceiver. Присваивание в поле ВЛОЖЕННОЙ сущности
// (`x.a.b = значение`) идёт другим путём - см. parseDotStatement,
// finishFieldAssign.
func (p *Parser) parseFieldAssign() (ast.Stmt, error) {
	receiver, field, static, line, err := p.parseSimpleFieldReceiver()
	if err != nil {
		return nil, err
	}
	return p.finishFieldAssign(receiver, field, static, line)
}

// parseFieldIncDec разбирает "receiver.поле++"/"receiver.поле--" - см.
// package doc parseSimpleFieldReceiver/finishFieldIncDec.
func (p *Parser) parseFieldIncDec() (ast.Stmt, error) {
	receiver, field, static, line, err := p.parseSimpleFieldReceiver()
	if err != nil {
		return nil, err
	}
	return p.finishFieldIncDec(receiver, field, static, line)
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
		return nil, errAt(opTok.Line, opTok.Col, CodeUnexpectedToken, "ожидался \"=\" в присваивании полю %q, получено %q", field, opTok.Value)
	}
	p.advance()
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в присваивании полю %q", field)
	}
	if baseOp, ok := compoundAssignOps[opTok.Type]; ok {
		value = &ast.BinaryExpr{Op: baseOp, Left: &ast.FieldAccess{Receiver: receiver, Field: field, Static: static, Line: line}, Right: value, Line: opTok.Line}
	}
	return &ast.FieldAssign{Receiver: receiver, Field: field, Static: static, Value: value, Line: line}, nil
}

// finishFieldIncDec разбирает хвост "receiver.поле++"/"receiver.поле--" -
// после того как получатель и имя поля уже разобраны (позиция парсера на
// входе - на "++"/"--"). Сахар над FieldAssign, тем же приёмом, что и
// составные операторы у finishFieldAssign ("поле += выражение" -
// "поле = поле + выражение") - "поле++" разворачивается в
// "поле = поле + 1": НАЙДЕНО пользователем вживую - "++"/"--" у поля
// сущности (в отличие от "+="/"-=", которые уже поддержаны finishFieldAssign
// через ту же compoundAssignOps) не были разобраны никак. Отдельного узла
// AST/IR не заводится (как и у "+=") - вся семантика (числовой тип
// операнда "+"/константность и т. п.) уже проверяется обычным присваиванием
// полю. В отличие от ast.IncDecExpr (простое имя переменной) - только
// инструкция целиком ("поле++" на своей строке), не под-выражение
// ("x = поле++" не поддержано - не запрошено).
func (p *Parser) finishFieldIncDec(receiver ast.Expr, field string, static bool, line int) (ast.Stmt, error) {
	op := p.advance() // "++" или "--"
	baseOp := "+"
	if op.Type == lexer.DECR {
		baseOp = "-"
	}
	value := &ast.BinaryExpr{
		Op:    baseOp,
		Left:  &ast.FieldAccess{Receiver: receiver, Field: field, Static: static, Line: line},
		Right: &ast.IntLiteral{Value: 1, Line: op.Line},
		Line:  op.Line,
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
		return nil, diagnostics.Wrap(err, "после \"если\" ожидалось условие")
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
		if p.cur().Type == lexer.KwIf {
			// "иначе если" - чистый синтаксический сахар: разворачивается
			// в тот же вложенный "если" внутри "иначе" ("Else" - список из
			// ОДНОЙ инструкции, самого вложенного IfStmt), без отдельного
			// узла AST и без изменений semantic/backend - обе формы дают
			// идентичное дерево.
			nested, err := p.parseIfStmt()
			if err != nil {
				return nil, err
			}
			stmt.Else = []ast.Stmt{nested}
			return stmt, nil
		}
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
		return nil, diagnostics.Wrap(err, "после \"пока\" ожидалось условие")
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
// (IDENT + "в"/"от"; IDENT "," IDENT "в" - цикл по коллекции с двумя
// переменными), НЕ потребляя имя заранее, иначе для C-подобной формы
// пришлось бы возвращать уже считанный токен имени назад.
func (p *Parser) parseForStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwFor)
	if err != nil {
		return nil, err
	}
	if p.cur().Type == lexer.IDENT && p.peekAt(1).Type == lexer.COMMA && p.peekAt(2).Type == lexer.IDENT && p.peekAt(3).Type == lexer.KwIn {
		key := p.advance()
		p.advance() // ","
		name := p.advance()
		p.registerNoun(key.Value)
		return p.parseForEachStmt(kw, key.Value, name)
	}
	if p.cur().Type == lexer.IDENT && (p.peekAt(1).Type == lexer.KwIn || p.peekAt(1).Type == lexer.KwFrom) {
		name := p.advance()
		if p.cur().Type == lexer.KwIn {
			return p.parseForEachStmt(kw, "", name)
		}
		p.advance() // "от"
		from, err := p.parseExpr()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \"от\" ожидалось выражение")
		}
		if _, err := p.expect(lexer.KwTo); err != nil {
			return nil, diagnostics.Wrap(err, "после начала диапазона ожидалось \"до\"")
		}
		to, err := p.parseExpr()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \"до\" ожидалось выражение")
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
		return nil, diagnostics.Wrap(err, "после инициализации цикла \"для\" ожидалось \";\"")
	}
	var cond ast.Expr
	if p.cur().Type != lexer.SEMICOLON {
		var err error
		cond, err = p.parseExpr()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \";\" ожидалось условие цикла \"для\"")
		}
	}
	if _, err := p.expect(lexer.SEMICOLON); err != nil {
		return nil, diagnostics.Wrap(err, "после условия цикла \"для\" ожидалось \";\"")
	}
	var post ast.Stmt
	if p.cur().Type != lexer.LBRACE {
		var err error
		post, err = p.parseStatement()
		if err != nil {
			return nil, diagnostics.Wrap(err, "после \";\" ожидалась инструкция шага цикла \"для\"")
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
			return nil, errAtLine(p.cur().Line, CodeStmtRestrict, "инициализация цикла \"для\" должна содержать значение (\"=\")")
		}
		return vd, nil
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"для\" ожидалась инициализация переменной цикла")
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, diagnostics.Wrap(err, "после имени переменной цикла ожидалось \"=\"")
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"=\" ожидалось начальное значение переменной цикла")
	}
	p.registerNoun(name.Value)
	return &ast.VarDecl{Name: name.Value, Value: value, Line: name.Line}, nil
}

// parseForEachStmt разбирает цикл по коллекции (шаг 4.2.b):
//
//	для <имя> в <коллекция:P> { <тело> }
//	для <ключ>, <имя> в <коллекция:P> { <тело> }
//
// Позиция парсера на входе - на "в" (kw, key и name уже разобраны
// parseForStmt; key пуст у формы с одной переменной).
// Слово после "в" - существительное в предложном падеже (согласование с
// предлогом, живее, чем голая словарная форма "в список"), резолвится в
// переменную через resolvePrepositionalNoun - тот же механизм падежного
// сопоставления, что и у аргументов естественного вызова (шаг 3.4), а не
// совпадение текста.
func (p *Parser) parseForEachStmt(kw lexer.Token, key string, name lexer.Token) (ast.Stmt, error) {
	p.advance() // "в"
	collTok, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"в\" в цикле по коллекции ожидалось название коллекции (в предложном падеже)")
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
		KeyVar:     key,
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
	var lemmas []string
	for _, a := range analyses {
		if a.POS != morph.Noun || a.Tag.Case != morph.Prepositional || slices.Contains(lemmas, a.Lemma) {
			continue
		}
		lemmas = append(lemmas, a.Lemma)
	}
	if len(lemmas) > 1 {
		// Словарь общий на файл - оставляем имена ТЕКУЩЕЙ функции/метода
		// (см. Parser.declGen): "числах" в функции с параметром "числа" -
		// не переменная "число" из соседней функции.
		var own []string
		for _, l := range lemmas {
			if gen, ok := p.nounDecl[l]; ok && gen == p.declGen {
				own = append(own, l)
			}
		}
		if len(own) != 1 {
			return "", errAt(tok.Line, tok.Col, CodeNaturalCall, "словоформа %q неоднозначна как коллекция цикла - подходит нескольким переменным", tok.Value)
		}
		lemmas = own
	}
	lemma := ""
	if len(lemmas) == 1 {
		lemma = lemmas[0]
	}
	if lemma == "" {
		// Словоформа известна, но не как предложный падеж ("задачи" -
		// род. ед./им. мн. от "задача") - сопоставление с переменной
		// буквально, тем же путём, что и у неизвестного слова выше.
		return tok.Value, nil
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
			return nil, diagnostics.Wrap(err, "после \"вернуть\" ожидалось выражение")
		}
		return &ast.ReturnStmt{Value: value, Line: kw.Line}, nil
	}
}

// parseCallExpr разбирает "(" [ expr { "," expr } ] ")" вслед за именем
// функции. Позиция парсера на входе - на IDENT (имени функции).
