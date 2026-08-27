package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseGenericParams разбирает необязательный список параметров типа сразу
// после имени сущности/функции: "<" generic_param { "," generic_param } ">".
// nil (не ошибка), если текущий токен - не "<" (сущность/функция не
// generic). Позиция парсера на входе - сразу после имени.
//
//	generic_param := IDENT [ "как" type_name { "|" type_name } ]
func (p *Parser) parseGenericParams() ([]ast.GenericParam, error) {
	if p.cur().Type != lexer.LT {
		return nil, nil
	}
	p.advance() // "<"
	var params []ast.GenericParam
	seen := map[string]bool{}
	for {
		name, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, fmt.Errorf("в параметрах типа ожидалось имя параметра: %w", err)
		}
		if seen[name.Value] {
			return nil, fmt.Errorf("строка %d, столбец %d: параметр типа %q уже объявлен", name.Line, name.Col, name.Value)
		}
		seen[name.Value] = true
		param := ast.GenericParam{Name: name.Value, Line: name.Line}
		if p.cur().Type == lexer.KwAs {
			p.advance() // "как"
			for {
				c, err := p.parseTypeName()
				if err != nil {
					return nil, fmt.Errorf("в ограничении параметра типа %q: %w", name.Value, err)
				}
				param.Constraints = append(param.Constraints, c)
				if p.cur().Type != lexer.BITOR {
					break
				}
				p.advance() // "|"
			}
		}
		params = append(params, param)
		if p.cur().Type != lexer.COMMA {
			break
		}
		p.advance() // ","
	}
	if _, err := p.expect(lexer.GT); err != nil {
		return nil, fmt.Errorf("после параметров типа ожидалось \">\": %w", err)
	}
	return params, nil
}

// parseGenericCallExpr разбирает "имя<Тип, ...>(<арг>, ...)" - явный вызов
// generic-функции с аргументами типа (шаг 16, docs/SPEC.md - вывод типа
// из аргументов не реализован, только явная форма). Позиция парсера на
// входе - на IDENT имени функции (уже известного как generic -
// p.genericFuncs, см. вызывающий код).
func (p *Parser) parseGenericCallExpr() (*ast.CallExpr, error) {
	name := p.advance() // IDENT
	p.advance()          // "<"
	var typeArgs []string
	for {
		t, err := p.parseTypeName()
		if err != nil {
			return nil, fmt.Errorf("в аргументах типа вызова %q: %w", name.Value, err)
		}
		typeArgs = append(typeArgs, t)
		if p.cur().Type != lexer.COMMA {
			break
		}
		p.advance() // ","
	}
	if _, err := p.expect(lexer.GT); err != nil {
		return nil, fmt.Errorf("после аргументов типа вызова %q ожидалось \">\": %w", name.Value, err)
	}
	args, err := p.parseArgList()
	if err != nil {
		return nil, fmt.Errorf("в аргументах вызова %q: %w", name.Value, err)
	}
	return &ast.CallExpr{Callee: name.Value, Args: args, TypeArgs: typeArgs, Line: name.Line}, nil
}
