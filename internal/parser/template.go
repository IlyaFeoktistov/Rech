package parser

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// templatePart - один кусок разобранного шаблонного литерала: либо кусок
// буквального текста (isExpr == false, text уже содержит все раскрытые
// экранирования - lexer.readTag сделал это до нас), либо исходный текст
// одной "${...}"-вставки без окружающих "${"/"}" (isExpr == true, text
// разбирается отдельным вложенным лексером/парсером в parseTemplateLiteral).
type templatePart struct {
	text   string
	isExpr bool
}

// parseTemplateLiteral превращает TAG-токен в позиции выражения (шаблонный
// литерал `текст ${выражение} текст`) в дерево уже существующих узлов AST -
// StringLiteral для буквальных кусков, CastExpr{Type: "Строка"} вокруг
// каждой вставки (приведение результата к строке - тот же смысл, что и
// "текст + (Строка) значение"), всё вместе свёрнуто в левоассоциативную
// цепочку BinaryExpr{Op: "+"}. Ни семантика, ни бэкенды не знают о
// шаблонных литералах отдельно - для них это обычная конкатенация строк.
func (p *Parser) parseTemplateLiteral(tok lexer.Token) (ast.Expr, error) {
	parts, err := splitTemplateParts(tok.Value)
	if err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{Line: tok.Line, Col: tok.Col})
	}
	if len(parts) == 0 {
		return &ast.StringLiteral{Value: "", Line: tok.Line}, nil
	}
	var result ast.Expr
	for _, part := range parts {
		var piece ast.Expr
		if part.isExpr {
			expr, err := parseTemplateExprPart(part.text, tok.Line)
			if err != nil {
				return nil, diagnostics.WrapAt(err, diagnostics.Position{Line: tok.Line, Col: tok.Col}, "во вставке \"${%s}\" шаблонного литерала", part.text)
			}
			piece = &ast.CastExpr{Type: "Строка", Value: expr, Line: tok.Line}
		} else {
			piece = &ast.StringLiteral{Value: part.text, Line: tok.Line}
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
// выражение языка, включая свои собственные строковые/символьные литералы
// и вложенные шаблонные литералы. Строки токенов сдвигаются на line -
// строку самого литерала, иначе позиции узлов вставки (и ошибок семантики
// по ним) отсчитывались бы от 1.
func parseTemplateExprPart(src string, line int) (ast.Expr, error) {
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return nil, err
	}
	for i := range toks {
		toks[i].Line += line - 1
		if toks[i].EndLine != 0 {
			toks[i].EndLine += line - 1
		}
	}
	sub := New(toks)
	expr, err := sub.parseExpr()
	if err != nil {
		return nil, err
	}
	if sub.cur().Type != lexer.EOF {
		return nil, errAt(sub.cur().Line, sub.cur().Col, CodeTemplate, "лишние символы после выражения")
	}
	return expr, nil
}

// splitTemplateParts разбивает уже раскрытый (readTag снял экранирования)
// текст шаблонного литерала на чередующиеся куски буквального текста и
// вставок. Внутри вставки отслеживается глубина фигурных скобок (сама
// вставка вправе содержать вложенные "{...}", например литерал сущности)
// и пропускаются строковые/символьные литералы ("..."/'...'), чтобы "}"
// или кавычка внутри них не спутали границу вставки.
func splitTemplateParts(raw string) ([]templatePart, error) {
	var parts []templatePart
	var textBuf []rune
	runes := []rune(raw)
	i := 0
	flushText := func() {
		if len(textBuf) > 0 {
			parts = append(parts, templatePart{text: string(textBuf)})
			textBuf = nil
		}
	}
	for i < len(runes) {
		if runes[i] == '$' && i+1 < len(runes) && runes[i+1] == '{' {
			flushText()
			start := i + 2
			depth := 1
			j := start
			for j < len(runes) {
				switch runes[j] {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						goto found
					}
				case '"', '\'':
					quote := runes[j]
					j++
					for j < len(runes) && runes[j] != quote {
						if runes[j] == '\\' {
							j++
						}
						j++
					}
				}
				j++
			}
			return nil, diagnostics.New(CodeTemplate, diagnostics.Position{}, "вставка \"${\" не закрыта символом \"}\"")
		found:
			parts = append(parts, templatePart{text: string(runes[start:j]), isExpr: true})
			i = j + 1
			continue
		}
		textBuf = append(textBuf, runes[i])
		i++
	}
	flushText()
	return parts, nil
}
