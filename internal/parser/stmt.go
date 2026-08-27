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

	var stmts []ast.Stmt
	for p.cur().Type != lexer.RBRACE {
		if p.cur().Type == lexer.EOF {
			return nil, fmt.Errorf("строка %d: не найдена закрывающая }", p.cur().Line)
		}
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
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
	case lexer.KwVariable:
		return p.parseVarDecl()
	case lexer.KwConst:
		return p.parseConstDecl()
	case lexer.KwIf:
		return p.parseIfStmt()
	case lexer.KwWhile:
		return p.parseWhileStmt()
	case lexer.KwFor:
		return p.parseForStmt()
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
	case lexer.IDENT:
		if p.peekAt(1).Type == lexer.ASSIGN {
			return p.parseAssign()
		}
		if p.peekAt(1).Type == lexer.DOT {
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
		if p.peekAt(1).Type == lexer.LPAREN {
			call, err := p.parseCallExpr()
			if err != nil {
				return nil, err
			}
			return &ast.ExprStmt{Call: call, Line: call.Line}, nil
		}
		if params, ok := p.funcParams[p.cur().Value]; ok {
			call, err := p.parseNaturalCall(params)
			if err != nil {
				return nil, err
			}
			return &ast.ExprStmt{Call: call, Line: call.Line}, nil
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
		return nil, fmt.Errorf("строка %d, столбец %d: \"этот\" в начале инструкции допустимо только с полем, методом или переходом состояния (\"этот.поле = ...\"/\"этот.метод(...)\"/\"этот становится ...\")",
			p.cur().Line, p.cur().Col)
	default:
		return nil, fmt.Errorf("строка %d, столбец %d: неожиданный токен %q в теле функции",
			p.cur().Line, p.cur().Col, p.cur().Value)
	}
}

// parseDotStatement разбирает инструкцию, начинающуюся с получателя (IDENT
// или "этот"), за которым следует "." - либо присваивание полю
// (receiver.поле = выражение), либо вызов метода как самостоятельная
// инструкция (receiver.метод(...)). Позиция парсера на входе - на
// получателе.
func (p *Parser) parseDotStatement() (ast.Stmt, error) {
	if p.peekAt(3).Type == lexer.ASSIGN {
		return p.parseFieldAssign()
	}
	start := p.cur()
	expr, err := p.parsePostfix()
	if err != nil {
		return nil, err
	}
	if call, ok := expr.(*ast.MethodCallExpr); ok {
		return &ast.ExprStmt{MethodCall: call, Line: call.Line}, nil
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
	return &ast.AssertStmt{Cond: cond, Line: kw.Line}, nil
}

func (p *Parser) parseVarDecl() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwVariable)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"переменная\" ожидалось имя переменной: %w", err)
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, fmt.Errorf("объявление переменной %q должно содержать инициализацию (\"=\"): %w", name.Value, err)
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в инициализации переменной %q: %w", name.Value, err)
	}
	p.registerNoun(name.Value)
	return &ast.VarDecl{Name: name.Value, Value: value, Line: kw.Line}, nil
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

func (p *Parser) parseAssign() (ast.Stmt, error) {
	name := p.advance()
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в присваивании %q: %w", name.Value, err)
	}
	return &ast.Assign{Name: name.Value, Value: value, Line: name.Line}, nil
}

// parseFieldAssign разбирает "receiver.поле = выражение". Позиция парсера
// на входе - на получателе (IDENT переменной или "этот").
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
	p.advance() // "."
	field, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \".\" ожидалось имя поля: %w", err)
	}
	if _, err := p.expect(lexer.ASSIGN); err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, fmt.Errorf("в присваивании полю %q: %w", field.Value, err)
	}
	return &ast.FieldAssign{Receiver: receiver, Field: field.Value, Value: value, Line: receiverTok.Line}, nil
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

func (p *Parser) parseForStmt() (ast.Stmt, error) {
	kw, err := p.expect(lexer.KwFor)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"для\" ожидалось имя переменной цикла: %w", err)
	}
	if p.cur().Type == lexer.KwIn {
		return p.parseForEachStmt(kw, name)
	}
	if _, err := p.expect(lexer.KwFrom); err != nil {
		return nil, fmt.Errorf("после имени переменной цикла ожидалось \"от\": %w", err)
	}
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
func (p *Parser) resolvePrepositionalNoun(tok lexer.Token) (string, error) {
	lemma := ""
	for _, a := range p.dict.Analyze(tok.Value) {
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
