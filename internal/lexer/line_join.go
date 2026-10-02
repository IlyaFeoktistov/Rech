package lexer

// Перенос строки перед "." и "," незначим: строка, начинающаяся с точки
// (продолжение цепочки вызовов "список\n    .добавить(1)") или с запятой
// (запятая в начале строки в аргументах, параметрах, литералах), не бывает
// самостоятельной инструкцией - такая строка продолжает предыдущую. NEWLINE
// перед ней (вместе с пустыми строками между) убирается здесь, один раз для
// всех мест грамматики.

// joinLeadingDotComma - tokens без NEWLINE, за которыми (через другие
// NEWLINE) следует DOT или COMMA.
func joinLeadingDotComma(tokens []Token) []Token {
	out := tokens[:0]
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Type == NEWLINE {
			j := i
			for j < len(tokens) && tokens[j].Type == NEWLINE {
				j++
			}
			if j < len(tokens) && (tokens[j].Type == DOT || tokens[j].Type == COMMA) {
				i = j - 1
				continue
			}
		}
		out = append(out, tokens[i])
	}
	return out
}

// withoutNewlines - tokens без NEWLINE: внутри "${...}" перенос строки
// ничего не разделяет.
func withoutNewlines(tokens []Token) []Token {
	out := tokens[:0]
	for _, t := range tokens {
		if t.Type != NEWLINE {
			out = append(out, t)
		}
	}
	return out
}
