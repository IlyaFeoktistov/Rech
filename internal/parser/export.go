package parser

import (
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
		fn, err = p.parseFunction(false)
		if err != nil {
			return nil, nil, err
		}
		if fn.Name == entryPointName {
			return nil, nil, errAtLine(kw.Line, CodeEntryPoint, "%q - точка входа программы, не может быть экспортирована", entryPointName)
		}
		fn.Exported = true
		return fn, nil, nil
	case lexer.KwAbstract, lexer.KwEntity, lexer.KwClass, lexer.KwStruct:
		ent, err = p.parseEntity()
		if err != nil {
			return nil, nil, err
		}
		ent.Exported = true
		return nil, ent, nil
	default:
		return nil, nil, errAtLine(p.cur().Line, CodeUnexpectedToken, "после %q ожидалось \"функция\", \"сущность\", \"класс\" или \"структура\", получено %q", kw.Value, p.cur().Value)
	}
}
