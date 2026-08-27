package parser

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseLambdaExpr разбирает "функция(<парам>: <Тип>, ...): <ТипРезультата>
// { <тело> }" без имени, в позиции выражения - та же грамматика параметров/
// тела, что и у именованного parseFunction, кроме имени и параметров типа
// (лямбда не может быть generic). Позиция парсера на входе - на "функция".
func (p *Parser) parseLambdaExpr() (*ast.LambdaExpr, error) {
	kw, err := p.expect(lexer.KwFunction)
	if err != nil {
		return nil, err
	}

	var params []ast.Param
	if p.cur().Type == lexer.LPAREN {
		params, err = p.parseParams()
		if err != nil {
			return nil, fmt.Errorf("в списке параметров лямбды: %w", err)
		}
	}

	returnType := ""
	if p.cur().Type == lexer.COLON {
		p.advance()
		returnType, err = p.parseTypeName()
		if err != nil {
			return nil, fmt.Errorf("после \":\" у лямбды ожидалось имя типа результата: %w", err)
		}
	}

	p.skipNewlines()
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.LambdaExpr{Params: params, ReturnType: returnType, Body: body, Line: kw.Line}, nil
}

// callableTypeName - зарезервированное имя типа "функция как значение".
const callableTypeName = "Вызываемый"

// parseCallableTypeName разбирает тип "функция как значение" после уже
// считанного имени "Вызываемый":
//
//	"<" [ type_name { "," type_name } ] ">" [ type_name ]
//
// Отдельная форма от общего generic-механизма (parseTypeName) - в отличие
// от Список<T>/Коробка<T>, список параметров может быть пустым (нулевая
// арность), и сразу за закрывающей "<...>" без разделителя может идти ещё
// один тип - результат (пусто - без результата). Позиция парсера на входе -
// сразу после IDENT "Вызываемый".
func (p *Parser) parseCallableTypeName(name lexer.Token) (string, error) {
	if p.cur().Type != lexer.LT {
		return "", fmt.Errorf("строка %d, столбец %d: после %q ожидался список параметров \"<...>\"", p.cur().Line, p.cur().Col, name.Value)
	}
	p.advance() // "<"
	var params []string
	if p.cur().Type != lexer.GT {
		for {
			t, err := p.parseTypeName()
			if err != nil {
				return "", fmt.Errorf("в параметрах типа %q: %w", name.Value, err)
			}
			params = append(params, t)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
		}
	}
	if err := p.expectCloseAngle(); err != nil {
		return "", fmt.Errorf("после параметров типа %q: %w", name.Value, err)
	}
	result := ""
	if p.cur().Type == lexer.IDENT {
		r, err := p.parseTypeName()
		if err != nil {
			return "", fmt.Errorf("в типе результата %q: %w", name.Value, err)
		}
		result = r
	}
	return fmt.Sprintf("%s<%s>%s", name.Value, strings.Join(params, ", "), result), nil
}
