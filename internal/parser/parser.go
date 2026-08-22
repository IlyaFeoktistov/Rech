// Package parser строит ast.Program из потока токенов лексера.
//
// Грамматика v0.1 (расширяется по этапам из docs/ROADMAP.md, подробный
// EBNF-набросок - docs/GRAMMAR.md):
//
//	program     := { newline } { ( function | entity ) { newline } }
//	entity      := ( "сущность" | "класс" ) IDENT newline "{" [ fields_section ] "}"
//	fields_section := "имеет" ":" { field }
//	field       := IDENT ":" IDENT
//	function    := "функция" IDENT [ "(" [ params ] ")" ] [ ":" IDENT ] newline block
//	params      := param { "," param }
//	param       := IDENT ":" IDENT
//	block       := "{" newline { statement } "}"
//	statement   := var_decl | assign | field_assign | print_stmt | if_stmt | while_stmt | for_stmt | return_stmt | call_stmt
//	var_decl    := "переменная" IDENT "=" expr
//	assign      := IDENT "=" expr
//	field_assign := IDENT "." IDENT "=" expr
//	print_stmt  := "вывести" expr
//	if_stmt     := "если" expr block [ "иначе" block ]
//	while_stmt  := "пока" expr block
//	for_stmt    := "для" IDENT "от" expr "до" expr block
//	return_stmt := "вернуть" [ expr ]
//	call_stmt   := call_expr
//	call_expr   := IDENT "(" [ expr { "," expr } ] ")"
//
//	expr        := or_expr
//	or_expr     := and_expr { "или" and_expr }
//	and_expr    := not_expr { "и" not_expr }
//	not_expr    := "не" not_expr | comparison
//	comparison  := additive [ ("==" | "!=" | "<" | ">" | "<=" | ">=") additive ]
//	additive    := multiplicative { ("+" | "-") multiplicative }
//	multiplicative := unary { ("*" | "/") unary }
//	unary       := "-" unary | primary
//	primary     := INT | FLOAT | STRING | "истина" | "ложь" | IDENT | call_expr | "(" expr ")"
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// entryPointName дублирует cbackend.entryPointName - парсеру нужно знать
// имя точки входа, чтобы запретить у неё параметры и тип результата (у
// главной фиксированная C-сигнатура). Не импортируется из cbackend, чтобы
// parser не зависел от backend'а (архитектурный инвариант, см.
// docs/ROADMAP.md).
const entryPointName = "главная"

type Parser struct {
	tokens []lexer.Token
	pos    int

	// dict накапливает переменные, объявленные по ходу разбора, как
	// существительные морфологического движка (internal/morph) - имя
	// переменной становится леммой (registerNoun). Это то, что позволяет
	// падежному сопоставлению аргументов естественного вызова опознавать
	// словоформы вроде "пользователя" как аргумент-ссылку на переменную
	// "пользователь".
	dict *morph.Dictionary

	// funcParams - параметры всех функций программы по имени, собранные
	// предварительным проходом (collectFunctionSignatures) до разбора
	// любого тела. Нужен, чтобы отличить, разбирая начало инструкции,
	// естественный вызов (IDENT - это имя объявленной функции) от ссылки
	// на переменную - иначе с этим нельзя определиться, пока не дойдёт
	// очередь до объявления самой функции (которое может оказаться и
	// раньше, и позже места вызова, как и для строгого вызова, шаг 3.2).
	funcParams map[string][]ast.Param

	// inMethodBody - true, пока парсер разбирает тело метода сущности
	// (parseMethod). "этот" валиден только пока это поле true - структурное
	// свойство (где мы сейчас в дереве разбора), не зависящее от типов,
	// поэтому проверяется здесь, а не в backend'е.
	inMethodBody bool
}

func New(tokens []lexer.Token) *Parser {
	return &Parser{tokens: tokens, dict: morph.NewDictionary()}
}

// registerNoun лучшим образом пытается зарегистрировать имя переменной как
// существительное в dict. Ошибку намеренно игнорирует - не всякий
// идентификатор пригоден как русское существительное (однобуквенные имена
// вроде "i", латиница, слова с окончанием, которое ClassifyByEnding не
// берёт и которого нет в курируемых исключениях): такие переменные просто
// не будут доступны для естественного вызова, только для строгого - это
// не ошибка компиляции.
func (p *Parser) registerNoun(name string) {
	_ = p.dict.RegisterNoun(name, morph.AnimacyOf(name))
}

func (p *Parser) cur() lexer.Token {
	return p.tokens[p.pos]
}

func (p *Parser) peekAt(offset int) lexer.Token {
	idx := p.pos + offset
	if idx >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1] // EOF
	}
	return p.tokens[idx]
}

func (p *Parser) advance() lexer.Token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) expect(tt lexer.TokenType) (lexer.Token, error) {
	if p.cur().Type != tt {
		return lexer.Token{}, fmt.Errorf("строка %d, столбец %d: ожидался %s, получен %q",
			p.cur().Line, p.cur().Col, tt, p.cur().Value)
	}
	return p.advance(), nil
}

func (p *Parser) skipNewlines() {
	for p.cur().Type == lexer.NEWLINE {
		p.advance()
	}
}

// peekPastNewlines возвращает первый токен от текущей позиции, который не
// NEWLINE, не потребляя ничего. Нужен, чтобы заглянуть вперёд через
// необязательные пустые строки (например, есть ли "иначе" после блока
// "если") без риска съесть разделитель инструкций, который окажется нужен,
// если искомая конструкция не найдётся.
func (p *Parser) peekPastNewlines() lexer.Token {
	offset := 0
	for p.peekAt(offset).Type == lexer.NEWLINE {
		offset++
	}
	return p.peekAt(offset)
}

// skipSeparators пропускает ноль или больше разделителей инструкций
// (перенос строки или ";", в любом сочетании) и возвращает, сколько было
// пропущено.
func (p *Parser) skipSeparators() int {
	n := 0
	for p.cur().Type == lexer.NEWLINE || p.cur().Type == lexer.SEMICOLON {
		p.advance()
		n++
	}
	return n
}

// requireSeparator - как skipSeparators, но ошибка, если разделителей не
// нашлось ни одного. Используется между двумя инструкциями внутри блока:
// перенос строки/`;` возле "{"/"}" остаётся необязательным (расстановка
// скобок свободная), а вот между соседними инструкциями - обязателен,
// иначе "переменная а = 5 переменная б = 3" разбиралось бы как две
// инструкции без разделителя, что становится неоднозначным, как только в
// языке появятся многословные конструкции (естественные вызовы, шаг 3).
func (p *Parser) requireSeparator() error {
	if p.skipSeparators() == 0 {
		return fmt.Errorf("строка %d, столбец %d: инструкции должны быть разделены переносом строки или \";\", получено %q",
			p.cur().Line, p.cur().Col, p.cur().Value)
	}
	return nil
}

// collectFunctionSignatures сканирует все объявления "функция ИМЯ(...)" во
// всей программе, не разбирая тела (skipFunctionBody просто пропускает их
// подсчётом фигурных скобок), и заполняет p.funcParams. Позиция парсера
// восстанавливается в исходную после сканирования - это отдельный проход
// перед настоящим разбором, а не часть его.
//
// Нужен для естественного вызова (parseNaturalCall): встретив в начале
// инструкции IDENT, парсер должен уже знать, имя ли это объявленной
// функции (тогда - естественный вызов) или ссылка на переменную, а
// объявление этой функции в файле может быть как раньше, так и позже места
// вызова (как и для строгого вызова, шаг 3.2).
func (p *Parser) collectFunctionSignatures() error {
	saved := p.pos
	defer func() { p.pos = saved }()
	p.pos = 0

	sigs := map[string][]ast.Param{}
	for p.cur().Type != lexer.EOF {
		if p.cur().Type != lexer.KwFunction {
			p.advance()
			continue
		}
		p.advance() // "функция"
		name, err := p.expect(lexer.IDENT)
		if err != nil {
			return fmt.Errorf("после \"функция\" ожидалось имя функции: %w", err)
		}
		var params []ast.Param
		if p.cur().Type == lexer.LPAREN {
			params, err = p.parseParams()
			if err != nil {
				return fmt.Errorf("в списке параметров функции %q: %w", name.Value, err)
			}
		}
		if p.cur().Type == lexer.COLON {
			p.advance()
			if _, err := p.parseTypeName(); err != nil {
				return fmt.Errorf("после \":\" у функции %q ожидалось имя типа результата: %w", name.Value, err)
			}
		}
		sigs[name.Value] = params
		if err := p.skipFunctionBody(); err != nil {
			return err
		}
	}
	p.funcParams = sigs
	return nil
}

// skipFunctionBody пропускает тело функции подсчётом фигурных скобок, не
// разбирая его - используется только в collectFunctionSignatures. Позиция
// парсера на входе - сразу после сигнатуры функции (перед "{").
func (p *Parser) skipFunctionBody() error {
	p.skipNewlines()
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return err
	}
	depth := 1
	for depth > 0 {
		switch p.cur().Type {
		case lexer.LBRACE:
			depth++
		case lexer.RBRACE:
			depth--
		case lexer.EOF:
			return fmt.Errorf("строка %d: не найдена закрывающая }", p.cur().Line)
		}
		p.advance()
	}
	return nil
}

// ParseProgram - точка входа парсера.
func (p *Parser) ParseProgram() (*ast.Program, error) {
	if err := p.collectFunctionSignatures(); err != nil {
		return nil, err
	}
	prog := &ast.Program{}
	p.skipNewlines()
	for p.cur().Type != lexer.EOF {
		switch p.cur().Type {
		case lexer.KwFunction:
			fn, err := p.parseFunction()
			if err != nil {
				return nil, err
			}
			prog.Functions = append(prog.Functions, fn)
		case lexer.KwEntity, lexer.KwClass:
			ent, err := p.parseEntity()
			if err != nil {
				return nil, err
			}
			prog.Entities = append(prog.Entities, ent)
		default:
			return nil, fmt.Errorf("строка %d, столбец %d: на верхнем уровне программы ожидалась \"функция\" или \"сущность\"/\"класс\", получено %q",
				p.cur().Line, p.cur().Col, p.cur().Value)
		}
		p.skipNewlines()
	}
	if len(prog.Functions) == 0 {
		return nil, fmt.Errorf("программа не содержит ни одной функции")
	}
	return prog, nil
}

// parseEntity разбирает "сущность"/"класс" (синонимы) и её тело.
func (p *Parser) parseEntity() (*ast.EntityDecl, error) {
	kw := p.advance() // "сущность" или "класс"
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после %q ожидалось имя сущности: %w", kw.Value, err)
	}
	p.skipNewlines()
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	p.skipSeparators()

	var fields []ast.Field
	if p.cur().Type == lexer.KwHas {
		fields, err = p.parseFieldsSection()
		if err != nil {
			return nil, fmt.Errorf("сущность %q: %w", name.Value, err)
		}
		p.skipSeparators()
	}

	var methods []*ast.MethodDecl
	for p.cur().Type == lexer.IDENT {
		m, err := p.parseMethod()
		if err != nil {
			return nil, fmt.Errorf("сущность %q: %w", name.Value, err)
		}
		methods = append(methods, m)
		p.skipSeparators()
	}

	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	return &ast.EntityDecl{Name: name.Value, Fields: fields, Methods: methods, Line: kw.Line}, nil
}

// parseMethod разбирает метод сущности: IDENT "(" [ params ] ")"
// [ ":" IDENT ] block - как функцию, но без ключевого слова "функция" и со
// скобками параметров обязательными (docs/GRAMMAR.md, "method"). Внутри
// тела метода включается p.inMethodBody - это то, что делает "этот"
// валидным (parsePrimary, parseFieldAssign).
func (p *Parser) parseMethod() (*ast.MethodDecl, error) {
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}
	if p.cur().Type != lexer.LPAREN {
		return nil, fmt.Errorf("строка %d, столбец %d: после имени метода %q ожидалось \"(\" (даже без параметров - \"()\")",
			p.cur().Line, p.cur().Col, name.Value)
	}
	params, err := p.parseParams()
	if err != nil {
		return nil, fmt.Errorf("в списке параметров метода %q: %w", name.Value, err)
	}
	returnType := ""
	if p.cur().Type == lexer.COLON {
		p.advance()
		var err error
		returnType, err = p.parseTypeName()
		if err != nil {
			return nil, fmt.Errorf("после \":\" у метода %q ожидалось имя типа результата: %w", name.Value, err)
		}
	}
	p.skipNewlines()
	prevInMethod := p.inMethodBody
	p.inMethodBody = true
	body, err := p.parseBlock()
	p.inMethodBody = prevInMethod
	if err != nil {
		return nil, err
	}
	return &ast.MethodDecl{Name: name.Value, Params: params, ReturnType: returnType, Body: body, Line: name.Line}, nil
}

// parseFieldsSection разбирает "имеет:" и поля, отступленные относительно
// самого слова "имеет" (docs/GRAMMAR.md, "Секции внутри объявления
// сущности"): поле принадлежит секции, пока отступ (Col) его первого
// токена строго больше Col метки "имеет"; иначе секция закончилась. Как и
// lookahead на "иначе" в parseIfStmt, решение "продолжать секцию или нет"
// принимается через непотребляющий peekPastNewlines() до того, как
// разделитель инструкций будет съеден - иначе первый же разделитель после
// последнего поля достался бы этой функции, а не следующему члену
// сущности. Позиция парсера на входе - на "имеет".
func (p *Parser) parseFieldsSection() ([]ast.Field, error) {
	label, err := p.expect(lexer.KwHas)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return nil, fmt.Errorf("после \"имеет\" ожидалось \":\": %w", err)
	}

	var fields []ast.Field
	for {
		next := p.peekPastNewlines()
		if next.Col <= label.Col || next.Type == lexer.RBRACE || next.Type == lexer.EOF {
			break
		}
		if err := p.requireSeparator(); err != nil {
			return nil, fmt.Errorf("перед полем: %w", err)
		}
		field, err := p.parseField()
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func (p *Parser) parseField() (ast.Field, error) {
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return ast.Field{}, fmt.Errorf("ожидалось имя поля: %w", err)
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return ast.Field{}, fmt.Errorf("после имени поля %q ожидалось \":\" и тип: %w", name.Value, err)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return ast.Field{}, fmt.Errorf("после \":\" у поля %q ожидалось имя типа: %w", name.Value, err)
	}
	return ast.Field{Name: name.Value, Type: typeName, Line: name.Line}, nil
}

func (p *Parser) parseFunction() (*ast.FunctionDecl, error) {
	kw, err := p.expect(lexer.KwFunction)
	if err != nil {
		return nil, err
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"функция\" ожидалось имя функции: %w", err)
	}

	var params []ast.Param
	if p.cur().Type == lexer.LPAREN {
		params, err = p.parseParams()
		if err != nil {
			return nil, fmt.Errorf("в списке параметров функции %q: %w", name.Value, err)
		}
	}

	returnType := ""
	if p.cur().Type == lexer.COLON {
		p.advance()
		var err error
		returnType, err = p.parseTypeName()
		if err != nil {
			return nil, fmt.Errorf("после \":\" у функции %q ожидалось имя типа результата: %w", name.Value, err)
		}
	}

	if name.Value == entryPointName && (len(params) > 0 || returnType != "") {
		return nil, fmt.Errorf("строка %d: %q - точка входа программы, у неё фиксированная сигнатура: нельзя объявить параметры или тип результата", kw.Line, entryPointName)
	}

	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.FunctionDecl{Name: name.Value, Params: params, ReturnType: returnType, Body: body, Line: kw.Line}, nil
}

// parseParams разбирает "(" [ param { "," param } ] ")". Позиция парсера
// на входе - на "(".
func (p *Parser) parseParams() ([]ast.Param, error) {
	p.advance() // "("
	var params []ast.Param
	seenCases := map[morph.Case]string{}
	if p.cur().Type != lexer.RPAREN {
		for {
			param, err := p.parseParam()
			if err != nil {
				return nil, err
			}
			c, _ := morph.CaseOfParamName(param.Name)
			if prev, exists := seenCases[c]; exists {
				return nil, fmt.Errorf("строка %d: параметр %q дублирует падежную роль параметра %q - у каждого падежа может быть только один параметр",
					param.Line, param.Name, prev)
			}
			seenCases[c] = param.Name
			params = append(params, param)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
		}
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return nil, err
	}
	return params, nil
}

// parseTypeName разбирает имя типа: IDENT [ "<" type_name ">" ] - обычное
// имя (примитив или сущность) либо generic-инстанциация вроде
// "Список<Целое>" (шаг 4.2). Кодируется в одну строку - AST продолжает
// хранить тип как string везде (Param.Type, Field.Type, ReturnType,
// NewExpr.Type), backend сам разбирает и резолвит эту строку
// (internal/backend/cbackend, resolveType). "<"/">" - уже существующие
// токены LT/GT (операторы сравнения) - неоднозначности с выражениями нет,
// это отдельная синтаксическая позиция (сразу после имени типа).
func (p *Parser) parseTypeName() (string, error) {
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return "", err
	}
	if p.cur().Type != lexer.LT {
		return name.Value, nil
	}
	p.advance() // "<"
	arg, err := p.parseTypeName()
	if err != nil {
		return "", fmt.Errorf("в аргументе типа %q: %w", name.Value, err)
	}
	if _, err := p.expect(lexer.GT); err != nil {
		return "", fmt.Errorf("после аргумента типа %q<%s> ожидалось \">\": %w", name.Value, arg, err)
	}
	return fmt.Sprintf("%s<%s>", name.Value, arg), nil
}

func (p *Parser) parseParam() (ast.Param, error) {
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return ast.Param{}, fmt.Errorf("ожидалось имя параметра: %w", err)
	}
	if _, ok := morph.CaseOfParamName(name.Value); !ok {
		return ast.Param{}, fmt.Errorf("строка %d, столбец %d: имя параметра должно быть одним из падежных вопросительных слов (%s), получено %q",
			name.Line, name.Col, strings.Join(morph.ParamNames, ", "), name.Value)
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return ast.Param{}, fmt.Errorf("после имени параметра %q ожидалось \":\" и тип: %w", name.Value, err)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return ast.Param{}, fmt.Errorf("после \":\" у параметра %q ожидалось имя типа: %w", name.Value, err)
	}
	return ast.Param{Name: name.Value, Type: typeName, Line: name.Line}, nil
}

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

func (p *Parser) parseStatement() (ast.Stmt, error) {
	switch p.cur().Type {
	case lexer.KwPrint:
		return p.parsePrintStmt()
	case lexer.KwVariable:
		return p.parseVarDecl()
	case lexer.KwIf:
		return p.parseIfStmt()
	case lexer.KwWhile:
		return p.parseWhileStmt()
	case lexer.KwFor:
		return p.parseForStmt()
	case lexer.KwReturn:
		return p.parseReturnStmt()
	case lexer.IDENT:
		if p.peekAt(1).Type == lexer.ASSIGN {
			return p.parseAssign()
		}
		if p.peekAt(1).Type == lexer.DOT {
			return p.parseDotStatement()
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
		return nil, fmt.Errorf("строка %d, столбец %d: \"этот\" в начале инструкции допустимо только с полем или методом (\"этот.поле = ...\"/\"этот.метод(...)\")",
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
func (p *Parser) parseCallExpr() (*ast.CallExpr, error) {
	name := p.advance() // IDENT
	args, err := p.parseArgList()
	if err != nil {
		return nil, fmt.Errorf("в аргументах вызова %q: %w", name.Value, err)
	}
	return &ast.CallExpr{Callee: name.Value, Args: args, Line: name.Line}, nil
}

// parseArgList разбирает "(" [ expr { "," expr } ] ")" - список аргументов,
// общий для строгого вызова функции, конструктора и вызова метода. Позиция
// парсера на входе - на "(".
func (p *Parser) parseArgList() ([]ast.Expr, error) {
	p.advance() // "("
	var args []ast.Expr
	if p.cur().Type != lexer.RPAREN {
		for {
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
		}
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return nil, err
	}
	return args, nil
}

// parseNewExpr разбирает "создать" IDENT "(" [ expr {"," expr} ] ")" -
// конструктор сущности (шаг 4). Только строгий синтаксис, аргументы
// позиционные по порядку полей - падежное согласование имени типа как
// прямого дополнения отложено (docs/GRAMMAR.md).
func (p *Parser) parseNewExpr() (ast.Expr, error) {
	kw := p.advance() // "создать"
	typeName, err := p.parseTypeName()
	if err != nil {
		return nil, fmt.Errorf("после \"создать\" ожидалось имя сущности: %w", err)
	}
	if p.cur().Type != lexer.LPAREN {
		return nil, fmt.Errorf("строка %d, столбец %d: после имени сущности %q ожидалось \"(\"", p.cur().Line, p.cur().Col, typeName)
	}
	args, err := p.parseArgList()
	if err != nil {
		return nil, fmt.Errorf("в аргументах конструктора %q: %w", typeName, err)
	}
	return &ast.NewExpr{Type: typeName, Args: args, Line: kw.Line}, nil
}

// naturalCallPrepositions - предлоги, разрешённые перед аргументом
// естественного вызова. Чисто декоративные (падеж определяет роль
// аргумента, не предлог, docs/GRAMMAR.md) - парсер их просто пропускает,
// не проверяя согласование с падежом следующего слова. Небольшой курируемый
// список (как и всё в этом движке, а не попытка перечислить все предлоги
// русского языка) - ровно то, что нужно каноническому примеру из
// docs/GRAMMAR.md; расширяется по мере надобности.
var naturalCallPrepositions = map[lexer.TokenType]bool{
	lexer.KwWith: true,
	lexer.KwIn:   true,
}

// parseNaturalCall разбирает естественный вызов - вызов функции без скобок,
// с аргументами, которые связываются с параметрами по падежу словоформы
// (docs/GRAMMAR.md, "Естественный вызов и падежное сопоставление"), а не по
// порядку слов в предложении. Позиция парсера на входе - на имени функции
// (IDENT), params - её параметры в порядке объявления (для построения
// CallExpr.Args по позиции, а не по порядку слов).
//
// Ограничения этой реализации (docs/SPEC.md): аргумент - всегда IDENT,
// ссылающийся на уже объявленную переменную (лемма из p.dict), не литерал;
// вся фраза - в одной строке (без продолжения по отступу).
func (p *Parser) parseNaturalCall(params []ast.Param) (*ast.CallExpr, error) {
	name := p.advance() // имя функции
	args := make([]ast.Expr, len(params))
	filled := make([]bool, len(params))
	remaining := len(params)

	for remaining > 0 {
		if naturalCallPrepositions[p.cur().Type] {
			p.advance() // декоративный предлог
		}
		argTok, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, fmt.Errorf("в естественном вызове %q: не хватает аргументов (не заполнено ещё %d): %w", name.Value, remaining, err)
		}

		analyses := p.dict.Analyze(argTok.Value)
		if len(analyses) == 0 {
			return nil, fmt.Errorf("строка %d, столбец %d: словоформа %q не опознана - естественный вызов понимает только ссылки на уже объявленные переменные",
				argTok.Line, argTok.Col, argTok.Value)
		}

		matchIdx := -1
		matchLemma := ""
		for i, param := range params {
			if filled[i] {
				continue
			}
			paramCase, _ := morph.CaseOfParamName(param.Name)
			for _, a := range analyses {
				if a.POS != morph.Noun || a.Tag.Case != paramCase {
					continue
				}
				if paramCase == morph.Accusative {
					lemma, ok := p.dict.Lookup(a.Lemma)
					if !ok || lemma.Noun == nil || (lemma.Noun.Animacy == morph.Animate) != morph.IsAnimateParamName(param.Name) {
						continue
					}
				}
				if matchIdx != -1 && (matchIdx != i || matchLemma != a.Lemma) {
					return nil, fmt.Errorf("строка %d, столбец %d: словоформа %q неоднозначна - подходит сразу нескольким незаполненным параметрам вызова %q",
						argTok.Line, argTok.Col, argTok.Value, name.Value)
				}
				matchIdx, matchLemma = i, a.Lemma
			}
		}
		if matchIdx == -1 {
			return nil, fmt.Errorf("строка %d, столбец %d: словоформа %q не подходит по падежу ни одному ещё не заполненному параметру вызова %q",
				argTok.Line, argTok.Col, argTok.Value, name.Value)
		}
		args[matchIdx] = &ast.Identifier{Name: matchLemma, Line: argTok.Line}
		filled[matchIdx] = true
		remaining--
	}
	return &ast.CallExpr{Callee: name.Value, Args: args, Line: name.Line}, nil
}

func (p *Parser) parseExpr() (ast.Expr, error) {
	return p.parseOr()
}

func (p *Parser) parseOr() (ast.Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.KwOr {
		op := p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{Op: "или", Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseAnd() (ast.Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.KwAnd {
		op := p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{Op: "и", Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseNot() (ast.Expr, error) {
	if p.cur().Type == lexer.KwNot {
		op := p.advance()
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "не", Operand: operand, Line: op.Line}, nil
	}
	return p.parseComparison()
}

var comparisonOps = map[lexer.TokenType]string{
	lexer.EQ:  "==",
	lexer.NEQ: "!=",
	lexer.LT:  "<",
	lexer.GT:  ">",
	lexer.LE:  "<=",
	lexer.GE:  ">=",
}

func (p *Parser) parseComparison() (ast.Expr, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	if opStr, ok := comparisonOps[p.cur().Type]; ok {
		op := p.advance()
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		return &ast.BinaryExpr{Op: opStr, Left: left, Right: right, Line: op.Line}, nil
	}
	return left, nil
}

func (p *Parser) parseAdditive() (ast.Expr, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.PLUS || p.cur().Type == lexer.MINUS {
		op := p.advance()
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		opStr := "+"
		if op.Type == lexer.MINUS {
			opStr = "-"
		}
		left = &ast.BinaryExpr{Op: opStr, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseMultiplicative() (ast.Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.STAR || p.cur().Type == lexer.SLASH {
		op := p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		opStr := "*"
		if op.Type == lexer.SLASH {
			opStr = "/"
		}
		left = &ast.BinaryExpr{Op: opStr, Left: left, Right: right, Line: op.Line}
	}
	return left, nil
}

func (p *Parser) parseUnary() (ast.Expr, error) {
	if p.cur().Type == lexer.MINUS {
		op := p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "-", Operand: operand, Line: op.Line}, nil
	}
	return p.parsePostfix()
}

// parsePostfix разбирает postfix := primary { "." IDENT }. Цикл (а не
// одиночная проверка) уже сейчас поддерживает цепочки (`x.поле.поле`),
// даже если шаг 4 сам их ещё не порождает - незачем переписывать этот
// уровень грамматики, когда они понадобятся.
func (p *Parser) parsePostfix() (ast.Expr, error) {
	expr, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.cur().Type == lexer.DOT {
		dot := p.advance()
		field, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, fmt.Errorf("после \".\" ожидалось имя поля или метода: %w", err)
		}
		if p.cur().Type == lexer.LPAREN {
			args, err := p.parseArgList()
			if err != nil {
				return nil, fmt.Errorf("в аргументах вызова метода %q: %w", field.Value, err)
			}
			expr = &ast.MethodCallExpr{Receiver: expr, Method: field.Value, Args: args, Line: dot.Line}
			continue
		}
		expr = &ast.FieldAccess{Receiver: expr, Field: field.Value, Line: dot.Line}
	}
	return expr, nil
}

func (p *Parser) parsePrimary() (ast.Expr, error) {
	tok := p.cur()
	switch tok.Type {
	case lexer.STRING:
		p.advance()
		return &ast.StringLiteral{Value: tok.Value, Line: tok.Line}, nil
	case lexer.INT:
		p.advance()
		v, err := strconv.ParseInt(tok.Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("строка %d, столбец %d: некорректный целочисленный литерал %q", tok.Line, tok.Col, tok.Value)
		}
		return &ast.IntLiteral{Value: v, Line: tok.Line}, nil
	case lexer.FLOAT:
		p.advance()
		v, err := strconv.ParseFloat(tok.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("строка %d, столбец %d: некорректный числовой литерал %q", tok.Line, tok.Col, tok.Value)
		}
		return &ast.FloatLiteral{Value: v, Line: tok.Line}, nil
	case lexer.KwTrue:
		p.advance()
		return &ast.BoolLiteral{Value: true, Line: tok.Line}, nil
	case lexer.KwFalse:
		p.advance()
		return &ast.BoolLiteral{Value: false, Line: tok.Line}, nil
	case lexer.IDENT:
		if p.peekAt(1).Type == lexer.LPAREN {
			return p.parseCallExpr()
		}
		p.advance()
		return &ast.Identifier{Name: tok.Value, Line: tok.Line}, nil
	case lexer.KwCreate:
		return p.parseNewExpr()
	case lexer.KwThis:
		p.advance()
		if !p.inMethodBody {
			return nil, fmt.Errorf("строка %d, столбец %d: \"этот\" валиден только внутри тела метода", tok.Line, tok.Col)
		}
		return &ast.ThisExpr{Line: tok.Line}, nil
	case lexer.LPAREN:
		p.advance()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.RPAREN); err != nil {
			return nil, err
		}
		return inner, nil
	default:
		return nil, fmt.Errorf("строка %d, столбец %d: ожидалось выражение, получено %q",
			tok.Line, tok.Col, tok.Value)
	}
}
