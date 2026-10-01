package lsp

import "github.com/IlyaFeoktistov/Rech/internal/lexer"

// expandTemplateTokens - toks, где каждая шаблонная строка со вставками
// (TAG с "${...}") заменена токенами самих вставок с их позициями в
// исходнике, вложенные шаблоны - так же. Буквальный текст шаблона токенов
// не даёт. Позиционные возможности LSP (hover, definition, references,
// семантическая подсветка) ищут имя под курсором в этом потоке, поэтому
// видят имена и внутри "${...}". Вставка, которую не удалось разобрать
// лексером, пропускается - её ошибку показывает диагностика разбора.
func expandTemplateTokens(toks []lexer.Token) []lexer.Token {
	out := make([]lexer.Token, 0, len(toks))
	for _, tok := range toks {
		if tok.Type != lexer.TAG || !hasTemplateExpr(tok) {
			out = append(out, tok)
			continue
		}
		for _, part := range tok.Parts {
			if !part.Expr {
				continue
			}
			inner, err := lexer.TemplatePartTokens(part)
			if err != nil {
				continue
			}
			for _, t := range expandTemplateTokens(inner) {
				if t.Type != lexer.EOF && t.Type != lexer.NEWLINE {
					out = append(out, t)
				}
			}
		}
	}
	return out
}

func hasTemplateExpr(tok lexer.Token) bool {
	for _, part := range tok.Parts {
		if part.Expr {
			return true
		}
	}
	return false
}
