package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// tryParseTypedVarDecl - альтернативная форма объявления переменной без
// слова "переменная": "<Тип> <имя> = <выражение>" (docs/SPEC.md, "Объявление
// переменной") - тип пишется явно первым, тем же порядком, что в C#/Java/C++
// ("Список<Целое> список = [1, 2, 3]"), вместо "переменная список:
// Список<Целое> = [1, 2, 3]". Строит тот же самый ast.VarDecl, что и
// parseVarDecl (с непустым Type) - семантика/backend'ы не отличают одну
// форму от другой, разница только на уровне парсера. Без инициализации
// ("Тип имя" без "=") эта форма не поддержана вовсе - в отличие от
// "переменная X" (см. pendingvar.go), у неё нет отдельного сценария
// отложенного объявления, добавлять его ради формы, которую и так почти
// всегда пишут вместе со значением, не стоило.
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
func (p *Parser) tryParseTypedVarDecl() (ast.Stmt, bool, error) {
	saved := p.pos
	typeName, err := p.parseTypeName()
	if err != nil {
		p.pos = saved
		return nil, false, nil
	}
	if p.cur().Type != lexer.IDENT || p.peekAt(1).Type != lexer.ASSIGN {
		p.pos = saved
		return nil, false, nil
	}
	name := p.advance()
	p.advance() // "="
	value, err := p.parseExpr()
	if err != nil {
		return nil, true, fmt.Errorf("в инициализации переменной %q: %w", name.Value, err)
	}
	p.registerNoun(name.Value)
	p.registerVarEntityType(name.Value, typeName)
	return &ast.VarDecl{Name: name.Value, Type: typeName, Value: value, Line: name.Line}, true, nil
}
