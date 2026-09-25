// Package parser строит ast.Program из потока токенов лексера.
//
// Грамматика v0.1 (расширяется по этапам из подробный
// EBNF-набросок - docs/GRAMMAR_DRAFT.md):
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
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

// entryPointName дублирует cbackend.entryPointName - парсеру нужно знать
// имя точки входа, чтобы запретить у неё параметры и тип результата (у
// главной фиксированная C-сигнатура). Не импортируется из cbackend, чтобы
// parser не зависел от backend'а (архитектурный инвариант, см.
// ).
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

	// declGen - номер текущего объявления функции/метода/конструктора
	// (растёт с каждым новым), nounDecl - в каком из них имя последний раз
	// зарегистрировано (registerNoun). Словарь dict общий на файл, поэтому
	// переменная из соседней функции тоже в нём есть - при неоднозначной
	// словоформе побеждает имя текущего объявления (см.
	// resolvePrepositionalNoun).
	declGen  int
	nounDecl map[string]int

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

	// entityMethods - параметры всех методов каждой сущности программы,
	// собранные тем же предварительным проходом, что и funcParams -
	// entityMethods[имя_сущности][имя_метода] - нужен для естественного
	// вызова метода (natural_method_call.go): получатель уже известного
	// сущностного типа (varEntityTypes/currentEntityName) плюс имя метода
	// сразу после него однозначно определяют список параметров, так же как
	// funcParams делает это для имени свободной функции.
	entityMethods map[string]map[string][]ast.Param

	// varEntityTypes - переменная/параметр -> имя сущностного типа, если он
	// указан ЯВНО (аннотация "переменная x: Тип"/типизированная форма
	// "Тип x = ..."/тип параметра функции или метода - у параметра тип
	// всегда явный). Заполняется по ходу разбора ТЕЛА текущей функции/
	// метода (не предварительным проходом - тип переменной в общем случае
	// неизвестен до места её объявления, а вывод типа из значения парсеру
	// недоступен). Нужен для той же цели, что и entityMethods -
	// естественный вызов метода не может, в отличие от строгого
	// "receiver.метод(...)", отличить получателя от произвольной
	// переменной без явного знания её типа (парсер не выводит типы).
	varEntityTypes map[string]string

	// currentEntityName - имя сущности, чьё тело сейчас разбирается
	// (parseEntity), "" вне сущности. Вместе с inMethodBody даёт тип
	// получателя "этот" для естественного вызова метода - тем же
	// принципом, что и varEntityTypes для обычной переменной-получателя.
	currentEntityName string

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

	// lenient - true только в ParseProgramLenient (LSP, см. recovery.go) -
	// ошибка внутри ОДНОГО верхнеуровневого объявления не останавливает
	// разбор всего файла: она копится в errors, а разбор продолжается со
	// следующего объявления (recoverToNextTopLevel). ParseProgram (настоящая
	// сборка) этот путь не проходит вовсе - там первая же ошибка остаётся
	// фатальной, как и раньше.
	lenient bool
	errors  []error
}

func New(tokens []lexer.Token) *Parser {
	dict := morph.NewDictionary()
	// SystemVerbs - фиксированный, известный на этапе компиляции словарь
	// - ошибка здесь означала бы дубль инфинитива внутри
	// самого SystemVerbs, а не что-то, зависящее от разбираемой программы,
	// поэтому это panic, а не ошибка New (у New и так нет возврата error).
	if err := morph.RegisterSystemVerbs(dict); err != nil {
		panic(fmt.Sprintf("internal/morph.SystemVerbs: %v", err))
	}
	return &Parser{tokens: tokens, dict: dict}
}

// Dict возвращает морфологический словарь, накопленный по ходу разбора
// (все переменные/типы/catch-переменные программы, зарегистрированные
// registerNoun) - нужен внешнему коду (internal/lsp, для морфологически
// осведомлённого "подсветить все вхождения"/"найти ссылки" - словоформа
// вроде "паролем" должна опознаваться как ссылка на переменную "пароль")
// без пересборки того же словаря заново.
func (p *Parser) Dict() *morph.Dictionary {
	return p.dict
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
	if p.nounDecl == nil {
		p.nounDecl = map[string]int{}
	}
	p.nounDecl[name] = p.declGen
}

// enterDecl - начало нового объявления функции/метода/конструктора (см.
// Parser.declGen).
func (p *Parser) enterDecl() {
	p.declGen++
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
// atCloseAngle сообщает, стоит ли парсер прямо на закрывающей ">" списка
// generic-параметров/аргументов типа - GT или ПЕРВАЯ половина ">>" (SHR,
// разбираемого на два отдельных ">" у вложенных generic, см. pendingGT/
// expectCloseAngle) - нужен отдельно от expectCloseAngle, чтобы проверить
// висячую запятую перед закрывающей ">" БЕЗ побочного эффекта продвижения
// позиции (expectCloseAngle сам продвигается).
func (p *Parser) atCloseAngle() bool {
	return p.cur().Type == lexer.GT || (p.cur().Type == lexer.SHR && !p.pendingGT)
}

// skipNewlinesAfterAngleComma - тот же принцип, что и
// skipNewlinesAfterListComma, для generic-параметров/аргументов типа
// ("<...>"), где закрывающий токен - не единственный TokenType (см.
// atCloseAngle - GT или ещё не расщеплённый SHR).
func (p *Parser) skipNewlinesAfterAngleComma() (bool, error) {
	if p.atCloseAngle() {
		return false, errAt(p.cur().Line, p.cur().Col, CodeUnclosedBracket, "висячая запятая перед \">\" требует переноса строки - закрывающая скобка обязана оказаться на своей строке (либо уберите запятую)")
	}
	p.skipNewlines()
	return p.atCloseAngle(), nil
}

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
	return errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "ожидался \">\", получен %q", p.cur().Value)
}

func (p *Parser) expect(tt lexer.TokenType) (lexer.Token, error) {
	if p.cur().Type != tt {
		return lexer.Token{}, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "ожидался %s, получен %q", tt, p.cur().Value)
	}
	return p.advance(), nil
}

func (p *Parser) skipNewlines() {
	for p.cur().Type == lexer.NEWLINE {
		p.advance()
	}
}

// skipNewlinesAfterListComma - вызывается сразу после уже потреблённой ","
// разделителя многострочного списка в скобках ("(...)"/"[...]"/"<...>":
// параметры, аргументы вызова/конструктора, список типов "Вызываемый(...)",
// литерал списка, generic-параметры/аргументы типа) - пропускает переносы
// строк и сообщает, не оказалась ли следом сразу closing (висячая запятая
// перед закрывающей скобкой, тот же PSR-12-подобный стиль, что и перенос
// внутри самого списка) - вызывающая сторона обязана прервать цикл элементов
// списка, если результат true. Висячая запятая БЕЗ переноса строки прямо
// перед closing ("f(a, b,)" одной строкой) - ошибка, а не молча принятый
// синоним "f(a, b)": иначе один и тот же список можно было бы написать
// двумя ничем не различимыми способами без причины предпочесть один - по
// решению пользователя висячая запятая обязана сопровождаться переносом
// закрывающей скобки на свою строку.
func (p *Parser) skipNewlinesAfterListComma(closing lexer.TokenType) (bool, error) {
	if p.cur().Type == closing {
		return false, errAt(p.cur().Line, p.cur().Col, CodeUnclosedBracket, "висячая запятая перед %q требует переноса строки - закрывающая скобка обязана оказаться на своей строке (либо уберите запятую)", closing)
	}
	p.skipNewlines()
	return p.cur().Type == closing, nil
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
func (p *Parser) consumeDoc() (string, error) {
	doc := ""
	for p.cur().Type == lexer.DOC_COMMENT {
		if err := p.checkDocAttached(); err != nil {
			return "", err
		}
		doc = normalizeDoc(p.cur().Value)
		p.advance()
		p.skipNewlines()
	}
	return doc, nil
}

// errDocInBody - doc-комментарий перед инструкцией в теле функции/метода/
// ветки "когда": документируются только объявления.
func (p *Parser) errDocInBody() error {
	return errAtLine(p.cur().Line, CodeFloatingDoc, "doc-комментарий \"/** ... */\" внутри тела ни к чему не относится - документируются только объявления (функция, метод, сущность, поле, константа); для обычного комментария используйте \"//\"")
}

// checkDocAttached - текущий токен DOC_COMMENT обязан стоять вплотную перед
// тем, что он документирует: следующий токен - на следующей же строке
// после "*/" и не "}"/конец файла/ещё один doc-комментарий. Пустая строка
// или //-комментарий между ними (лексер их не отдаёт, виден только разрыв
// в номерах строк) - ошибка: иначе описание файла или соседнего блока
// молча прилипало бы к ближайшему объявлению (hover показывал бы чужой
// текст).
func (p *Parser) checkDocAttached() error {
	doc := p.cur()
	offset := 1
	for p.peekAt(offset).Type == lexer.NEWLINE {
		offset++
	}
	next := p.peekAt(offset)
	endLine := doc.Line + strings.Count(doc.Value, "\n")
	switch {
	case next.Type == lexer.EOF || next.Type == lexer.RBRACE:
		return errAtLine(doc.Line, CodeFloatingDoc, "doc-комментарий \"/** ... */\" ни к чему не относится - после него нет объявления; для обычного комментария используйте \"//\"")
	case next.Type == lexer.DOC_COMMENT || next.Line != endLine+1:
		return errAtLine(doc.Line, CodeFloatingDoc, "doc-комментарий \"/** ... */\" отделён от объявления (пустая строка, \"//\"-комментарий или ещё один doc-комментарий между ними) - поставьте его вплотную перед объявлением или замените на \"//\"")
	}
	return nil
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
	return p.peekPastNewlinesFrom(0)
}

// peekPastNewlinesFrom - как peekPastNewlines, но начинает поиск с
// произвольного смещения от текущей позиции, а не сразу от неё - нужен,
// когда решение "продолжается ли инструкция на следующей строке" зависит от
// токена ПОСЛЕ уже известного текущего (например, IDENT в начале
// инструкции - смотрим на смещение 1, ожидая "=" присваивания).
func (p *Parser) peekPastNewlinesFrom(start int) lexer.Token {
	offset := start
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
		return errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "инструкции должны быть разделены переносом строки или \";\", получено %q", p.cur().Value)
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
	entityMethods := map[string]map[string][]ast.Param{}
	for p.cur().Type != lexer.EOF {
		start := p.pos
		if err := p.collectOneSignature(sigs, generics, entityMethods); err != nil {
			if !p.lenient {
				return err
			}
			// Ошибка здесь - LSP best-effort проход (natural-call
			// disambiguation), не источник итоговой диагностики: та же
			// сломанная декларация встретится ещё раз в основном цикле
			// ParseProgram, где и попадёт в p.errors по-настоящему - здесь
			// достаточно молча пропустить её и не задваивать сообщение.
			p.pos = start
			p.recoverToNextTopLevel()
		}
	}
	p.funcParams = sigs
	p.genericFuncs = generics
	p.entityMethods = entityMethods
	return nil
}

// collectOneSignature разбирает ОДНО верхнеуровневое объявление в проходе
// collectFunctionSignatures - вынесено отдельно от цикла ради
// recoverToNextTopLevel (см. lenient-ветку выше), которому нужна позиция
// начала объявления, а не только факт ошибки.
func (p *Parser) collectOneSignature(sigs map[string][]ast.Param, generics map[string]bool, entityMethods map[string]map[string][]ast.Param) error {
	switch p.cur().Type {
	case lexer.KwAbstract, lexer.KwEntity, lexer.KwClass, lexer.KwStruct:
		return p.collectEntitySignature(entityMethods)
	case lexer.KwExport:
		next := p.peekAt(1).Type
		if next == lexer.KwAbstract || next == lexer.KwEntity || next == lexer.KwClass || next == lexer.KwStruct {
			p.advance() // "экспортировать"
			return p.collectEntitySignature(entityMethods)
		}
		if isExportListStart(next) {
			// Списковая форма (exportlist.go) не объявляет ни функцию, ни
			// сущность - только мутирует Exported у уже разобранных узлов,
			// этому проходу (сигнатуры для естественного вызова) нечего в
			// ней собирать, но токены нужно съесть целиком - иначе внешний
			// цикл collectFunctionSignatures увидит "Точка"/"(" как начало
			// нового (и невалидного) верхнеуровневого объявления.
			_, err := p.parseExportPathList()
			return err
		}
		p.advance()
		return nil
	case lexer.KwFunction:
		p.advance() // "функция"
		name, err := p.expect(lexer.IDENT)
		if err != nil {
			return diagnostics.Wrap(err, "после \"функция\" ожидалось имя функции")
		}
		typeParams, err := p.parseGenericParams()
		if err != nil {
			return diagnostics.Wrap(err, "функция %q", name.Value)
		}
		if typeParams != nil {
			generics[name.Value] = true
		}
		var params []ast.Param
		if p.cur().Type == lexer.LPAREN {
			params, err = p.parseParams()
			if err != nil {
				return diagnostics.Wrap(err, "в списке параметров функции %q", name.Value)
			}
		}
		if p.cur().Type == lexer.COLON {
			p.advance()
			if _, err := p.parseTypeName(); err != nil {
				return diagnostics.Wrap(err, "после \":\" у функции %q ожидалось имя типа результата", name.Value)
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
		return p.skipFunctionBody()
	case lexer.KwExternal:
		switch p.peekAt(1).Type {
		case lexer.KwEntity:
			_, err := p.parseExternEntityDecl()
			return err
		case lexer.KwStruct:
			_, err := p.parseExternStructDecl()
			return err
		default:
			ext, err := p.parseExternFuncDecl()
			if err != nil {
				return err
			}
			sigs[ext.Alias] = ext.Params
			return nil
		}
	default:
		p.advance()
		return nil
	}
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
			return errAtLine(p.cur().Line, CodeUnclosedBracket, "не найдена закрывающая }")
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
		start := p.pos
		skip, err := p.parseTopLevelItem(prog, &topLevelStmts, &explicitMain)
		if err != nil {
			if !p.lenient {
				return nil, err
			}
			// Сломанное объявление - откат к его собственному началу
			// (позиция после ошибки может быть где угодно внутри его тела,
			// подсчёт скобок в recoverToNextTopLevel обязан начаться от 0
			// именно с этой точки) и пропуск целиком до следующего.
			p.errors = append(p.errors, err)
			p.pos = start
			p.recoverToNextTopLevel()
			continue
		}
		if skip {
			continue
		}
		if p.cur().Type == lexer.EOF {
			break
		}
		if err := p.requireSeparator(); err != nil {
			if !p.lenient {
				return nil, err
			}
			p.errors = append(p.errors, err)
			p.recoverToNextTopLevel()
		}
	}
	if err := p.endPendingScope(savedPending); err != nil {
		if !p.lenient {
			return nil, err
		}
		p.errors = append(p.errors, err)
		p.pendingVarDecls = savedPending
	}
	if len(topLevelStmts) > 0 {
		if explicitMain != nil {
			err := errAtLine(explicitMain.Line, CodeEntryPoint, "программа объявляет функцию %q явно и одновременно содержит инструкции на верхнем уровне - выберите одну форму", entryPointName)
			if !p.lenient {
				return nil, err
			}
			// Best-effort: явная "главная" уже в prog.Functions - лишние
			// инструкции верхнего уровня просто отбрасываются, не
			// синтезируя вторую, конфликтующую "главная".
			p.errors = append(p.errors, err)
		} else {
			prog.Functions = append(prog.Functions, &ast.FunctionDecl{Name: entryPointName, Body: topLevelStmts})
		}
	}
	return prog, nil
}

// parseTopLevelItem разбирает ОДНО верхнеуровневое объявление или (если ни
// одно ключевое слово не подошло) инструкцию неявной "главная" - вынесено
// из ParseProgram ради recoverToNextTopLevel в lenient-режиме, которому, как
// и collectOneSignature, нужна позиция САМОГО НАЧАЛА объявления, а не только
// факт ошибки (см. вызывающий цикл). skip=true - "/** ... */" висел в самом
// конце файла, не перед объявлением - обычный комментарий, ни к чему не
// привязанный (doc уже прочитан и отброшен), разбирать нечего.
func (p *Parser) parseTopLevelItem(prog *ast.Program, topLevelStmts *[]ast.Stmt, explicitMain **ast.FunctionDecl) (skip bool, err error) {
	doc, err := p.consumeDoc()
	if err != nil {
		return false, err
	}
	if p.cur().Type == lexer.EOF {
		return true, nil
	}
	vis, err := p.parseModuleVisibility()
	if err != nil {
		return false, err
	}
	isAsync, err := p.parseAsyncModifier()
	if err != nil {
		return false, err
	}
	switch p.cur().Type {
	case lexer.KwFunction:
		fn, err := p.parseFunction(isAsync)
		if err != nil {
			return false, err
		}
		fn.Doc = doc
		fn.Visibility = vis
		if fn.Name == entryPointName {
			*explicitMain = fn
		}
		prog.Functions = append(prog.Functions, fn)
	case lexer.KwAbstract, lexer.KwEntity, lexer.KwClass, lexer.KwStruct:
		ent, err := p.parseEntity()
		if err != nil {
			return false, err
		}
		ent.Doc = doc
		ent.Visibility = vis
		prog.Entities = append(prog.Entities, ent)
	case lexer.KwExport:
		if isExportListStart(p.peekAt(1).Type) {
			items, err := p.parseExportPathList()
			if err != nil {
				return false, err
			}
			for _, item := range items {
				if err := applyExportItem(prog, item); err != nil {
					return false, err
				}
			}
			break
		}
		fn, ent, err := p.parseExportedDecl()
		if err != nil {
			return false, err
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
			return false, err
		}
		prog.Uses = append(prog.Uses, use)
	case lexer.HASH:
		block, err := p.parsePlatformBlock()
		if err != nil {
			return false, err
		}
		prog.PlatformBlocks = append(prog.PlatformBlocks, block)
	case lexer.KwExternal:
		switch p.peekAt(1).Type {
		case lexer.KwEntity:
			extEnt, err := p.parseExternEntityDecl()
			if err != nil {
				return false, err
			}
			extEnt.Doc = doc
			prog.ExternEntities = append(prog.ExternEntities, extEnt)
		case lexer.KwStruct:
			extStruct, err := p.parseExternStructDecl()
			if err != nil {
				return false, err
			}
			extStruct.Doc = doc
			prog.Entities = append(prog.Entities, extStruct)
		default:
			ext, err := p.parseExternFuncDecl()
			if err != nil {
				return false, err
			}
			ext.Doc = doc
			prog.ExternFuncs = append(prog.ExternFuncs, ext)
		}
	case lexer.KwEnum:
		enum, err := p.parseEnumDecl()
		if err != nil {
			return false, err
		}
		enum.Visibility = vis
		prog.Enums = append(prog.Enums, enum)
	case lexer.KwInterface:
		iface, err := p.parseInterfaceDecl()
		if err != nil {
			return false, err
		}
		prog.Interfaces = append(prog.Interfaces, iface)
	case lexer.KwConst:
		// Отдельная ветка, не "default" -> parseStatement: константа
		// уровня модуля - ОБЪЯВЛЕНИЕ (как функция/сущность), не
		// топ-уровневая ИНСТРУКЦИЯ неявной "главная" - файл может иметь
		// её одновременно с явной "функция главная" без конфликта
		// "объявляет главная явно и одновременно содержит инструкции
		// на верхнем уровне" (см. ParseProgram, после цикла).
		c, err := p.parseConstDecl()
		if err != nil {
			return false, err
		}
		c.Doc = doc
		c.Visibility = vis
		prog.Consts = append(prog.Consts, c)
	case lexer.KwSynonym:
		syn, err := p.parseSynonymDecl()
		if err != nil {
			return false, err
		}
		syn.Doc = doc
		syn.Visibility = vis
		prog.Synonyms = append(prog.Synonyms, syn)
	default:
		stmt, err := p.parseStatement()
		if err != nil {
			return false, err
		}
		if stmt != nil {
			// nil - "переменная X" без инициализации (pendingvar.go),
			// см. тот же случай в parseBlock.
			*topLevelStmts = append(*topLevelStmts, stmt)
		}
	}
	return false, nil
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
			return nil, errAtLine(abstractTok.Line, CodeEntityInvariant, "\"структура\" не может быть абстрактной - абстрактность применима только к \"сущность\"/\"класс\" (наследование ограничено ссылочными типами)")
		}
		want := "абстрактная"
		if kw.Type == lexer.KwClass {
			want = "абстрактный"
		}
		if abstractTok.Value != want {
			return nil, errAt(abstractTok.Line, abstractTok.Col, CodeModifierAgree, "перед %q модификатор согласуется по роду", kw.Value).
				WithHint("ожидалось %q, получено %q", want, abstractTok.Value)
		}
	}

	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, diagnostics.Wrap(err, "после %q ожидалось имя %s", kw.Value, label)
	}
	p.registerNoun(name.Value)

	typeParams, err := p.parseGenericParams()
	if err != nil {
		return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
	}

	var baseName string
	var interfaces []string
	if p.cur().Type == lexer.KwInherits {
		verb := p.advance() // "наследует"
		base, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, diagnostics.Wrap(err, "%s %q: после \"наследует\" ожидалось имя базовой сущности", label, name.Value)
		}
		baseName, err = p.resolveDeclinedTypeRef(base, verb.Value)
		if err != nil {
			return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
		}
		// "наследует Коробку<Целое>" / "Коробку<T>" - база-инстанциация
		// generic-сущности, склоняется только имя шаблона.
		if baseName, err = p.parseTypeArgsSuffix(baseName); err != nil {
			return nil, diagnostics.Wrap(err, "%s %q: базовая сущность", label, name.Value)
		}
	}
	if p.cur().Type == lexer.KwImplements {
		verb := p.advance() // "реализует"
		for {
			iface, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, diagnostics.Wrap(err, "%s %q: после \"реализует\" ожидалось имя интерфейса", label, name.Value)
			}
			ifaceName, err := p.resolveDeclinedTypeRef(iface, verb.Value)
			if err != nil {
				return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
			}
			interfaces = append(interfaces, ifaceName)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
			p.skipNewlines()
		}
	}
	if kw.Type == lexer.KwStruct && (baseName != "" || len(interfaces) > 0) {
		return nil, errAt(name.Line, name.Col, CodeEntityInvariant, "структура %q: \"наследует\"/\"реализует\" допустимы только у \"сущность\"/\"класс\" (наследование/интерфейсы ограничены ссылочными типами)", name.Value)
	}
	if typeParams != nil && abstractTok != nil {
		return nil, errAt(name.Line, name.Col, CodeEntityInvariant, "%s %q: generic-%s не может быть абстрактной - её нельзя ни унаследовать, ни (тем более) создать экземпляр", label, name.Value, label)
	}

	var tag string
	if p.cur().Type == lexer.TAG {
		tag = p.advance().Value
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

	prevEntityName := p.currentEntityName
	p.currentEntityName = name.Value
	defer func() { p.currentEntityName = prevEntityName }()

	for p.cur().Type != lexer.RBRACE {
		doc, err := p.consumeDoc()
		if err != nil {
			return nil, err
		}
		if p.cur().Type == lexer.RBRACE {
			// "/** ... */" висело в конце тела, не перед объявлением -
			// обычный комментарий, ни к чему не привязанный (doc уже
			// прочитан и отбрасывается).
			continue
		}
		switch p.cur().Type {
		case lexer.KwConstructor:
			if ctor != nil {
				return nil, errAt(p.cur().Line, p.cur().Col, CodeDuplicateDecl, "%s %q: конструктор объявлен более одного раза", label, name.Value)
			}
			c, err := p.parseConstructor()
			if err != nil {
				return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
			}
			ctor = c

		case lexer.KwConst:
			c, err := p.parseConstDecl()
			if err != nil {
				return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
			}
			c.Doc = doc

			constants = append(constants, c)

		case lexer.KwState:
			parsedStates, err := p.parseStatesSection(name.Value)
			if err != nil {
				return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
			}

			states = append(states, parsedStates...)

		case lexer.KwHas:
			parsedFields, err := p.parseFieldsSection()
			if err != nil {
				return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
			}

			fields = append(fields, parsedFields...)

		case lexer.IDENT:
			m, err := p.parseMethod()
			if err != nil {
				return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
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
					return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
				}
				c, err := p.parseConstDecl()
				if err != nil {
					return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
				}
				c.Visibility = visibilityFromToken(visTok)
				c.Doc = doc
				constants = append(constants, c)
				break
			}
			m2, err := p.parseMethod()
			if err != nil {
				return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
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
				return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
			}
			m3.Doc = doc
			methods = append(methods, m3)

		case lexer.KwExport:
			// "экспортировать конструктор(...)" - конструктор, видимый за
			// границей разделяемой библиотеки (docs/SPEC.md, "Экспорт в
			// разделяемую библиотеку") - в отличие от метода, конструктор
			// нельзя вызвать снаружи по умолчанию вовсе, только автор явно
			// решает, пересекает ли создание объекта границу ABI.
			if p.peekAt(1).Type == lexer.KwConstructor {
				if ctor != nil {
					return nil, errAt(p.cur().Line, p.cur().Col, CodeDuplicateDecl, "%s %q: конструктор объявлен более одного раза", label, name.Value)
				}
				p.advance() // "экспортировать"
				c, err := p.parseConstructor()
				if err != nil {
					return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
				}
				c.Exported = true
				ctor = c
				break
			}
			// "экспортировать имя(...)" - метод, видимый за границей
			// разделяемой библиотеки - только форма без "статический"/
			// видимости: экспорт сам по себе делает метод публичным на
			// границе ABI, сочетание с ними проверяется сразу здесь, а не в
			// internal/semantic (та же строгость, что и у остальных
			// модификаторов метода выше).
			expTok := p.advance()
			m4, err := p.parseMethod()
			if err != nil {
				return nil, diagnostics.Wrap(err, "%s %q", label, name.Value)
			}
			if m4.IsStatic {
				return nil, errAtLine(expTok.Line, CodeUnexpectedToken, "\"экспортировать\" перед статическим методом %q не поддержан - экспортируемый метод обязан иметь получателя", m4.Name)
			}
			if m4.IsAbstract {
				return nil, errAtLine(expTok.Line, CodeEntityInvariant, "\"экспортировать\" перед абстрактным методом %q не поддержан - экспортированный символ обязан иметь реализацию", m4.Name)
			}
			if m4.Visibility != ast.Public {
				return nil, errAtLine(expTok.Line, CodeUnexpectedToken, "\"экспортировать\" метода %q не сочетается с модификатором видимости - экспорт сам по себе делает метод видимым за границей библиотеки", m4.Name)
			}
			m4.Exported = true
			m4.Doc = doc
			methods = append(methods, m4)

		default:
			return nil, errAt(p.cur().Line, p.cur().Col, CodeUnexpectedToken, "%s %q: неожиданный токен %q", label, name.Value, p.cur().Value)
		}

		p.skipSeparators()
	}

	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}

	if kw.Type == lexer.KwStruct && ctor != nil {
		return nil, errAt(name.Line, name.Col, CodeEntityInvariant, "структура %q: \"конструктор\" допустим только у \"сущность\"/\"класс\" (структура продолжает неявное позиционное присваивание полей)", name.Value)
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
		Tag:        tag,
	}, nil
}
