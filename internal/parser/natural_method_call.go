package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// collectEntitySignature - как KwFunction в collectOneSignature, но для
// "[абстрактная] сущность|класс|структура ИМЯ ... { ... }": собирает только
// параметры каждого МЕТОДА (entityMethods[имя_сущности][имя_метода]), не
// разбирая тела методов и не трогая поля/константы/состояния - нужен только
// список параметров для естественного вызова метода (natural_method_call.go),
// так же, как funcParams нужен для естественного вызова свободной функции
// (parser.go, collectFunctionSignatures). Позиция парсера на входе - на
// "абстрактная"/"сущность"/"класс"/"структура".
//
// Тело сущности сканируется ОДНИМ проходом без вызова parseExpr где бы то ни
// было (в отличие от полноценного parseEntity) - ровно та же причина, что и у
// skipFunctionBody: этот проход выполняется ДО того, как funcParams
// заполнен, поэтому естественный вызов внутри значения поля/константы по
// умолчанию, ссылающийся на функцию, объявленную позже в файле, не должен
// падать здесь с ошибкой - тело метода и любое выражение внутри сущности
// пропускаются вслепую, подсчётом "{"/"}" (см. depth ниже), а не разбором.
func (p *Parser) collectEntitySignature(entityMethods map[string]map[string][]ast.Param) error {
	if p.cur().Type == lexer.KwAbstract {
		p.advance()
	}
	p.advance() // "сущность"/"класс"/"структура"

	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return diagnostics.Wrap(err, "после \"сущность\"/\"класс\"/\"структура\" ожидалось имя")
	}

	if p.cur().Type == lexer.LT {
		if _, err := p.parseGenericParams(); err != nil {
			return diagnostics.Wrap(err, "сущность %q", name.Value)
		}
	}
	if p.cur().Type == lexer.KwInherits {
		p.advance()
		base, err := p.expect(lexer.IDENT)
		if err != nil {
			return diagnostics.Wrap(err, "сущность %q: после \"наследует\" ожидалось имя", name.Value)
		}
		if _, err := p.parseTypeArgsSuffix(base.Value); err != nil {
			return diagnostics.Wrap(err, "сущность %q: базовая сущность", name.Value)
		}
	}
	if p.cur().Type == lexer.KwImplements {
		p.advance()
		for {
			if _, err := p.expect(lexer.IDENT); err != nil {
				return diagnostics.Wrap(err, "сущность %q: после \"реализует\" ожидалось имя", name.Value)
			}
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance()
			p.skipNewlines()
		}
	}
	if p.cur().Type == lexer.TAG {
		p.advance()
	}
	p.skipNewlines()
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return diagnostics.Wrap(err, "сущность %q", name.Value)
	}

	methods := map[string][]ast.Param{}
	depth := 1
	atMemberStart := true
	for depth > 0 {
		if p.cur().Type == lexer.EOF {
			return errAt(p.cur().Line, p.cur().Col, CodeUnclosedBracket, "сущность %q: не найдена закрывающая \"}\"", name.Value)
		}
		if atMemberStart && depth == 1 {
			methodName, params, ok, err := p.tryCollectMethodHeader()
			if err != nil {
				return diagnostics.Wrap(err, "сущность %q", name.Value)
			}
			if ok {
				methods[methodName] = params
				atMemberStart = false
				continue
			}
		}
		switch p.cur().Type {
		case lexer.LBRACE:
			depth++
			atMemberStart = false
		case lexer.RBRACE:
			depth--
			atMemberStart = false
		case lexer.NEWLINE, lexer.SEMICOLON:
			if depth == 1 {
				atMemberStart = true
			}
		default:
			atMemberStart = false
		}
		p.advance()
	}
	entityMethods[name.Value] = methods
	return nil
}

// beginVarTypeScope сбрасывает varEntityTypes на новую карту, заполненную
// явными типами params (Name -> Type - тип параметра всегда явный, в отличие
// от переменной), и возвращает функцию восстановления прежней карты -
// вызывается в начале разбора тела функции/метода/конструктора
// (parseFunction/parseMethod/parseConstructor), до этого места переменные
// предыдущего тела не должны быть видны (иначе получатель естественного
// вызова метода в одной функции мог бы ложно унаследовать тип переменной с
// тем же именем из другой). Само тело (parseVarDecl/tryParseTypedVarDecl)
// добавляет в эту же карту переменные с явной аннотацией типа по ходу
// разбора.
func (p *Parser) beginVarTypeScope(params []ast.Param) func() {
	prev := p.varEntityTypes
	m := make(map[string]string, len(params))
	for _, param := range params {
		m[param.Name] = param.Type
	}
	p.varEntityTypes = m
	return func() { p.varEntityTypes = prev }
}

// registerVarEntityType запоминает явно аннотированный тип переменной -
// общая точка для parseVarDecl (stmt.go) и tryParseTypedVarDecl
// (typed_var_decl.go), которые обе строят ast.VarDecl с непустым Type.
// typeName - имя типа как есть (может оказаться не сущностью вообще, а
// примитивом/generic-инстанциацией - естественный вызов метода просто не
// найдёт для такого имени записи в entityMethods, ничего страшного).
func (p *Parser) registerVarEntityType(name, typeName string) {
	if p.varEntityTypes == nil {
		p.varEntityTypes = map[string]string{}
	}
	p.varEntityTypes[name] = typeName
}

// receiverEntityType возвращает имя сущностного типа получателя tok, если
// оно известно ПАРСЕРУ на этом месте разбора (в отличие от семантики, парсер
// не выводит типы - только то, что написано явно):
//
//   - "этот" - currentEntityName, но только внутри тела метода (inMethodBody) -
//     та же проверка, что и у обычного "этот.поле"/"этот.метод(...)".
//   - обычная переменная/параметр - varEntityTypes[tok.Value], если она была
//     объявлена с явной аннотацией типа (registerVarEntityType/
//     beginVarTypeScope) - переменная без явного типа (тип выведен из
//     значения) для естественного вызова метода недоступна в этом срезе.
func (p *Parser) receiverEntityType(tok lexer.Token) (string, bool) {
	if tok.Type == lexer.KwThis {
		if p.inMethodBody && p.currentEntityName != "" {
			return p.currentEntityName, true
		}
		return "", false
	}
	if tok.Type != lexer.IDENT {
		return "", false
	}
	t, ok := p.varEntityTypes[tok.Value]
	if !ok || t == "" {
		return "", false
	}
	return t, true
}

// startsNaturalMethodCall - true, если текущий токен - получатель известного
// сущностного типа (receiverEntityType), а СРАЗУ следующий - имя одного из
// методов этого типа (entityMethods, собранный предварительным проходом,
// см. collectEntitySignature). Не потребляет токены. params - параметры
// найденного метода, нужны parseNaturalMethodCall для падежного
// сопоставления аргументов, той же общей функцией (matchNaturalCallArgs,
// call.go), что и у естественного вызова свободной функции.
//
// "IDENT IDENT" (получатель сразу за которым - ещё один голый идентификатор,
// без "."/"("/оператора между ними) до этой фичи было ГАРАНТИРОВАННО
// синтаксической ошибкой (тем же принципом, что и у typed_var_decl.go,
// "IDENT IDENT =") - ложных срабатываний на существующем коде нет.
func (p *Parser) startsNaturalMethodCall() ([]ast.Param, bool) {
	entityType, ok := p.receiverEntityType(p.cur())
	if !ok {
		return nil, false
	}
	methods, ok := p.entityMethods[entityType]
	if !ok {
		return nil, false
	}
	next := p.peekAt(1)
	if next.Type != lexer.IDENT {
		return nil, false
	}
	params, ok := methods[next.Value]
	return params, ok
}

// parseNaturalMethodCall разбирает естественный вызов метода - "<получатель>
// <метод> [падежные аргументы...]" (docs/GRAMMAR_DRAFT.md, "Естественный вызов
// методов сущности"): получатель - "этот" или переменная известного
// сущностного типа (startsNaturalMethodCall это уже проверила), имя метода -
// ровно объявленное (без спряжения - тот же принцип, что и у естественного
// вызова свободной функции, "глагол - ровно объявленное имя функции").
// Позиция парсера на входе - на получателе.
func (p *Parser) parseNaturalMethodCall(params []ast.Param) (*ast.MethodCallExpr, error) {
	receiverTok := p.advance()
	var receiver ast.Expr
	if receiverTok.Type == lexer.KwThis {
		receiver = &ast.ThisExpr{Line: receiverTok.Line}
	} else {
		receiver = &ast.Identifier{Name: receiverTok.Value, Line: receiverTok.Line}
	}
	methodTok := p.advance() // имя метода
	args, err := p.matchNaturalCallArgs(methodTok, params)
	if err != nil {
		return nil, err
	}
	return &ast.MethodCallExpr{Receiver: receiver, Method: methodTok.Value, Args: args, Line: receiverTok.Line}, nil
}

// parseNaturalMethodCallContinuation - как parseNaturalCallContinuation
// (call.go), но для естественного вызова метода - тот же общий разбор "и
// <действие>" (parseNaturalCallContinuationCore), различается только то, в
// какое поле ast.ExprStmt попадает вызов без продолжения (MethodCall, не
// Call).
func (p *Parser) parseNaturalMethodCallContinuation(call *ast.MethodCallExpr) (ast.Stmt, error) {
	stmt, has, err := p.parseNaturalCallContinuationCore(call, call.Line)
	if err != nil {
		return nil, err
	}
	if has {
		return stmt, nil
	}
	return &ast.ExprStmt{MethodCall: call, Line: call.Line}, nil
}

// tryCollectMethodHeader - спекулятивная попытка разобрать начало члена тела
// сущности как заголовок метода ("[статический] [приватный|публичный|
// защищённый] ИМЯ "(" [параметры] ")" [":" ТипРезультата|"этот"] [тег]"),
// вызывается только при atMemberStart на depth==1 (collectEntitySignature) -
// это то, что отличает настоящее начало метода от, например, вызова функции
// внутри значения поля/константы по умолчанию ("поле: Целое = вычислить(5)"),
// который синтаксически тоже выглядит как "IDENT(", но не стоит в позиции
// начала члена. Не совпало - позиция восстанавливается (ok == false, err ==
// nil), тело метода НЕ потребляется (его пропускает общий цикл вызывающей
// стороны подсчётом скобок, тем же путём, что и любой другой токен).
func (p *Parser) tryCollectMethodHeader() (methodName string, params []ast.Param, ok bool, err error) {
	saved := p.pos
	if p.cur().Type == lexer.KwStatic {
		p.advance()
	}
	if isVisibilityKeyword(p.cur().Type) {
		p.advance()
	}
	if p.cur().Type != lexer.IDENT || p.peekAt(1).Type != lexer.LPAREN {
		p.pos = saved
		return "", nil, false, nil
	}
	name := p.advance() // имя метода
	params, err = p.parseParams()
	if err != nil {
		return "", nil, false, err
	}
	if p.cur().Type == lexer.COLON {
		p.advance()
		if p.cur().Type == lexer.KwThis {
			p.advance()
		} else if _, err := p.parseTypeName(); err != nil {
			return "", nil, false, err
		}
	}
	if p.cur().Type == lexer.TAG {
		p.advance()
	}
	return name.Value, params, true, nil
}
