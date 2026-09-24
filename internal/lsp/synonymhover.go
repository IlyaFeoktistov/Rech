package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// hoverSynonym - объявление синонима name ровно так, как оно написано в
// исходнике ("приватный синоним Группы = Словарь<Строка, Числа>"), плюс
// doc-комментарий. Текст восстанавливается из токенов файла объявления, а
// не из SynonymDecl.Target: там цель уже раскрыта и в канонической форме
// ("Вызываемый<Целое>Строка").
func (d *doc) hoverSynonym(name string) (string, bool) {
	syn, ok := d.astSynonyms[name]
	if !ok {
		return "", false
	}
	toks := d.tokens
	if !d.isCurrentFile(syn.File) {
		foreign, ok := d.foreignTokens(syn.File)
		if !ok {
			return "", false
		}
		toks = foreign
	}
	text, ok := synonymDeclText(toks, syn.Line)
	if !ok {
		return "", false
	}
	return withDoc(codeBlock(text), syn.Doc), true
}

// synonymDeclText собирает текст объявления синонима, начинающегося
// ключевым словом "синоним" на строке line: модификатор видимости перед
// ним (если есть) и всё до конца типа - перевода строки вне скобок, не
// сразу после "=" или ",". Пробелы между токенами - как в исходнике
// (Token.SpaceBefore), перенос строки внутри объявления - один пробел.
func synonymDeclText(toks []lexer.Token, line int) (string, bool) {
	start := -1
	for i, t := range toks {
		if t.Type == lexer.KwSynonym && t.Line == line {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}
	if start > 0 {
		if prev := toks[start-1]; prev.Line == line && (prev.Type == lexer.KwPrivate || prev.Type == lexer.KwPublic || prev.Type == lexer.KwInternal) {
			start--
		}
	}
	var b strings.Builder
	depth := 0
	pendingSpace := false
	for i := start; i < len(toks); i++ {
		t := toks[i]
		if t.Type == lexer.EOF {
			break
		}
		if t.Type == lexer.NEWLINE {
			prev := toks[i-1].Type
			if depth == 0 && prev != lexer.ASSIGN && prev != lexer.COMMA {
				break
			}
			pendingSpace = true
			continue
		}
		switch t.Type {
		case lexer.LT, lexer.LPAREN:
			depth++
		case lexer.GT, lexer.RPAREN:
			depth--
		case lexer.SHR:
			depth -= 2
		}
		if b.Len() > 0 && (t.SpaceBefore || pendingSpace) {
			b.WriteByte(' ')
		}
		pendingSpace = false
		b.WriteString(t.Value)
	}
	return b.String(), true
}
