package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
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
	p.skipNewlines()
	var params []ast.GenericParam
	seen := map[string]bool{}
	for {
		name, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, diagnostics.Wrap(err, "в параметрах типа ожидалось имя параметра")
		}
		if seen[name.Value] {
			return nil, errAt(name.Line, name.Col, CodeDuplicateDecl, "параметр типа %q уже объявлен", name.Value)
		}
		seen[name.Value] = true
		param := ast.GenericParam{Name: name.Value, Line: name.Line}
		if p.cur().Type == lexer.KwAs {
			p.advance() // "как"
			for {
				c, err := p.parseTypeName()
				if err != nil {
					return nil, diagnostics.Wrap(err, "в ограничении параметра типа %q", name.Value)
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
		atClose, err := p.skipNewlinesAfterAngleComma()
		if err != nil {
			return nil, err
		}
		if atClose {
			break // висячая запятая перед ">"
		}
	}
	p.skipNewlines()
	if err := p.expectCloseAngle(); err != nil {
		return nil, diagnostics.Wrap(err, "после параметров типа")
	}
	return params, nil
}

// parseGenericCallExpr разбирает "имя<Тип, ...>(<арг>, ...)" - явный вызов
// generic-функции с аргументами типа (вывод типа из аргументов не
// реализован, только явная форма). Позиция парсера на
// входе - на IDENT имени функции (уже известного как generic -
// p.genericFuncs, см. вызывающий код).
func (p *Parser) parseGenericCallExpr() (*ast.CallExpr, error) {
	name := p.advance() // IDENT
	p.advance()         // "<"
	p.skipNewlines()
	var typeArgs []string
	for {
		t, err := p.parseTypeName()
		if err != nil {
			return nil, diagnostics.Wrap(err, "в аргументах типа вызова %q", name.Value)
		}
		typeArgs = append(typeArgs, t)
		if p.cur().Type != lexer.COMMA {
			break
		}
		p.advance() // ","
		atClose, err := p.skipNewlinesAfterAngleComma()
		if err != nil {
			return nil, err
		}
		if atClose {
			break // висячая запятая перед ">"
		}
	}
	p.skipNewlines()
	if err := p.expectCloseAngle(); err != nil {
		return nil, diagnostics.Wrap(err, "после аргументов типа вызова %q", name.Value)
	}
	args, err := p.parseArgList()
	if err != nil {
		return nil, diagnostics.Wrap(err, "в аргументах вызова %q", name.Value)
	}
	return &ast.CallExpr{Callee: name.Value, Args: args, TypeArgs: typeArgs, Line: name.Line}, nil
}

// looksLikeGenericCall - с текущего IDENT начинается "имя<Тип, ...>(":
// явные аргументы типа у функции, о которой парсер не знает, что она
// generic (импортирована из другого файла - парсер однофайловый). Как
// сравнение это прочитать нельзя: сравнения не цепочные, "а < б > (в)" -
// не выражение. Внутри "<...>" - только то, из чего пишутся имена типов.
func (p *Parser) looksLikeGenericCall() bool {
	if p.cur().Type != lexer.IDENT || p.peekAt(1).Type != lexer.LT {
		return false
	}
	depth := 0
	for i := 1; i < 64; i++ {
		switch p.peekAt(i).Type {
		case lexer.LT:
			depth++
		case lexer.GT:
			depth--
		case lexer.SHR:
			depth -= 2
		case lexer.IDENT, lexer.COMMA, lexer.QUESTION, lexer.STAR, lexer.LBRACKET, lexer.RBRACKET, lexer.NEWLINE:
		default:
			return false
		}
		if depth < 0 {
			return false
		}
		if depth == 0 {
			return p.peekAt(i+1).Type == lexer.LPAREN
		}
	}
	return false
}
