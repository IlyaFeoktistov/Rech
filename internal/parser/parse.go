package parser

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseParseExpr разбирает:
//
//	разобрать JSON <выражение> как <Тип>
//
// "JSON" - формат-маркер (lexer.KwJSON), не склоняется, единственный
// поддержанный формат в этом срезе. Источник - произвольное выражение
// (обычно Строка), не только литерал - тот же уровень приоритета, что и
// операнд каста (parseUnary, а не parseExpr - "как" должен связывать
// сильнее, чем, например, "+" слева от "разобрать").
func (p *Parser) parseParseExpr() (ast.Expr, error) {
	kw := p.advance() // "разобрать"
	format, err := p.expect(lexer.KwJSON)
	if err != nil {
		return nil, fmt.Errorf("после \"разобрать\" ожидался формат (JSON): %w", err)
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
//
// Обратное к parseParseExpr - без "как <Тип>" (целевой тип - тип самого
// значения, уже известен на этапе анализа).
func (p *Parser) parseSerializeExpr() (ast.Expr, error) {
	kw := p.advance() // "сериализовать"
	format, err := p.expect(lexer.KwJSON)
	if err != nil {
		return nil, fmt.Errorf("после \"сериализовать\" ожидался формат (JSON): %w", err)
	}
	value, err := p.parseUnary()
	if err != nil {
		return nil, fmt.Errorf("после \"сериализовать %s\" ожидалось выражение: %w", format.Value, err)
	}
	return &ast.SerializeExpr{Format: format.Value, Value: value, Line: kw.Line}, nil
}
