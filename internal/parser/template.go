package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// parseTemplateLiteral превращает TAG-токен в позиции выражения (шаблонный
// литерал `текст ${выражение} текст`) в дерево уже существующих узлов AST -
// StringLiteral для буквальных кусков, CastExpr{Type: "Строка"} вокруг
// каждой вставки (приведение результата к строке - тот же смысл, что и
// "текст + (Строка) значение"), всё вместе свёрнуто в левоассоциативную
// цепочку BinaryExpr{Op: "+"}. Ни семантика, ни бэкенды не знают о
// шаблонных литералах отдельно - для них это обычная конкатенация строк.
func (p *Parser) parseTemplateLiteral(tok lexer.Token) (ast.Expr, error) {
	if len(tok.Parts) == 0 {
		return &ast.StringLiteral{Value: "", Line: tok.Line}, nil
	}
	var result ast.Expr
	for _, part := range tok.Parts {
		var piece ast.Expr
		if part.Expr {
			expr, err := p.parseTemplateExprPart(part)
			if err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{Line: part.Line, Col: part.Col}, "во вставке \"${%s}\" шаблонного литерала", part.Text)
			}
			piece = &ast.CastExpr{Type: "Строка", Value: expr, Line: part.Line, Template: true}
		} else {
			piece = &ast.StringLiteral{Value: part.Text, Line: part.Line}
		}
		if result == nil {
			result = piece
			continue
		}
		result = &ast.BinaryExpr{Op: "+", Left: result, Right: piece, Line: tok.Line}
	}
	return result, nil
}

// parseTemplateExprPart разбирает исходный текст одной "${...}"-вставки
// отдельным вложенным лексером/парсером - вставка вправе содержать любое
// выражение языка, включая свои строковые/символьные литералы и вложенные
// строки в обратных кавычках. Позиции токенов - в исходнике всего файла
// (lexer.TemplatePartTokens), иначе узлы вставки и ошибки по ним
// отсчитывались бы от строки 1. Вложенный парсер - копия p с другими
// токенами: контекст объемлющего кода ("этот" в теле метода, словарь
// существительных, сигнатуры функций для естественного вызова) тот же.
func (p *Parser) parseTemplateExprPart(part lexer.TemplatePart) (ast.Expr, error) {
	toks, err := lexer.TemplatePartTokens(part)
	if err != nil {
		return nil, err
	}
	sub := *p
	sub.tokens, sub.pos = toks, 0
	expr, err := sub.parseExpr()
	if err != nil {
		return nil, err
	}
	if sub.cur().Type != lexer.EOF {
		return nil, errAt(sub.cur().Line, sub.cur().Col, CodeTemplate, "лишние символы после выражения")
	}
	return expr, nil
}
