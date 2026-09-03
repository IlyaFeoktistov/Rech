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

func TestLexerCompoundAssignTokens(t *testing.T) {
	cases := map[string]TokenType{
		"+=": PLUS_ASSIGN,
		"-=": MINUS_ASSIGN,
		"*=": STAR_ASSIGN,
		"/=": SLASH_ASSIGN,
		"%=": PERCENT_ASSIGN,
		"?":  QUESTION,
		"??": QUESTIONQUESTION,
		"?=": NULL_ASSIGN,
	}
	for src, want := range cases {
		types := tokenTypes(t, src)
		if len(types) != 2 || types[0] != want { // + EOF
			t.Fatalf("%q: ожидался токен %v, получено %v", src, want, types)
		}
	}
}

func TestLexerPercentToken(t *testing.T) {
	types := tokenTypes(t, "а % б")
	if len(types) != 4 || types[1] != PERCENT {
		t.Fatalf("одиночный \"%%\" должен остаться PERCENT: %v", types)
	}
}

func TestLexerCompoundAssignDoesNotClashWithPlainOps(t *testing.T) {
	types := tokenTypes(t, "а + б")
	if len(types) != 4 || types[1] != PLUS {
		t.Fatalf("одиночный \"+\" должен остаться PLUS: %v", types)
	}
	types = tokenTypes(t, "а = б")
	if len(types) != 4 || types[1] != ASSIGN {
		t.Fatalf("одиночный \"=\" должен остаться ASSIGN: %v", types)
	}
}

func TestLexerNullKeyword(t *testing.T) {
	types := tokenTypes(t, "Ничто")
	if len(types) != 2 || types[0] != KwNull {
		t.Fatalf("\"Ничто\" должно быть ключевым словом KwNull: %v", types)
	}
}

func TestLexerCharLiteral(t *testing.T) {
	toks, err := New(`'a' 'я' '\n' '\''`).Tokenize()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := []string{"a", "я", "\n", "'"}
	if len(toks) != len(want)+1 {
		t.Fatalf("ожидалось %d токенов + EOF, получено %d: %v", len(want), len(toks), toks)
	}
	for i, w := range want {
		if toks[i].Type != CHAR || toks[i].Value != w {
			t.Fatalf("токен %d: ожидался CHAR %q, получено %s %q", i, w, toks[i].Type, toks[i].Value)
		}
	}
}

func TestLexerCarriageReturnEscape(t *testing.T) {
	toks, err := New(`"a\rb" '\r'`).Tokenize()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if toks[0].Type != STRING || toks[0].Value != "a\rb" {
		t.Fatalf("ожидался STRING %q, получено %s %q", "a\rb", toks[0].Type, toks[0].Value)
	}
	if toks[1].Type != CHAR || toks[1].Value != "\r" {
		t.Fatalf("ожидался CHAR %q, получено %s %q", "\r", toks[1].Type, toks[1].Value)
	}
}

func TestLexerCharLiteralRejectsEmptyAndMultiChar(t *testing.T) {
	if _, err := New(`''`).Tokenize(); err == nil {
		t.Fatal("ожидалась ошибка на пустом символьном литерале")
	}
	if _, err := New(`'ab'`).Tokenize(); err == nil {
		t.Fatal("ожидалась ошибка на символьном литерале из нескольких символов")
	}
}

func TestLexerTag(t *testing.T) {
	toks, err := New("`json:name, sql:user_name`").Tokenize()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(toks) != 2 || toks[0].Type != TAG {
		t.Fatalf("ожидался один токен TAG + EOF, получено %v", toks)
	}
	if want := "json:name, sql:user_name"; toks[0].Value != want {
		t.Fatalf("значение тега: ожидалось %q, получено %q", want, toks[0].Value)
	}
}

func TestLexerTagRejectsUnclosedAndNewline(t *testing.T) {
	if _, err := New("`json:name").Tokenize(); err == nil {
		t.Fatal("ожидалась ошибка на незакрытом теге")
	}
	if _, err := New("`json:name\n`").Tokenize(); err == nil {
		t.Fatal("ожидалась ошибка на переносе строки внутри тега")
	}
}
