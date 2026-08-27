// Package parser строит ast.Program из потока токенов лексера.
//
// Грамматика v0.1 (расширяется по этапам из docs/ROADMAP.md, подробный
// EBNF-набросок - docs/GRAMMAR.md):
//
//	program     := { newline } { ( function | entity ) { newline } }
//	entity      := ( "сущность" | "класс" | "структура" ) IDENT newline "{" [ constants_section ] [ states_section ] [ fields_section ] [ methods_section ] "}"
//	states_section := "состояние" ":" { IDENT }        (* одна строка, шаг 4.1 *)
//	fields_section := "имеет" ":" { field }
//	field       := IDENT ":" IDENT
//	function    := "функция" IDENT [ "(" [ params ] ")" ] [ ":" IDENT ] newline block
//	params      := param { "," param }
//	param       := IDENT ":" IDENT
//	block       := "{" newline { statement } "}"
//	statement   := var_decl | assign | field_assign | become_stmt | print_stmt | if_stmt | while_stmt | for_stmt | return_stmt | call_stmt
//	var_decl    := "переменная" IDENT "=" expr
//	assign      := IDENT "=" expr
//	field_assign := IDENT "." IDENT "=" expr
//	become_stmt := ( IDENT | "этот" ) "становится" IDENT   (* шаг 4.1, упрощённая версия *)
//	print_stmt  := "вывести" expr
//	if_stmt     := "если" expr block [ "иначе" block ]
//	while_stmt  := "пока" expr block
//	for_stmt    := "для" IDENT "от" expr "до" expr block
//	             | "для" IDENT "в" NOUN_PREP block
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
	dict := morph.NewDictionary()
	// SystemVerbs - фиксированный, известный на этапе компиляции словарь
	// (docs/ROADMAP.md) - ошибка здесь означала бы дубль инфинитива внутри
	// самого SystemVerbs, а не что-то, зависящее от разбираемой программы,
	// поэтому это panic, а не ошибка New (у New и так нет возврата error).
	if err := morph.RegisterSystemVerbs(dict); err != nil {
		panic(fmt.Sprintf("internal/morph.SystemVerbs: %v", err))
	}
	return &Parser{tokens: tokens, dict: dict}
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
		switch p.cur().Type {
		case lexer.KwFunction:
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
		case lexer.KwExternal:
			ext, err := p.parseExternFuncDecl()
			if err != nil {
				return err
			}
			sigs[ext.Alias] = ext.Params
		default:
			p.advance()
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
// ParseProgram разбирает всю программу. Верхний уровень - обычно только
// "функция"/"сущность"/"класс" (как раньше), но любая другая инструкция на
// верхнем уровне разбирается как часть тела неявной точки входа: файл без
// объявленной "главная" явно, где верхний уровень содержит инструкции
// (переменные, "вывести" и т. п.), собирает их по порядку встречи в
// одну синтезированную "главная" - плоский, скриптовый стиль записи для
// программы без явной обёртки функцией, наравне с обычной формой (docs/
// SPEC.md). Явно объявленная "главная" и инструкции на верхнем уровне в
// одном файле одновременно - ошибка (два конкурирующих определения точки
// входа), а не одно из них молча отбрасывается.
func (p *Parser) ParseProgram() (*ast.Program, error) {
	if err := p.collectFunctionSignatures(); err != nil {
		return nil, err
	}
	prog := &ast.Program{}
	var topLevelStmts []ast.Stmt
	var explicitMain *ast.FunctionDecl
	p.skipSeparators()
	for p.cur().Type != lexer.EOF {
		switch p.cur().Type {
		case lexer.KwFunction:
			fn, err := p.parseFunction()
			if err != nil {
				return nil, err
			}
			if fn.Name == entryPointName {
				explicitMain = fn
			}
			prog.Functions = append(prog.Functions, fn)
		case lexer.KwEntity, lexer.KwClass, lexer.KwStruct:
			ent, err := p.parseEntity()
			if err != nil {
				return nil, err
			}
			prog.Entities = append(prog.Entities, ent)
		case lexer.KwUse:
			use, err := p.parseUseDecl()
			if err != nil {
				return nil, err
			}
			prog.Uses = append(prog.Uses, use)
		case lexer.KwExternal:
			ext, err := p.parseExternFuncDecl()
			if err != nil {
				return nil, err
			}
			prog.ExternFuncs = append(prog.ExternFuncs, ext)
		case lexer.KwEnum:
			enum, err := p.parseEnumDecl()
			if err != nil {
				return nil, err
			}
			prog.Enums = append(prog.Enums, enum)
		default:
			stmt, err := p.parseStatement()
			if err != nil {
				return nil, err
			}
			topLevelStmts = append(topLevelStmts, stmt)
		}
		if p.cur().Type == lexer.EOF {
			break
		}
		if err := p.requireSeparator(); err != nil {
			return nil, err
		}
	}
	if len(topLevelStmts) > 0 {
		if explicitMain != nil {
			return nil, fmt.Errorf("программа объявляет функцию %q явно и одновременно содержит инструкции на верхнем уровне - выберите одну форму", entryPointName)
		}
		prog.Functions = append(prog.Functions, &ast.FunctionDecl{Name: entryPointName, Body: topLevelStmts})
	}
	return prog, nil
}

// parseEntity разбирает "сущность"/"класс" (синонимы, ссылочная семантика)
// либо "структура" (та же форма тела, но семантика по значению - см.
// ast.EntityDecl.IsStruct) и тело объявления.
func (p *Parser) parseEntity() (*ast.EntityDecl, error) {
	kw := p.advance() // "сущность", "класс" или "структура"
	label := "сущность"
	if kw.Type == lexer.KwStruct {
		label = "структура"
	}

	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf(
			"после %q ожидалось имя %s: %w",
			kw.Value,
			label,
			err,
		)
	}

	p.skipNewlines()

	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}

	p.skipSeparators()

	var constants []*ast.ConstDecl
	var states []ast.StateDecl
	var fields []ast.Field
	var methods []*ast.MethodDecl

	for p.cur().Type != lexer.RBRACE {
		switch p.cur().Type {
		case lexer.KwConst:
			c, err := p.parseConstDecl()
			if err != nil {
				return nil, fmt.Errorf(
					"%s %q: %w",
					label,
					name.Value,
					err,
				)
			}

			constants = append(constants, c)

		case lexer.KwState:
			parsedStates, err := p.parseStatesSection(name.Value)
			if err != nil {
				return nil, fmt.Errorf(
					"%s %q: %w",
					label,
					name.Value,
					err,
				)
			}

			states = append(states, parsedStates...)

		case lexer.KwHas:
			parsedFields, err := p.parseFieldsSection()
			if err != nil {
				return nil, fmt.Errorf(
					"%s %q: %w",
					label,
					name.Value,
					err,
				)
			}

			fields = append(fields, parsedFields...)

		case lexer.IDENT:
			m, err := p.parseMethod()
			if err != nil {
				return nil, fmt.Errorf(
					"%s %q: %w",
					label,
					name.Value,
					err,
				)
			}

			methods = append(methods, m)

		case lexer.KwPrivate, lexer.KwPublic:
			// Модификатор видимости перед "константа" ("[приватная|
			// публичная] константа ИМЯ = ...") либо модификатор видимости
			// метода ("[приватный|публичный] МЕТОД...") - различаем по
			// следующему токену без потребления текущего.
			if p.peekAt(1).Type == lexer.KwConst {
				visTok := p.advance()
				if err := checkConstVisibilityAgreement(visTok); err != nil {
					return nil, fmt.Errorf("%s %q: %w", label, name.Value, err)
				}
				c, err := p.parseConstDecl()
				if err != nil {
					return nil, fmt.Errorf(
						"%s %q: %w",
						label,
						name.Value,
						err,
					)
				}
				c.Private = visTok.Type == lexer.KwPrivate
				constants = append(constants, c)
				break
			}
			m2, err := p.parseMethod()
			if err != nil {
				return nil, fmt.Errorf(
					"%s %q: %w",
					label,
					name.Value,
					err,
				)
			}
			methods = append(methods, m2)

		default:
			return nil, fmt.Errorf(
				"%s %q: неожиданный токен %q",
				label,
				name.Value,
				p.cur().Value,
			)
		}

		p.skipSeparators()
	}

	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}

	return &ast.EntityDecl{
		Name:      name.Value,
		IsStruct:  kw.Type == lexer.KwStruct,
		Constants: constants,
		States:    states,
		Fields:    fields,
		Methods:   methods,
		Line:      kw.Line,
	}, nil
}
