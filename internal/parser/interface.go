package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseInterfaceDecl разбирает "интерфейс Имя { ... }" - тело
// состоит только из сигнатур методов (имя, параметры, необязательный тип
// результата), без тел и без "имеет:" - интерфейс не хранит данные, только
// контракт на поведение.
func (p *Parser) parseInterfaceDecl() (*ast.InterfaceDecl, error) {
	kw := p.advance() // "интерфейс"
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, fmt.Errorf("после %q ожидалось имя интерфейса: %w", kw.Value, err)
	}
	p.registerNoun(name.Value)

	// "наследует А, Б" - методы каждого перечисленного интерфейса становятся
	// частью контракта этого интерфейса тоже (см. package doc
	// internal/semantic/interfaces.go, collectInterfaces) - через запятую
	// можно несколько (в отличие от "наследует" у сущности, где базовая
	// ровно одна).
	var extends []string
	if p.cur().Type == lexer.KwInherits {
		verb := p.advance() // "наследует"
		for {
			base, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, fmt.Errorf("интерфейс %q: после \"наследует\" ожидалось имя интерфейса: %w", name.Value, err)
			}
			baseName, err := p.resolveDeclinedTypeRef(base, verb.Value)
			if err != nil {
				return nil, fmt.Errorf("интерфейс %q: %w", name.Value, err)
			}
			extends = append(extends, baseName)
			if p.cur().Type != lexer.COMMA {
				break
			}
			p.advance() // ","
			p.skipNewlines()
		}
	}

	p.skipNewlines()
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	p.skipSeparators()

	var methods []ast.InterfaceMethod
	for p.cur().Type != lexer.RBRACE {
		if p.cur().Type == lexer.DOC_COMMENT {
			// Интерфейс пока не несёт doc-комментариев у своих сигнатур
			// (ast.InterfaceMethod без поля Doc) - "/** ... */" здесь ведёт
			// себя как обычный комментарий, просто пропускается.
			p.advance()
			p.skipSeparators()
			continue
		}
		m, err := p.parseInterfaceMethod()
		if err != nil {
			return nil, fmt.Errorf("интерфейс %q: %w", name.Value, err)
		}
		methods = append(methods, m)
		p.skipSeparators()
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	return &ast.InterfaceDecl{Name: name.Value, Extends: extends, Methods: methods, Line: kw.Line}, nil
}

// parseInterfaceMethod разбирает одну сигнатуру метода внутри "интерфейс" -
// как parseMethod (internal/parser/entity.go), но без модификатора
// видимости и без тела (сразу после ":ТипРезультата" или списка параметров
// идёт разделитель, а не "{").
func (p *Parser) parseInterfaceMethod() (ast.InterfaceMethod, error) {
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return ast.InterfaceMethod{}, err
	}
	if p.cur().Type != lexer.LPAREN {
		return ast.InterfaceMethod{}, fmt.Errorf("строка %d, столбец %d: после имени метода %q ожидалось \"(\" (даже без параметров - \"()\")",
			p.cur().Line, p.cur().Col, name.Value)
	}
	params, err := p.parseParams()
	if err != nil {
		return ast.InterfaceMethod{}, fmt.Errorf("в списке параметров метода %q: %w", name.Value, err)
	}
	returnType := ""
	selfReturn := false
	if p.cur().Type == lexer.COLON {
		p.advance()
		if p.cur().Type == lexer.KwThis {
			p.advance()
			selfReturn = true
		} else {
			returnType, err = p.parseTypeName()
			if err != nil {
				return ast.InterfaceMethod{}, fmt.Errorf("после \":\" у метода %q ожидалось имя типа результата: %w", name.Value, err)
			}
			returnType = normalizeReturnType(returnType)
		}
	}
	return ast.InterfaceMethod{Name: name.Value, Params: params, ReturnType: returnType, SelfReturn: selfReturn, Line: name.Line}, nil
}
