package lexer

import "testing"

func tokenTypes(t *testing.T, src string) []TokenType {
	t.Helper()
	toks, err := New(src).Tokenize()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	var types []TokenType
	for _, tok := range toks {
		if tok.Type == NEWLINE {
			continue
		}
		types = append(types, tok.Type)
	}
	return types
}

func TestLexerBitwiseAndShiftTokens(t *testing.T) {
	cases := map[string]TokenType{
		"&":  BITAND,
		"|":  BITOR,
		"^":  BITXOR,
		"~":  BITNOT,
		"<<": SHL,
		">>": SHR,
		"++": INCR,
		"--": DECR,
	}
	for src, want := range cases {
		types := tokenTypes(t, src)
		if len(types) != 2 || types[0] != want { // + EOF
			t.Fatalf("%q: ожидался токен %v, получено %v", src, want, types)
		}
	}
}

func TestLexerAndOrSymbolSynonyms(t *testing.T) {
	types := tokenTypes(t, "а && б")
	if len(types) != 4 || types[1] != KwAnd { // IDENT && IDENT EOF
		t.Fatalf("\"&&\" должно давать тот же токен, что и \"и\": %v", types)
	}
	types = tokenTypes(t, "а || б")
	if len(types) != 4 || types[1] != KwOr {
		t.Fatalf("\"||\" должно давать тот же токен, что и \"или\": %v", types)
	}
}

func TestLexerShiftDoesNotClashWithComparison(t *testing.T) {
	types := tokenTypes(t, "а < б")
	if len(types) != 4 || types[1] != LT {
		t.Fatalf("одиночный \"<\" должен остаться LT: %v", types)
	}
	types = tokenTypes(t, "а <= б")
	if len(types) != 4 || types[1] != LE {
		t.Fatalf("\"<=\" должен остаться LE: %v", types)
	}
}

func TestLexerIncrDoesNotClashWithPlus(t *testing.T) {
	types := tokenTypes(t, "а + б")
	if len(types) != 4 || types[1] != PLUS {
		t.Fatalf("одиночный \"+\" должен остаться PLUS: %v", types)
	}
}
