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

	// gerundWords - деепричастная форма -> имя функции, собрано тем же
	// предварительным проходом (collectFunctionSignatures), что и funcParams -
	// см. internal/parser/gerund.go, registerGerundWord.
	gerundWords map[string]string

	// genericFuncs - имена функций, объявленных с параметром типа
	// ("первый<T>(...)") - собрано тем же предварительным проходом,
	// что и funcParams. Нужен, чтобы разобрать "имя<Тип>(...)" как generic-
	// вызов, а не начало сравнения ("имя < ..."), не заглядывая внутрь "<...>"
	// вслепую - обычная функция без параметра типа никогда не читает "<"
	// сразу после своего имени как начало вызова.
	genericFuncs map[string]bool

	// pendingGT - см. expectCloseAngle/cur/advance.
	pendingGT bool

	// inMethodBody - true, пока парсер разбирает тело метода сущности
	// (parseMethod). "этот" валиден только пока это поле true - структурное
	// свойство (где мы сейчас в дереве разбора), не зависящее от типов,
	// поэтому проверяется здесь, а не в backend'е.
	inMethodBody bool

	// inConstructorBody - true, пока парсер разбирает тело явного
	// конструктора (parseConstructor) - то, что делает "родитель(...)"
	// валидным (parseParentCallStmt), тем же принципом, что и inMethodBody
	// у "этот". Тело конструктора всегда одновременно и inMethodBody (в нём
	// тоже валиден "этот.поле = ..."), поэтому оба поля включаются вместе.
	inConstructorBody bool

	// inAbstractEntity - true, пока парсер разбирает тело "абстрактная
	// сущность"/"абстрактный класс" (parseEntity) - то, что делает
	// отсутствие тела у метода валидным (parseMethod, ast.MethodDecl.
	// IsAbstract), тем же принципом, что и inMethodBody у "этот".
	inAbstractEntity bool

	// pendingVarDecls - отложенные объявления ("переменная X" без "=") в
	// ТЕКУЩЕМ блоке - см. pendingvar.go.
	pendingVarDecls []pendingVarDecl
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

// registerNoun лучшим образом пытается зарегистрировать имя (переменной,
// сущности, интерфейса) как существительное в dict. Ошибку намеренно
// игнорирует - не всякий идентификатор пригоден как русское
// существительное (однобуквенные имена вроде "i", латиница, слова с
// окончанием, которое ClassifyByEnding не берёт и которого нет в
// курируемых исключениях): такое имя просто не будет доступно для
// естественного вызова/склонения (например, объекта "наследует"/
// "реализует", см. resolveDeclinedTypeRef) - это не ошибка компиляции.
func (p *Parser) registerNoun(name string) {
	_ = p.dict.RegisterNoun(name, morph.AnimacyOf(name))
}

// cur/advance учитывают pendingGT (расщеплённая вторая половина ">>" -
// см. expectCloseAngle) раньше настоящего p.tokens[p.pos]: пока она не
// "использована", любой код, спрашивающий текущий токен, обязан увидеть
// ">", а не то, что реально стоит в потоке дальше.
func (p *Parser) cur() lexer.Token {
	if p.pendingGT {
		return lexer.Token{Type: lexer.GT, Value: ">", Line: p.tokens[p.pos].Line, Col: p.tokens[p.pos].Col}
	}
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
	if p.pendingGT {
		p.pendingGT = false
		return lexer.Token{Type: lexer.GT, Value: ">", Line: p.tokens[p.pos].Line, Col: p.tokens[p.pos].Col}
	}
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

// expectCloseAngle разбирает закрывающий ">" списка параметров/аргументов
// типа ("Коробка<T>") - лексер токенизирует "Коробка<Коробка<T>>" так, что
// два подряд идущих ">" сливаются в один токен SHR (побитовый сдвиг,
// значимый только в выражениях) - без расщепления вложенный generic
// ("Коробка<Коробка<T>>") было бы вообще невозможно разобрать. Расщепление
// затрагивает только эту позицию разбора (закрытие generic-скобок) -
// cur()/advance() иначе видят SHR как есть, оператор сдвига в обычных
// выражениях не задет.
func (p *Parser) expectCloseAngle() error {
	if p.cur().Type == lexer.GT {
		p.advance()
		return nil
	}
	if p.cur().Type == lexer.SHR && !p.pendingGT {
		if p.pos < len(p.tokens)-1 {
			p.pos++
		}
		p.pendingGT = true
		return nil
	}
	return fmt.Errorf("строка %d, столбец %d: ожидался \">\", получен %q", p.cur().Line, p.cur().Col, p.cur().Value)
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

// consumeDoc - если текущий токен DOC_COMMENT ("/** ... */"), возвращает
// его текст и продвигается за него и за последующие переносы строк (сам
// doc-комментарий стоит на собственной строке перед документируемым
// объявлением) - иначе просто "", ничего не потребляя. Несколько подряд
// идущих doc-комментариев (например, файловый заголовок в /** */, сразу
// за которым - собственный doc-комментарий объявления) поглощаются ВСЕ -
// побеждает ПОСЛЕДНИЙ (ближайший к объявлению), более ранние молча
// отбрасываются как обычная проза, а не привязываются ни к чему - раньше
// поглощался только первый, второй оставался нетронутым текущим токеном
// и вызывающая сторона пыталась разобрать его как начало ОБЪЯВЛЕНИЯ,
// давая невнятную ошибку "неожиданный токен ... в теле функции" вместо
// понятной. Вызывается в точке диспетчеризации ПЕРЕД тем, как решить,
// какое объявление разбирать дальше (ParseProgram, тело сущности, тело
// "имеет:") - сам doc-комментарий не становится отдельным ast.Stmt/узлом,
// он просто текст, привязанный к следующему объявлению.
func (p *Parser) consumeDoc() string {
	doc := ""
	for p.cur().Type == lexer.DOC_COMMENT {
		doc = normalizeDoc(p.cur().Value)
		p.advance()
		p.skipNewlines()
	}
	return doc
}

// normalizeDoc убирает построчное "оформление" многострочного
// doc-комментария в стиле JSDoc/Javadoc:
//
//	/**
//	 * Первая строка.
//	 * Вторая строка.
//	 */
//
// - с каждой строки снимаются ведущие пробелы/табы и ОДНА ведущая "*" (с
// пробелом после неё, если он есть), затем снимаются висящие пробелы в
// конце строки; строка без ведущей "*" (обычный многострочный текст без
// оформления) остаётся как есть, кроме обрезки пробелов по краям - оба
// стиля дают одинаково чистый результат. Результат в целом дополнительно
// обрезается по краям (TrimSpace) - пустые первая/последняя строки вокруг
// "/**"/"*/" не должны оставлять лишних пустых строк в hover.
func normalizeDoc(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		line = strings.TrimLeft(line, " \t")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimPrefix(line, " ")
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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

// peekPastNewlinesAndDoc - как peekPastNewlines, но пропускает вперёд ещё и
// "/** ... */" (в любом чередовании с переносами строк) - используется там,
// где решение "продолжать секцию или нет" нельзя принимать по самому
// doc-комментарию (parseFieldsSection): висячий комментарий в конце секции
// не должен ложно продлевать её только потому, что формально стоит на более
// глубоком отступе, чем метка секции.
func (p *Parser) peekPastNewlinesAndDoc() lexer.Token {
	offset := 0
	for {
		for p.peekAt(offset).Type == lexer.NEWLINE {
			offset++
		}
		if p.peekAt(offset).Type == lexer.DOC_COMMENT {
			offset++
			continue
		}
		return p.peekAt(offset)
	}
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
	generics := map[string]bool{}
	for p.cur().Type != lexer.EOF {
		switch p.cur().Type {
		case lexer.KwFunction:
			p.advance() // "функция"
			name, err := p.expect(lexer.IDENT)
			if err != nil {
				return fmt.Errorf("после \"функция\" ожидалось имя функции: %w", err)
			}
			typeParams, err := p.parseGenericParams()
			if err != nil {
				return fmt.Errorf("функция %q: %w", name.Value, err)
			}
			if typeParams != nil {
				generics[name.Value] = true
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
			var tag string
			if p.cur().Type == lexer.TAG {
				tag = p.advance().Value
			}
			if err := p.registerGerundWord(name.Value, tag); err != nil {
				return err
			}
			sigs[name.Value] = params
			if err := p.skipFunctionBody(); err != nil {
				return err
			}
		case lexer.KwExternal:
			switch p.peekAt(1).Type {
			case lexer.KwEntity:
				if _, err := p.parseExternEntityDecl(); err != nil {
					return err
				}
			case lexer.KwStruct:
				if _, err := p.parseExternStructDecl(); err != nil {
					return err
				}
			default:
				ext, err := p.parseExternFuncDecl()
				if err != nil {
					return err
				}
				sigs[ext.Alias] = ext.Params
			}
		default:
			p.advance()
		}
	}
	p.funcParams = sigs
	p.genericFuncs = generics
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
	// Инструкции верхнего уровня - тело неявной "главная" (см. ниже) - та
	// же область отложенных объявлений (pendingvar.go), что и у настоящего
	// тела функции.
	savedPending := p.beginPendingScope()
	for p.cur().Type != lexer.EOF {
		doc := p.consumeDoc()
		if p.cur().Type == lexer.EOF {
			// "/** ... */" висело в самом конце файла, не перед
			// объявлением - обычный комментарий, ни к чему не привязанный
			// (doc уже прочитан и отбрасывается).
			continue
		}
		vis, err := p.parseModuleVisibility()
		if err != nil {
			return nil, err
		}
		switch p.cur().Type {
		case lexer.KwFunction:
			fn, err := p.parseFunction()
			if err != nil {
				return nil, err
			}
			fn.Doc = doc
			fn.Visibility = vis
			if fn.Name == entryPointName {
				explicitMain = fn
			}
			prog.Functions = append(prog.Functions, fn)
		case lexer.KwAbstract, lexer.KwEntity, lexer.KwClass, lexer.KwStruct:
			ent, err := p.parseEntity()
			if err != nil {
				return nil, err
			}
			ent.Doc = doc
			ent.Visibility = vis
			prog.Entities = append(prog.Entities, ent)
		case lexer.KwExport:
			fn, ent, err := p.parseExportedDecl()
			if err != nil {
				return nil, err
			}
			if fn != nil {
				fn.Doc = doc
				fn.Visibility = vis
				prog.Functions = append(prog.Functions, fn)
			} else {
				ent.Doc = doc
				ent.Visibility = vis
				prog.Entities = append(prog.Entities, ent)
			}
		case lexer.KwUse:
			use, err := p.parseUseDecl()
			if err != nil {
				return nil, err
			}
			prog.Uses = append(prog.Uses, use)
		case lexer.KwExternal:
			switch p.peekAt(1).Type {
			case lexer.KwEntity:
				extEnt, err := p.parseExternEntityDecl()
				if err != nil {
					return nil, err
				}
				prog.ExternEntities = append(prog.ExternEntities, extEnt)
			case lexer.KwStruct:
				extStruct, err := p.parseExternStructDecl()
				if err != nil {
					return nil, err
				}
				extStruct.Doc = doc
				prog.Entities = append(prog.Entities, extStruct)
			default:
				ext, err := p.parseExternFuncDecl()
				if err != nil {
					return nil, err
				}
				prog.ExternFuncs = append(prog.ExternFuncs, ext)
			}
		case lexer.KwEnum:
			enum, err := p.parseEnumDecl()
			if err != nil {
				return nil, err
			}
			enum.Visibility = vis
			prog.Enums = append(prog.Enums, enum)
		case lexer.KwInterface:
			iface, err := p.parseInterfaceDecl()
			if err != nil {
				return nil, err
			}
			prog.Interfaces = append(prog.Interfaces, iface)
		case lexer.KwConst:
			// Отдельная ветка, не "default" -> parseStatement: константа
			// уровня модуля - ОБЪЯВЛЕНИЕ (как функция/сущность), не
			// топ-уровневая ИНСТРУКЦИЯ неявной "главная" - файл может иметь
			// её одновременно с явной "функция главная" без конфликта
			// "объявляет главная явно и одновременно содержит инструкции
			// на верхнем уровне" (см. ниже, после цикла).
			c, err := p.parseConstDecl()
			if err != nil {
				return nil, err
			}
			c.Doc = doc
			c.Visibility = vis
			prog.Consts = append(prog.Consts, c)
		default:
			stmt, err := p.parseStatement()
			if err != nil {
				return nil, err
			}
			if stmt != nil {
				// nil - "переменная X" без инициализации (pendingvar.go),
				// см. тот же случай в parseBlock.
				topLevelStmts = append(topLevelStmts, stmt)
			}
		}
		if p.cur().Type == lexer.EOF {
			break
		}
		if err := p.requireSeparator(); err != nil {
			return nil, err
		}
	}
	if err := p.endPendingScope(savedPending); err != nil {
		return nil, err
	}
	if len(topLevelStmts) > 0 {
		if explicitMain != nil {
			return nil, fmt.Errorf("программа объявляет функцию %q явно и одновременно содержит инструкции на верхнем уровне - выберите одну форму", entryPointName)
		}
		prog.Functions = append(prog.Functions, &ast.FunctionDecl{Name: entryPointName, Body: topLevelStmts})
	}
	return prog, nil
}

// parseEntity разбирает необязательный модификатор "абстрактная"/
// "абстрактный" перед "сущность"/"класс" (синонимы, ссылочная семантика)
// либо "структура" (та же форма тела, но семантика по значению - см.
// ast.EntityDecl.IsStruct) и тело объявления.
func (p *Parser) parseEntity() (*ast.EntityDecl, error) {
	var abstractTok *lexer.Token
	if p.cur().Type == lexer.KwAbstract {
		tok := p.advance()
		abstractTok = &tok
	}

	kw := p.advance() // "сущность", "класс" или "структура"
	label := "сущность"
	if kw.Type == lexer.KwStruct {
		label = "структура"
	}

	if abstractTok != nil {
		if kw.Type == lexer.KwStruct {
			return nil, fmt.Errorf("строка %d: \"структура\" не может быть абстрактной - абстрактность применима только к \"сущность\"/\"класс\" (наследование ограничено ссылочными типами)", abstractTok.Line)
		}
		want := "абстрактная"
		if kw.Type == lexer.KwClass {
			want = "абстрактный"
		}
		if abstractTok.Value != want {
			return nil, fmt.Errorf("строка %d, столбец %d: перед %q модификатор согласуется по роду - ожидалось %q, получено %q",
				abstractTok.Line, abstractTok.Col, kw.Value, want, abstractTok.Value)
		}
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
	p.registerNoun(name.Value)

	typeParams, err := p.parseGenericParams()
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", label, name.Value, err)
	}

	var baseName string
	var interfaces []string
	if p.cur().Type == lexer.KwInherits {
		verb := p.advance() // "наследует"
		base, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, fmt.Errorf("%s %q: после \"наследует\" ожидалось имя базовой сущности: %w", label, name.Value, err)
		}
		baseName, err = p.resolveDeclinedTypeRef(base, verb.Value)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", label, name.Value, err)
		}
	}
	if p.cur().Type == lexer.KwImplements {
		verb := p.advance() // "реализует"
		for {
			iface, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, fmt.Errorf("%s %q: после \"реализует\" ожидалось имя интерфейса: %w", label, name.Value, err)
			}
			ifaceName, err := p.resolveDeclinedTypeRef(iface, verb.Value)
			if err != nil {
				return nil, fmt.Errorf("%s %q: %w", label, name.Value, err)
			}
			interfaces = append(interfaces, ifaceName)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
		}
	}
	if kw.Type == lexer.KwStruct && (baseName != "" || len(interfaces) > 0) {
		return nil, fmt.Errorf("структура %q: \"наследует\"/\"реализует\" допустимы только у \"сущность\"/\"класс\" (наследование/интерфейсы ограничены ссылочными типами)", name.Value)
	}
	if typeParams != nil && (baseName != "" || len(interfaces) > 0) {
		return nil, fmt.Errorf("%s %q: generic-%s (с параметром типа) не может наследовать/реализовывать - не реализовано в этом срезе", label, name.Value, label)
	}
	if typeParams != nil && abstractTok != nil {
		return nil, fmt.Errorf("%s %q: generic-%s не может быть абстрактной - её нельзя ни унаследовать, ни (тем более) создать экземпляр", label, name.Value, label)
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
	var ctor *ast.ConstructorDecl

	prevInAbstractEntity := p.inAbstractEntity
	p.inAbstractEntity = abstractTok != nil
	defer func() { p.inAbstractEntity = prevInAbstractEntity }()

	for p.cur().Type != lexer.RBRACE {
		doc := p.consumeDoc()
		if p.cur().Type == lexer.RBRACE {
			// "/** ... */" висело в конце тела, не перед объявлением -
			// обычный комментарий, ни к чему не привязанный (doc уже
			// прочитан и отбрасывается).
			continue
		}
		switch p.cur().Type {
		case lexer.KwConstructor:
			if ctor != nil {
				return nil, fmt.Errorf("%s %q: конструктор объявлен более одного раза", label, name.Value)
			}
			c, err := p.parseConstructor()
			if err != nil {
				return nil, fmt.Errorf("%s %q: %w", label, name.Value, err)
			}
			ctor = c

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
			c.Doc = doc

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
			m.Doc = doc

			methods = append(methods, m)

		case lexer.KwPrivate, lexer.KwPublic, lexer.KwProtected:
			// Модификатор видимости перед "константа" ("[приватная|
			// публичная|защищённая] константа ИМЯ = ...") либо модификатор
			// видимости метода ("[приватный|публичный|защищённый]
			// МЕТОД...") - различаем по следующему токену без потребления
			// текущего.
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
				c.Visibility = visibilityFromToken(visTok)
				c.Doc = doc
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
			m2.Doc = doc
			methods = append(methods, m2)

		case lexer.KwStatic:
			// "статический [видимость] имя(...)" - статический метод
			// (константа уже статическая сама по себе, см. package doc
			// ast.MethodDecl.IsStatic - "статическая константа" не
			// поддержана, только поле/метод). parseMethod сам разбирает
			// ведущий "статический" (в отличие от видимости выше, здесь не
			// нужен lookahead - у "статический" перед константой нет
			// смысла).
			m3, err := p.parseMethod()
			if err != nil {
				return nil, fmt.Errorf(
					"%s %q: %w",
					label,
					name.Value,
					err,
				)
			}
			m3.Doc = doc
			methods = append(methods, m3)

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

	if kw.Type == lexer.KwStruct && ctor != nil {
		return nil, fmt.Errorf("структура %q: \"конструктор\" допустим только у \"сущность\"/\"класс\" (структура продолжает неявное позиционное присваивание полей)", name.Value)
	}

	return &ast.EntityDecl{
		Name:       name.Value,
		IsStruct:   kw.Type == lexer.KwStruct,
		IsAbstract: abstractTok != nil,
		BaseName:   baseName,
		Interfaces: interfaces,
		Constants:  constants,
		States:     states,
		Fields:     fields,
		Methods:    methods,
		Ctor:       ctor,
		TypeParams: typeParams,
		Line:       kw.Line,
	}, nil
}
