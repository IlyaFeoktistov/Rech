package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// expectFormat разбирает формат-маркер после "разобрать"/"сериализовать" -
// "JSON" (lexer.KwJSON) или "XML" (lexer.KwXML), оба не склоняются.
func (p *Parser) expectFormat(after string) (lexer.Token, error) {
	if p.cur().Type != lexer.KwJSON && p.cur().Type != lexer.KwXML {
		tok := p.cur()
		return lexer.Token{}, fmt.Errorf("строка %d, столбец %d: после %q ожидался формат (JSON или XML), получено %q", tok.Line, tok.Col, after, tok.Value)
	}
	return p.advance(), nil
}

// parseParseExpr разбирает:
//
//	разобрать JSON <выражение> как <Тип>
//	разобрать XML <выражение> как <Тип>
//
// Источник - произвольное выражение (обычно Строка), не только литерал -
// тот же уровень приоритета, что и операнд каста (parseUnary, а не
// parseExpr - "как" должен связывать сильнее, чем, например, "+" слева от
// "разобрать").
func (p *Parser) parseParseExpr() (ast.Expr, error) {
	kw := p.advance() // "разобрать"
	format, err := p.expectFormat("разобрать")
	if err != nil {
		return nil, err
	}
	source, err := p.parseUnary()
	if err != nil {
		return nil, fmt.Errorf("после \"разобрать %s\" ожидалось выражение-источник: %w", format.Value, err)
	}
	if _, err := p.expect(lexer.KwAs); err != nil {
		return nil, fmt.Errorf("после источника \"разобрать %s\" ожидалось \"как\": %w", format.Value, err)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return nil, fmt.Errorf("после \"как\" ожидался целевой тип: %w", err)
	}
	return &ast.ParseExpr{Format: format.Value, Source: source, TypeName: typeName, Line: kw.Line}, nil
}

// parseSerializeExpr разбирает:
//
//	сериализовать JSON <выражение>
//	сериализовать XML <выражение>
//
// Обратное к parseParseExpr - без "как <Тип>" (целевой тип - тип самого
// значения, уже известен на этапе анализа).
func (p *Parser) parseSerializeExpr() (ast.Expr, error) {
	kw := p.advance() // "сериализовать"
	format, err := p.expectFormat("сериализовать")
	if err != nil {
		return nil, err
	}
	value, err := p.parseUnary()
	if err != nil {
		return nil, fmt.Errorf("после \"сериализовать %s\" ожидалось выражение: %w", format.Value, err)
	}
	return &ast.SerializeExpr{Format: format.Value, Value: value, Line: kw.Line}, nil
}
