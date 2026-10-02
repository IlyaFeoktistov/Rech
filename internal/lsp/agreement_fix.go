package lsp

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// Быстрое исправление рассогласованного модификатора: видимость/"статический"
// перед "Тип имя" (поле, переменная модуля) и "новый" перед типом
// согласуются с типом (parser.ModifierFormFor - те же правила, что
// проверяет компилятор). Ищется по токенам, а не по диагностике: парсер
// сообщает только первое рассогласование, а исправить можно все сразу.

// agreementFix - модификатор tok, вместо которого нужна форма want.
type agreementFix struct {
	tok  lexer.Token
	want string
}

// agreementFixes - все рассогласованные модификаторы документа по порядку.
func (d *doc) agreementFixes() []agreementFix {
	toks := d.tokens
	var out []agreementFix
	check := func(mod lexer.Token, typeName string) {
		if want, ok := parser.ModifierFormFor(mod, typeName); ok && want != mod.Value {
			out = append(out, agreementFix{tok: mod, want: want})
		}
	}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Type == lexer.KwNew {
			if i+1 < len(toks) && toks[i+1].Type == lexer.IDENT {
				check(t, toks[i+1].Value)
			}
			continue
		}
		if !isAgreeingModifier(t.Type) {
			continue
		}
		j := i
		for j < len(toks) && isAgreeingModifier(toks[j].Type) {
			j++
		}
		if typeName, ok := typeBeforeName(toks, j); ok {
			for _, mod := range toks[i:j] {
				check(mod, typeName)
			}
		}
		i = j - 1
	}
	return out
}

func isAgreeingModifier(t lexer.TokenType) bool {
	switch t {
	case lexer.KwStatic, lexer.KwPrivate, lexer.KwPublic, lexer.KwProtected, lexer.KwInternal:
		return true
	}
	return false
}

// typeBeforeName - с позиции i записано "Тип имя" (объявление с типом
// первым): возвращает внешнее имя типа. Метод ("имя(") и объявления с
// ключевым словом ("функция", "сущность", ...) - не оно.
func typeBeforeName(toks []lexer.Token, i int) (string, bool) {
	for i < len(toks) && (toks[i].Type == lexer.QUESTION || toks[i].Type == lexer.STAR) {
		i++
	}
	if i >= len(toks) || toks[i].Type != lexer.IDENT {
		return "", false
	}
	typeName := toks[i].Value
	i++
	if i < len(toks) && toks[i].Type == lexer.LT {
		depth := 0
		for ; i < len(toks); i++ {
			switch toks[i].Type {
			case lexer.LT:
				depth++
			case lexer.GT:
				depth--
			case lexer.SHR:
				depth -= 2
			}
			if depth <= 0 {
				i++
				break
			}
		}
	}
	if i < len(toks) && toks[i].Type == lexer.LPAREN {
		// "Вызываемый(...): Тип имя" - сигнатура до ")" и тип результата.
		depth := 0
		for ; i < len(toks); i++ {
			if toks[i].Type == lexer.LPAREN {
				depth++
			} else if toks[i].Type == lexer.RPAREN {
				depth--
				if depth == 0 {
					i++
					break
				}
			}
		}
		if i+1 < len(toks) && toks[i].Type == lexer.COLON && toks[i+1].Type == lexer.IDENT {
			i += 2
		}
	}
	if i < len(toks) && toks[i].Type == lexer.IDENT {
		return typeName, true
	}
	return "", false
}

// agreementCodeActions - quick fix'ы согласования для строк [reqStart,
// reqEnd] (1-индексные) и, если рассогласований в файле больше одного,
// одно действие "исправить все".
func (d *doc) agreementCodeActions(uri string, reqStart, reqEnd int) []any {
	fixes := d.agreementFixes()
	var out []any
	for _, f := range fixes {
		if f.tok.Line < reqStart || f.tok.Line > reqEnd {
			continue
		}
		out = append(out, quickFixJSON(uri, fmt.Sprintf("Согласовать: %q", f.want),
			f.tok.Line, f.tok.Col-1, f.tok.Line, f.tok.Col-1+len([]rune(f.tok.Value)), f.want))
	}
	if len(fixes) > 1 && len(out) > 0 {
		edits := make([]any, len(fixes))
		for i, f := range fixes {
			edits[i] = textEditJSON(f.tok.Line, f.tok.Col-1, f.tok.Line, f.tok.Col-1+len([]rune(f.tok.Value)), f.want)
		}
		out = append(out, map[string]any{
			"title": fmt.Sprintf("Согласовать все модификаторы в файле (%d)", len(fixes)),
			"kind":  "quickfix",
			"edit":  map[string]any{"changes": map[string]any{uri: edits}},
		})
	}
	return out
}
