package format

import (
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// insertLambdaSpace вставляет один пробел между "функция" и "(" у
// анонимной лямбды ("функция(параметры)" -> "функция (параметры)") -
// именованное объявление ("функция имя(...)") этого не задевает вовсе (там
// между ключевым словом и "(" всегда есть имя) - пользователь попросил
// явно, визуально отличать колбэк-лямбду от обычного вызова "функция(...)"
// без разбора грамматики на глаз.
func insertLambdaSpace(src string) (string, error) {
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

	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	insertCols := map[int][]int{} // строка -> столбцы вставки (1-индексные), возможно несколько лямбд на одной строке
	for i, t := range significant {
		if t.Type != lexer.KwFunction || i+1 >= len(significant) {
			continue
		}
		next := significant[i+1]
		if next.Type != lexer.LPAREN || next.Line != t.Line {
			continue
		}
		if next.Col != t.Col+len([]rune(t.Value)) {
			continue // уже есть пробел (или другой разделитель) - не трогаем
		}
		insertCols[t.Line] = append(insertCols[t.Line], next.Col)
	}
	if len(insertCols) == 0 {
		return src, nil
	}
	for line, cols := range insertCols {
		if line < 1 || line > len(lines) {
			continue
		}
		// По убыванию столбца - более ранняя (левая) вставка не должна
		// сдвинуть позицию ещё не обработанной, более правой (несколько
		// лямбд на одной физической строке - редкость, но не должна
		// портить вторую вставку).
		sort.Sort(sort.Reverse(sort.IntSlice(cols)))
		runes := []rune(lines[line-1])
		for _, col := range cols {
			insertAt := col - 1
			if insertAt < 0 || insertAt > len(runes) {
				continue
			}
			runes = append(runes[:insertAt], append([]rune{' '}, runes[insertAt:]...)...)
		}
		lines[line-1] = string(runes)
	}
	return strings.Join(lines, "\n") + "\n", nil
}
