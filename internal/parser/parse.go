package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// expectFormat разбирает формат-маркер после "разобрать"/"сериализовать" -
// "JSON" (lexer.KwJSON), "XML" (lexer.KwXML), "YAML" (lexer.KwYAML) или
// "PROTOBUF" (lexer.KwPROTOBUF) - ни один не склоняется.
func (p *Parser) expectFormat(after string) (lexer.Token, error) {
	switch p.cur().Type {
	case lexer.KwJSON, lexer.KwXML, lexer.KwYAML, lexer.KwPROTOBUF:
		return p.advance(), nil
	}
	tok := p.cur()
	return lexer.Token{}, errAt(tok.Line, tok.Col, CodeUnexpectedToken, "после %q ожидался формат (JSON, XML, YAML или PROTOBUF), получено %q", after, tok.Value)
}

// parseParseExpr разбирает:
//
//	разобрать JSON <выражение> как <Тип>
//	разобрать XML <выражение> как <Тип>
//	разобрать YAML <выражение> как <Тип>
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
		return nil, diagnostics.Wrap(err, "после \"разобрать %s\" ожидалось выражение-источник", format.Value)
	}
	if _, err := p.expect(lexer.KwAs); err != nil {
		return nil, diagnostics.Wrap(err, "после источника \"разобрать %s\" ожидалось \"как\"", format.Value)
	}
	typeName, err := p.parseTypeName()
	if err != nil {
		return nil, diagnostics.Wrap(err, "после \"как\" ожидался целевой тип")
	}
	return &ast.ParseExpr{Format: format.Value, Source: source, TypeName: typeName, Line: kw.Line}, nil
}

// parseSerializeExpr разбирает:
//
//	сериализовать JSON <выражение>
//	сериализовать XML <выражение>
//	сериализовать YAML <выражение>
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
		return nil, diagnostics.Wrap(err, "после \"сериализовать %s\" ожидалось выражение", format.Value)
	}
	return &ast.SerializeExpr{Format: format.Value, Value: value, Line: kw.Line}, nil
}
