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

	gender, number, ok := morph.NounAgreement(entityName)
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
		states[i] = ast.StateDecl{Name: w.Value, TypeName: capitalizeFirst(long) + entityName, Line: w.Line}
	}
	return states, nil
}

// stateWordAgreement выводит полную, согласованную по роду/числу форму
// слова состояния из word - двумя источниками (шаг 6.4 добавил второй
// наравне с уже существующим первым):
//
//  1. Действительное причастие прошедшего времени известного глагола
//     ("вошедший" из "войти", зарегистрированного в SystemVerbs -
//     p.dict.Analyze находит его как VerbForm == morph.FormPastParticiple) -
//     word уже сам по себе полная форма (муж. рода), morph.
//     ActiveParticipleAgreement согласует её с gender/number.
//  2. Краткая форма страдательного причастия ("авторизован", "закрыт") -
//     как и раньше, morph.LongFormAdjective.
//
// Первый источник проверяется раньше: он специфичнее (конкретный
// зарегистрированный глагол), второй - общее суффиксальное правило.
func (p *Parser) stateWordAgreement(word string, gender morph.Gender, number morph.Number) (string, error) {
	for _, a := range p.dict.Analyze(word) {
		if a.POS == morph.Verb && a.VerbForm == morph.FormPastParticiple {
			return morph.ActiveParticipleAgreement(word, gender, number)
		}
	}
	return morph.LongFormAdjective(word, gender, number)
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
// Модификатор видимости (шаг 4.3) у метода не согласуется по роду, в отличие
// от поля (parseField) - у метода нет рода (называется глаголом), поэтому
// допустима только форма мужского рода "приватный"/"публичный", любая другая
// форма ("приватная" и т.п.) - ошибка компиляции, а не угаданное согласование.
func (p *Parser) parseMethod() (*ast.MethodDecl, error) {
	var private bool
	if p.cur().Type == lexer.KwPrivate || p.cur().Type == lexer.KwPublic {
		tok := p.advance()
		private = tok.Type == lexer.KwPrivate
		want := "публичный"
		if private {
			want = "приватный"
		}
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
	return &ast.MethodDecl{Name: name.Value, Params: params, ReturnType: returnType, Body: body, Private: private, Line: name.Line}, nil
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

// parseField разбирает одно поле, необязательно с модификатором видимости
// (шаг 4.3) перед именем: "[приватный|публичный] имя: Тип". Модификатор
// согласуется по роду с именем поля тем же принципом, что и "новый"/"новая"
// у конструктора (см. visibilityKeywordFor/checkVisibilityAgreement) - без
// модификатора поле публичное (Field.Private == false).
func (p *Parser) parseField() (ast.Field, error) {
	var visTok *lexer.Token
	if p.cur().Type == lexer.KwPrivate || p.cur().Type == lexer.KwPublic {
		tok := p.advance()
		visTok = &tok
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return ast.Field{}, fmt.Errorf("ожидалось имя поля: %w", err)
	}
	private := visTok != nil && visTok.Type == lexer.KwPrivate
	if visTok != nil {
		if err := checkVisibilityAgreement(*visTok, private, name.Value); err != nil {
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
	return ast.Field{Name: name.Value, Type: typeName, Private: private, Line: name.Line}, nil
}

// visibilityKeywordFor возвращает форму модификатора видимости
// ("приватный"/"публичный" и производные), согласованную с (gender, number) -
// тот же принцип, что и newKeywordFor для конструктора.
func visibilityKeywordFor(private bool, gender morph.Gender, number morph.Number) string {
	stem := "публичн"
	if private {
		stem = "приватн"
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
func checkVisibilityAgreement(kw lexer.Token, private bool, fieldName string) error {
	gender, number, ok := morph.NounAgreement(fieldName)
	if !ok {
		return nil
	}
	want := visibilityKeywordFor(private, gender, number)
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
	want := visibilityKeywordFor(kw.Type == lexer.KwPrivate, morph.Feminine, morph.Singular)
	if kw.Value != want {
		return fmt.Errorf("строка %d, столбец %d: перед \"константа\" модификатор видимости согласуется со словом \"константа\" (женский род, единственное число) - ожидалось %q, получено %q",
			kw.Line, kw.Col, want, kw.Value)
	}
	return nil
}
