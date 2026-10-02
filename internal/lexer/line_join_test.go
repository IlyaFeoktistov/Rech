package lexer

import "testing"

// TestTokenizeJoinsLineStartingWithDotOrComma - перенос строки перед "." и
// "," не даёт NEWLINE (и пустые строки между тоже), перед остальным - даёт.
func TestTokenizeJoinsLineStartingWithDotOrComma(t *testing.T) {
	toks, err := New("сп\n    .добавить(1\n\n    , 2)\nх").Tokenize()
	if err != nil {
		t.Fatal(err)
	}
	var newlines int
	for _, tok := range toks {
		if tok.Type == NEWLINE {
			newlines++
		}
	}
	if newlines != 1 {
		t.Fatalf("ожидался один NEWLINE (перед \"х\"), получено %d: %+v", newlines, toks)
	}
}
