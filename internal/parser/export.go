package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseExportedDecl разбирает "экспортировать" и следующее за ним
// объявление - "функция" (тогда возвращается fn, ent == nil) либо
// "сущность"/"класс"/"структура" (тогда fn == nil, возвращается ent). Ровно
// один из результатов ненулевой - вызывающая сторона (ParseProgram)
// различает их так же, как обычные case lexer.KwFunction/KwEntity без
// "экспортировать". Допустимые типы параметров/результата экспортируемой
// функции (только примитивы, Строка и уже экспортированные сущности) -
// проверяются позже, internal/semantic: парсер проверяет только форму, не
// типы.
func (p *Parser) parseExportedDecl() (fn *ast.FunctionDecl, ent *ast.EntityDecl, err error) {
	kw := p.advance() // "экспортировать"
	switch p.cur().Type {
	case lexer.KwFunction:
		fn, err = p.parseFunction()
		if err != nil {
			return nil, nil, err
		}
		if fn.Name == entryPointName {
			return nil, nil, fmt.Errorf("строка %d: %q - точка входа программы, не может быть экспортирована", kw.Line, entryPointName)
		}
		fn.Exported = true
		return fn, nil, nil
	case lexer.KwEntity, lexer.KwClass, lexer.KwStruct:
		ent, err = p.parseEntity()
		if err != nil {
			return nil, nil, err
		}
		ent.Exported = true
		return nil, ent, nil
	default:
		return nil, nil, fmt.Errorf("строка %d: после %q ожидалось \"функция\", \"сущность\", \"класс\" или \"структура\", получено %q", p.cur().Line, kw.Value, p.cur().Value)
	}
}
