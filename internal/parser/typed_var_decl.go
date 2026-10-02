package parser

import (
	"strings"
	"unicode"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// tryParseTypedVarDecl - объявление переменной с явным типом: "<Тип> <имя>
// = <выражение>" (docs/SPEC.md, "Объявление переменной") - тип пишется
// первым, тем же порядком, что в C#/Java/C++ ("Список<Целое> список = [1, 2,
// 3]"); "переменная <имя> = ..." (parseVarDecl) - форма с выводом типа.
// Строит тот же ast.VarDecl, что и parseVarDecl, с непустым Type. Без инициализации
// ("Тип имя" и конец инструкции) - объявление без значения (Value == nil):
// значение присваивается позже, чтение до присваивания отвергает
// семантика. Эта форма требует, чтобы тип был похож на тип (с заглавной
// буквы, не имя объявленной функции) - иначе "IDENT IDENT" в начале
// строки - естественный вызов ("напечатать отчёт").
//
// Спекулятивный разбор с откатом позиции: parseTypeName сам по себе не
// может отличить "имя типа" от произвольного IDENT (типы не отмечены
// лексически, только по соглашению - заглавная буква) - поэтому пробуем
// разобрать тип и, если сразу после него не идёт "IDENT =", откатываемся и
// сообщаем "не эта форма" (ok == false, err == nil), а не саму ошибку -
// вызывающая сторона (parseStatement) продолжает обычную цепочку проверок
// (естественный вызов, присваивание и т. п.). Ложных срабатываний на
// существующем коде нет: до этой фичи "IDENT IDENT =" в начале инструкции
// был ГАРАНТИРОВАННО синтаксической ошибкой (два идентификатора без
// разделителя между инструкциями) - новая форма только придаёт смысл
// прежде невалидной последовательности токенов, ни одна существующая ветка
// разбора своего случая не теряет.
//
// "Вызываемый имя = значение" без "(...)" - сигнатура берётся из значения
// (лямбды/ссылки на функцию); Type остаётся голым "Вызываемый", его
// разбирает семантика (bareCallableType).
func (p *Parser) tryParseTypedVarDecl() (ast.Stmt, bool, error) {
	saved := p.pos
	if p.cur().Type == lexer.IDENT && p.cur().Value == callableTypeName &&
		p.peekAt(1).Type == lexer.IDENT && p.peekAt(2).Type == lexer.ASSIGN {
		p.advance() // "Вызываемый"
		return p.finishTypedVarDecl(callableTypeName)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		p.pos = saved
		return nil, false, nil
	}
	if p.cur().Type == lexer.IDENT && isStatementEnd(p.peekAt(1).Type) && p.looksLikeTypeName(typeName) {
		name := p.advance()
		p.registerNoun(name.Value)
		p.registerVarEntityType(name.Value, typeName)
		return &ast.VarDecl{Name: name.Value, Type: typeName, Line: name.Line}, true, nil
	}
	if p.cur().Type != lexer.IDENT || p.peekAt(1).Type != lexer.ASSIGN {
		p.pos = saved
		return nil, false, nil
	}
	return p.finishTypedVarDecl(typeName)
}

// isStatementEnd - токен, на котором кончается инструкция.
func isStatementEnd(t lexer.TokenType) bool {
	return t == lexer.NEWLINE || t == lexer.SEMICOLON || t == lexer.RBRACE || t == lexer.EOF
}

// looksLikeTypeName - typeName может быть типом в "Тип имя" без значения:
// с заглавной буквы (соглашение языка для типов; функции - snake_case) и
// не имя объявленной функции.
func (p *Parser) looksLikeTypeName(typeName string) bool {
	if _, isFunc := p.funcParams[typeName]; isFunc {
		return false
	}
	for _, r := range strings.TrimPrefix(typeName, "?") {
		return unicode.IsUpper(r)
	}
	return false
}

// finishTypedVarDecl - "<имя> = <выражение>" после уже разобранного типа.
func (p *Parser) finishTypedVarDecl(typeName string) (ast.Stmt, bool, error) {
	name := p.advance()
	p.advance() // "="
	value, err := p.parseExpr()
	if err != nil {
		return nil, true, diagnostics.Wrap(err, "в инициализации переменной %q", name.Value)
	}
	p.registerNoun(name.Value)
	p.registerVarEntityType(name.Value, typeName)
	return &ast.VarDecl{Name: name.Value, Type: typeName, Value: value, Line: name.Line}, true, nil
}
