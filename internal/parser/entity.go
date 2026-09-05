package parser

import (
	"fmt"
	"unicode"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
)

func (p *Parser) parseStatesSection(entityName string) ([]ast.StateDecl, error) {
	label, err := p.expect(lexer.KwState)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return nil, fmt.Errorf("после \"состояние\" ожидалось \":\": %w", err)
	}
	var words []lexer.Token
	for p.cur().Type == lexer.IDENT && p.cur().Line == label.Line {
		words = append(words, p.advance())
	}
	if len(words) == 0 {
		return nil, fmt.Errorf("строка %d: после \"состояние:\" ожидалось хотя бы одно название состояния", label.Line)
	}

	gender, number, ok := morph.CompoundNounAgreement(entityName)
	if !ok {
		return nil, fmt.Errorf("строка %d: не удалось определить род сущности %q (окончание не классифицируется, слова нет в словаре OpenCorpora) - без рода нельзя вывести имя производного типа состояния",
			label.Line, entityName)
	}

	states := make([]ast.StateDecl, len(words))
	for i, w := range words {
		long, err := p.stateWordAgreement(w.Value, gender, number)
		if err != nil {
			return nil, fmt.Errorf("строка %d, столбец %d: состояние %q: %w", w.Line, w.Col, w.Value, err)
		}
		become, err := p.stateWordInstrumentalAgreement(w.Value, gender, number)
		if err != nil {
			return nil, fmt.Errorf("строка %d, столбец %d: состояние %q (творительный падеж для \"становится\"): %w", w.Line, w.Col, w.Value, err)
		}
		states[i] = ast.StateDecl{Name: w.Value, TypeName: capitalizeFirst(long) + entityName, BecomeForm: become, Line: w.Line}
	}
	return states, nil
}

// stateWordAgreementFn - подпись morph.LongFormAdjective/
// morph.ActiveParticipleAgreement и их падежных вариантов
// (morph.InstrumentalLongFormAdjective/morph.InstrumentalActiveParticiple,
// instrumental.go) - общая, чтобы stateWordAgreementUsing могла принять
// любую пару из них, не дублируя диспетчеризацию по источнику слова.
type stateWordAgreementFn func(string, morph.Gender, morph.Number) (string, error)

// stateWordAgreementUsing выводит полную, согласованную по роду/числу форму
// слова состояния из word - двумя источниками (шаг 6.4 добавил второй
// наравне с уже существующим первым), одна и та же диспетчеризация нужна
// дважды (именительный падеж для производного имени типа состояния -
// stateWordAgreement, творительный для BecomeStmt -
// stateWordInstrumentalAgreement), меняется только то, какая конкретно
// пара функций согласования вызывается:
//
//  1. Действительное причастие прошедшего времени известного глагола
//     ("вошедший" из "войти", зарегистрированного в SystemVerbs -
//     p.dict.Analyze находит его как VerbForm == morph.FormPastParticiple) -
//     word уже сам по себе полная форма (муж. рода), participleFn
//     согласует её с gender/number.
//  2. Краткая форма страдательного причастия ("авторизован", "закрыт") -
//     adjectiveFn.
//
// Первый источник проверяется раньше: он специфичнее (конкретный
// зарегистрированный глагол), второй - общее суффиксальное правило.
func (p *Parser) stateWordAgreementUsing(word string, gender morph.Gender, number morph.Number, participleFn, adjectiveFn stateWordAgreementFn) (string, error) {
	for _, a := range p.dict.Analyze(word) {
		if a.POS == morph.Verb && a.VerbForm == morph.FormPastParticiple {
			return participleFn(word, gender, number)
		}
	}
	return adjectiveFn(word, gender, number)
}

// stateWordAgreement - именительный падеж (производное имя типа состояния,
// "АвторизованныйПользователь").
func (p *Parser) stateWordAgreement(word string, gender morph.Gender, number morph.Number) (string, error) {
	return p.stateWordAgreementUsing(word, gender, number, morph.ActiveParticipleAgreement, morph.LongFormAdjective)
}

// stateWordInstrumentalAgreement - творительный падеж (StateDecl.BecomeForm,
// то, что обязан написать программист после "становится").
func (p *Parser) stateWordInstrumentalAgreement(word string, gender morph.Gender, number morph.Number) (string, error) {
	return p.stateWordAgreementUsing(word, gender, number, morph.InstrumentalActiveParticiple, morph.InstrumentalLongFormAdjective)
}

// capitalizeFirst делает первую букву s заглавной - для имени
// производного типа состояния (parseStatesSection), синтезированного из
// строчного прилагательного.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// parseBecomeStmt разбирает переход состояния (шаг 4.1, упрощённая
// версия): "<получатель> становится <состояние>". Получатель - тот же
// класс, что и у присваивания полю (IDENT переменной или "этот",
// parseFieldAssign) - структурно узнаётся по лукахеду в parseStatement, не
// по типу. Позиция парсера на входе - на получателе.
func (p *Parser) parseBecomeStmt() (ast.Stmt, error) {
	receiverTok := p.advance()
	var receiver ast.Expr
	if receiverTok.Type == lexer.KwThis {
		if !p.inMethodBody {
			return nil, fmt.Errorf("строка %d, столбец %d: \"этот\" валиден только внутри тела метода", receiverTok.Line, receiverTok.Col)
		}
		receiver = &ast.ThisExpr{Line: receiverTok.Line}
	} else {
		receiver = &ast.Identifier{Name: receiverTok.Value, Line: receiverTok.Line}
	}
	kw, err := p.expect(lexer.KwBecomes)
	if err != nil {
		return nil, err
	}
	state, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после \"становится\" ожидалось название состояния: %w", err)
	}
	return &ast.BecomeStmt{Receiver: receiver, State: state.Value, Line: kw.Line}, nil
}

// parseMethod разбирает метод сущности: [приватный|публичный] IDENT "("
// [ params ] ")" [ ":" IDENT ] block - как функцию, но без ключевого слова
// "функция" и со скобками параметров обязательными (docs/GRAMMAR.md,
// "method"). Внутри тела метода включается p.inMethodBody - это то, что
// делает "этот" валидным (parsePrimary, parseFieldAssign).
//
// Модификатор видимости у метода не согласуется по роду, в отличие
// от поля (parseField) - у метода нет рода (называется глаголом), поэтому
// допустима только форма мужского рода "приватный"/"публичный"/
// "защищённый", любая другая форма ("приватная" и т.п.) - ошибка
// компиляции, а не угаданное согласование.
//
// block необязателен, только если p.inAbstractEntity (метод без тела внутри
// "абстрактная сущность"/"абстрактный класс") - тогда результат несёт
// IsAbstract == true и Body == nil (docs/SPEC.md, "Абстрактные методы").
func (p *Parser) parseMethod() (*ast.MethodDecl, error) {
	isStatic := false
	if p.cur().Type == lexer.KwStatic {
		tok := p.advance()
		if tok.Value != "статический" {
			return nil, fmt.Errorf("строка %d, столбец %d: у метода модификатор \"статический\" не согласуется по роду (метод - не существительное) - нужна форма \"статический\", получено %q",
				tok.Line, tok.Col, tok.Value)
		}
		isStatic = true
	}
	var vis ast.Visibility
	if isVisibilityKeyword(p.cur().Type) {
		tok := p.advance()
		vis = visibilityFromToken(tok)
		want := methodVisibilityWord(vis)
		if tok.Value != want {
			return nil, fmt.Errorf("строка %d, столбец %d: у метода модификатор видимости не согласуется по роду (метод - не существительное) - нужна форма %q, получено %q",
				tok.Line, tok.Col, want, tok.Value)
		}
	}
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
	selfReturn := false
	if p.cur().Type == lexer.COLON {
		p.advance()
		if p.cur().Type == lexer.KwThis {
			p.advance()
			selfReturn = true
		} else {
			var err error
			returnType, err = p.parseTypeName()
			if err != nil {
				return nil, fmt.Errorf("после \":\" у метода %q ожидалось имя типа результата: %w", name.Value, err)
			}
			returnType = normalizeReturnType(returnType)
		}
	}
	var tag string
	if p.cur().Type == lexer.TAG {
		tag = p.advance().Value
	}
	if p.peekPastNewlines().Type != lexer.LBRACE {
		if !p.inAbstractEntity {
			return nil, fmt.Errorf("строка %d, столбец %d: у метода %q не указано тело - тело может отсутствовать только внутри \"абстрактная сущность\"/\"абстрактный класс\"",
				p.cur().Line, p.cur().Col, name.Value)
		}
		if isStatic {
			return nil, fmt.Errorf("строка %d, столбец %d: статический метод %q не может быть абстрактным - у него нет получателя, отменять/переопределять через наследование нечего",
				p.cur().Line, p.cur().Col, name.Value)
		}
		return &ast.MethodDecl{Name: name.Value, Params: params, ReturnType: returnType, SelfReturn: selfReturn, IsAbstract: true, Visibility: vis, Line: name.Line, Tag: tag}, nil
	}
	p.skipNewlines()
	prevInMethod := p.inMethodBody
	// Статический метод разбирается как обычная функция - inMethodBody
	// остаётся false, поэтому "этот" внутри его тела грамматически
	// недопустим, той же проверкой, что и вне метода вообще (у него нет
	// получателя, см. package doc ast.MethodDecl.IsStatic).
	p.inMethodBody = !isStatic
	restoreVarTypes := p.beginVarTypeScope(params)
	body, err := p.parseBlock()
	restoreVarTypes()
	p.inMethodBody = prevInMethod
	if err != nil {
		return nil, err
	}
	return &ast.MethodDecl{Name: name.Value, Params: params, ReturnType: returnType, SelfReturn: selfReturn, Body: body, Visibility: vis, IsStatic: isStatic, Line: name.Line, Tag: tag}, nil
}

// resolveDeclinedTypeRef разбирает имя после "наследует"/"реализует" -
// оба глагола переходные и управляют винительным падежом ("наследовать
// кого/что", "реализовать что"), поэтому объект склоняется, а не берётся
// словарной формой как ключевое слово (тот же принцип, что и у падежного
// сопоставления аргументов естественного вызова - docs/GRAMMAR.md). tok -
// уже прочитанный IDENT-токен объекта, verb - "наследует"/"реализует" (для
// сообщения об ошибке).
//
// Три исхода: (1) tok.Value - известная винительная форма зарегистрированной
// сущности/интерфейса (p.registerNoun вызывается при разборе каждого
// объявления) - возвращает её именительную лемму (настоящее имя типа); (2)
// tok.Value - ИМЕНИТЕЛЬНАЯ форма такой сущности/интерфейса, у которой
// винительный отличается - явная ошибка (та же строгость, что и у
// checkNewKeywordAgreement/checkVisibilityAgreement, не тихое принятие
// неверной формы); (3) склонение неопределимо (имя не зарегистрировано или
// вне покрытия Фазы A, включая составные имена вроде "ОшибкаДеления" -
// курируемый классификатор Фазы A не различает голову и родительный
// довесок составного имени, слепо склоняет хвост всей строки как одно
// простое слово, см. docs/GRAMMAR.md - "Открытый вопрос: склонение
// составных имён") - permissive fallback, tok.Value принимается как есть,
// тем же принципом, что и везде в языке.
func (p *Parser) resolveDeclinedTypeRef(tok lexer.Token, verb string) (string, error) {
	for _, a := range p.dict.Analyze(tok.Value) {
		if a.POS == morph.Noun && a.Tag.Case == morph.Accusative {
			return a.Lemma, nil
		}
	}
	for _, a := range p.dict.Analyze(tok.Value) {
		if a.POS != morph.Noun || a.Tag.Case != morph.Nominative {
			continue
		}
		if acc, ok := p.dict.FormOf(a.Lemma, morph.Accusative); ok && acc != tok.Value {
			return "", fmt.Errorf("строка %d, столбец %d: после %q ожидается винительный падеж - %q, получено %q (именительный)",
				tok.Line, tok.Col, verb, acc, tok.Value)
		}
	}
	return tok.Value, nil
}

// parseConstructor разбирает явное тело конструктора: "конструктор" "("
// [ params ] ")" block - как parseMethod, но без имени, типа результата и
// модификатора видимости (конструктор не может быть переопределён/вызван
// как обычный метод, видимость к нему неприменима в этом срезе). Внутри
// тела включаются оба inMethodBody (нужен для "этот") и inConstructorBody
// (нужен для "родитель(...)", parseParentCallStmt).
func (p *Parser) parseConstructor() (*ast.ConstructorDecl, error) {
	kw, err := p.expect(lexer.KwConstructor)
	if err != nil {
		return nil, err
	}
	if p.cur().Type != lexer.LPAREN {
		return nil, fmt.Errorf("строка %d, столбец %d: после \"конструктор\" ожидалось \"(\" (даже без параметров - \"()\")",
			p.cur().Line, p.cur().Col)
	}
	params, err := p.parseParams()
	if err != nil {
		return nil, fmt.Errorf("в списке параметров конструктора: %w", err)
	}
	p.skipNewlines()
	prevInMethod, prevInCtor := p.inMethodBody, p.inConstructorBody
	p.inMethodBody, p.inConstructorBody = true, true
	restoreVarTypes := p.beginVarTypeScope(params)
	body, err := p.parseBlock()
	restoreVarTypes()
	p.inMethodBody, p.inConstructorBody = prevInMethod, prevInCtor
	if err != nil {
		return nil, err
	}
	return &ast.ConstructorDecl{Params: params, Body: body, Line: kw.Line}, nil
}

// parseParentCallStmt разбирает "родитель(<арг>, ...)" - вызов конструктора
// базовой сущности (docs, ast.ParentCallStmt). Валиден только внутри тела
// конструктора - структурная проверка здесь же (inConstructorBody), не в
// backend'е. Позиция парсера на входе - на "родитель".
func (p *Parser) parseParentCallStmt() (ast.Stmt, error) {
	kw := p.advance() // "родитель"
	if !p.inConstructorBody {
		return nil, fmt.Errorf("строка %d, столбец %d: \"родитель(...)\" валидно только внутри тела конструктора", kw.Line, kw.Col)
	}
	if p.cur().Type != lexer.LPAREN {
		return nil, fmt.Errorf("строка %d, столбец %d: после \"родитель\" ожидалось \"(\"", p.cur().Line, p.cur().Col)
	}
	args, err := p.parseArgList()
	if err != nil {
		return nil, fmt.Errorf("в аргументах \"родитель(...)\": %w", err)
	}
	return &ast.ParentCallStmt{Args: args, Line: kw.Line}, nil
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
		// peekPastNewlinesAndDoc, не peekPastNewlines - висячий
		// doc-комментарий в конце секции (не перед реальным полем) не
		// должен ложно продлевать её только потому, что формально стоит на
		// отступе глубже метки "имеет".
		next := p.peekPastNewlinesAndDoc()
		if next.Type == lexer.RBRACE || next.Type == lexer.EOF {
			// Секция закончилась - если по дороге был пропущен doc-
			// комментарий, он висит перед самой "}" (концом тела сущности),
			// ни к чему не привязан - съедаем его тоже, как обычный
			// комментарий, чтобы не запнуться на нём снаружи (parseEntity).
			for p.cur().Type == lexer.NEWLINE || p.cur().Type == lexer.DOC_COMMENT {
				p.advance()
			}
			break
		}
		if next.Col <= label.Col {
			// Секция закончилась - но если по дороге был пропущен doc-
			// комментарий, он привязан к СЛЕДУЮЩЕМУ члену сущности (не
			// полю), не висит без дела - оставляем его непотреблённым,
			// внешний цикл (parseEntity) заберёт его своим consumeDoc().
			break
		}
		if err := p.requireSeparator(); err != nil {
			return nil, fmt.Errorf("перед полем: %w", err)
		}
		doc := p.consumeDoc()
		field, err := p.parseField()
		if err != nil {
			return nil, err
		}
		field.Doc = doc
		fields = append(fields, field)
	}
	return fields, nil
}

// parseField разбирает одно поле, необязательно с модификатором "статический"
// и/или модификатором видимости перед именем: "[статический|статическая|
// статическое] [приватный|публичный|защищённый] имя: Тип". Оба модификатора
// согласуются по роду с именем поля тем же принципом, что и "новый"/"новая"
// у конструктора (см. visibilityKeywordFor/checkVisibilityAgreement) - без
// модификатора видимости поле публичное (Field.Visibility == ast.Public), без
// "статический" - обычное поле экземпляра (Field.IsStatic == false, см.
// package doc ast.Field.IsStatic).
func (p *Parser) parseField() (ast.Field, error) {
	var staticTok *lexer.Token
	if p.cur().Type == lexer.KwStatic {
		tok := p.advance()
		staticTok = &tok
	}
	var visTok *lexer.Token
	if isVisibilityKeyword(p.cur().Type) {
		tok := p.advance()
		visTok = &tok
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return ast.Field{}, fmt.Errorf("ожидалось имя поля: %w", err)
	}
	if staticTok != nil {
		if err := checkStaticAgreement(*staticTok, name.Value); err != nil {
			return ast.Field{}, fmt.Errorf("поле %q: %w", name.Value, err)
		}
	}
	var vis ast.Visibility
	if visTok != nil {
		vis = visibilityFromToken(*visTok)
		if err := checkVisibilityAgreement(*visTok, vis, name.Value); err != nil {
			return ast.Field{}, fmt.Errorf("поле %q: %w", name.Value, err)
		}
	}
	if _, err := p.expect(lexer.COLON); err != nil {
		return ast.Field{}, fmt.Errorf("после имени поля %q ожидалось \":\" и тип: %w", name.Value, err)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return ast.Field{}, fmt.Errorf("после \":\" у поля %q ожидалось имя типа: %w", name.Value, err)
	}
	var def ast.Expr
	if p.cur().Type == lexer.ASSIGN {
		p.advance()
		def, err = p.parseExpr()
		if err != nil {
			return ast.Field{}, fmt.Errorf("в значении по умолчанию поля %q: %w", name.Value, err)
		}
	}
	var tag string
	if p.cur().Type == lexer.TAG {
		tag = p.advance().Value
	}
	return ast.Field{Name: name.Value, Type: typeName, Visibility: vis, IsStatic: staticTok != nil, Line: name.Line, Default: def, Tag: tag}, nil
}

// staticKeywordFor - форма модификатора "статический", согласованная по
// роду/числу - тем же принципом, что и visibilityKeywordFor.
func staticKeywordFor(gender morph.Gender, number morph.Number) string {
	if number == morph.Plural {
		return "статические"
	}
	switch gender {
	case morph.Feminine:
		return "статическая"
	case morph.Neuter:
		return "статическое"
	default:
		return "статический"
	}
}

// checkStaticAgreement - как checkVisibilityAgreement, но для модификатора
// "статический" перед именем поля.
func checkStaticAgreement(kw lexer.Token, fieldName string) error {
	gender, number, ok := morph.CompoundNounAgreement(fieldName)
	if !ok {
		return nil
	}
	want := staticKeywordFor(gender, number)
	if kw.Value != want {
		return fmt.Errorf("строка %d, столбец %d: модификатор \"статический\" не согласован по роду с именем поля - требует форму %q, получено %q",
			kw.Line, kw.Col, want, kw.Value)
	}
	return nil
}

// isVisibilityKeyword - true для любого из трёх модификаторов видимости
// (приватный/публичный/защищённый, в любой согласованной форме).
func isVisibilityKeyword(t lexer.TokenType) bool {
	return t == lexer.KwPrivate || t == lexer.KwPublic || t == lexer.KwProtected
}

// methodVisibilityWord - форма модификатора видимости у метода: всегда
// мужской род единственного числа (метод называется глаголом, у него нет
// рода, поэтому согласование по роду поля/константы к нему не относится).
func methodVisibilityWord(vis ast.Visibility) string {
	return visibilityKeywordFor(vis, morph.Masculine, morph.Singular)
}

// visibilityFromToken переводит токен-модификатор видимости
// (KwPrivate/KwPublic/KwProtected) в ast.Visibility.
func visibilityFromToken(tok lexer.Token) ast.Visibility {
	switch tok.Type {
	case lexer.KwPrivate:
		return ast.Private
	case lexer.KwProtected:
		return ast.Protected
	default:
		return ast.Public
	}
}

func visibilityKeywordFor(vis ast.Visibility, gender morph.Gender, number morph.Number) string {
	stem := "публичн"
	switch vis {
	case ast.Private:
		stem = "приватн"
	case ast.Protected:
		stem = "защищённ"
	}
	if number == morph.Plural {
		return stem + "ые"
	}
	switch gender {
	case morph.Feminine:
		return stem + "ая"
	case morph.Neuter:
		return stem + "ое"
	default:
		return stem + "ый"
	}
}

// checkVisibilityAgreement проверяет, что фактически написанная форма
// модификатора видимости поля (kw.Value) согласуется по роду/числу с
// именем поля fieldName - permissive fallback, как и checkNewKeywordAgreement:
// если morph.NounAgreement не смогла определить род/число имени поля,
// подходит любая форма.
func checkVisibilityAgreement(kw lexer.Token, vis ast.Visibility, fieldName string) error {
	gender, number, ok := morph.CompoundNounAgreement(fieldName)
	if !ok {
		return nil
	}
	want := visibilityKeywordFor(vis, gender, number)
	if kw.Value != want {
		return fmt.Errorf("строка %d, столбец %d: требует форму модификатора видимости %q, получено %q",
			kw.Line, kw.Col, want, kw.Value)
	}
	return nil
}

// checkConstVisibilityAgreement - то же самое, что checkVisibilityAgreement,
// но для константы сущности: модификатор здесь согласуется не с именем
// самой константы (она обычно записана КАПС_ЛОКОМ_С_ПОДЧЁРКИВАНИЯМИ и не
// является обычным склоняемым существительным - checkVisibilityAgreement
// на таком имени просто ничего не проверил бы, morph.NounAgreement его не
// узнает), а со словом "константа" - оно всегда женского рода единственного
// числа, поэтому единственные допустимые формы - "приватная"/"публичная".
func checkConstVisibilityAgreement(kw lexer.Token) error {
	want := visibilityKeywordFor(visibilityFromToken(kw), morph.Feminine, morph.Singular)
	if kw.Value != want {
		return fmt.Errorf("строка %d, столбец %d: перед \"константа\" модификатор видимости согласуется со словом \"константа\" (женский род, единственное число) - ожидалось %q, получено %q",
			kw.Line, kw.Col, want, kw.Value)
	}
	return nil
}
