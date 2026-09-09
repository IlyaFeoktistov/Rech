package format

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// enforceTrailingCommas - PSR-12-подобный стиль (parser уже поддерживает
// эту форму грамматически - internal/parser/parser.go,
// skipNewlinesAfterListComma/skipNewlinesAfterAngleComma): у списка внутри
// "()"/"[]", РЕАЛЬНО разложенного по одному элементу на строку (не просто
// многострочного из-за вложенного колбэка - тот же признак "ownLevelBreak",
// что уже использует computeBracketWeights для отступа, format.go), висячая
// запятая после ПОСЛЕДНЕГО элемента - каноническая форма, если её ещё нет.
// Не трогает "<...>" (generic-параметры/аргументы типа) - "<"/">" в этом
// пакете сознательно не отслеживаются как скобки вовсе (см. package doc
// format.go, openers/closers - контекстная неоднозначность с операторами
// сравнения/сдвига).
func enforceTrailingCommas(src string) (string, error) {
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		return "", err
	}
	significant := make([]lexer.Token, 0, len(toks))
	for _, t := range toks {
		if t.Type == lexer.NEWLINE || t.Type == lexer.EOF {
			continue
		}
		significant = append(significant, t)
	}
	weight := computeBracketWeights(significant)
	match := matchAllBrackets(significant)

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	needsComma := map[int]bool{}

	for i, t := range significant {
		if t.Type != lexer.LPAREN && t.Type != lexer.LBRACKET {
			continue
		}
		if weight[i] != 1 {
			continue // не "свой элемент на строку" список (пустой/плоский/hanging-колбэк)
		}
		closeIdx := match[i]
		if closeIdx < 0 || closeIdx <= i+1 {
			continue // пустой список - разделять запятой нечего
		}
		lastIdx := closeIdx - 1
		if significant[lastIdx].Type == lexer.COMMA {
			continue // висячая запятая уже есть
		}
		if significant[lastIdx].Line == significant[closeIdx].Line {
			// Последний элемент и закрывающая скобка на одной строке -
			// добавлять запятую здесь означало бы поставить её ПОСЛЕ
			// закрывающей скобки на той же строке, либо нарушить
			// требование парсера "висячая запятая - только если
			// закрывающая скобка на СВОЕЙ строке" (skipNewlinesAfterListComma) -
			// безопаснее пропустить, чем сломать код.
			continue
		}
		needsComma[significant[lastIdx].Line] = true
	}
	if len(needsComma) == 0 {
		return src, nil
	}
	for line := range needsComma {
		if line < 1 || line > len(lines) {
			continue
		}
		lines[line-1] = strings.TrimRight(lines[line-1], " \t\r") + ","
	}
	return strings.Join(lines, "\n") + "\n", nil
}
