package lexer

import "testing"

// TestLexerFloatExponentLiterals - "1e10"/"2.5e-3"/"1.5E+10" - экспонента
// всегда делает литерал FLOAT, даже без десятичной точки в мантиссе (docs/
// "exp/e"). Value токена - буквальный текст исходника
// (strconv.ParseFloat на стороне парсера сам справляется с "e"/"E").
func TestLexerFloatExponentLiterals(t *testing.T) {
	cases := map[string]string{
		"1e10":    "1e10",
		"1E10":    "1E10",
		"2.5e-3":  "2.5e-3",
		"1.5E+10": "1.5E+10",
		"6e0":     "6e0",
	}
	for src, wantValue := range cases {
		toks, err := New(src).Tokenize()
		if err != nil {
			t.Fatalf("%q: неожиданная ошибка: %v", src, err)
		}
		if len(toks) < 1 || toks[0].Type != FLOAT {
			t.Fatalf("%q: ожидался токен FLOAT, получено %v", src, toks)
		}
		if toks[0].Value != wantValue {
			t.Fatalf("%q: Value = %q, хотели %q", src, toks[0].Value, wantValue)
		}
	}
}

// TestLexerBareExponentLetterWithoutDigitsIsNotExponent - "e"/"E" сразу за
// числом БЕЗ хотя бы одной цифры после (пропустив необязательный знак) - не
// экспонента, число заканчивается раньше, "e"/"E" (и необязательный знак)
// остаются отдельными токенами дальше - тот же принцип, что и одинокая "."
// без цифры после неё не делает число дробным.
func TestLexerBareExponentLetterWithoutDigitsIsNotExponent(t *testing.T) {
	toks, err := New("1e").Tokenize()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(toks) < 2 || toks[0].Type != INT || toks[0].Value != "1" {
		t.Fatalf("ожидался INT \"1\" первым токеном, получено %v", toks)
	}
	if toks[1].Type != IDENT || toks[1].Value != "e" {
		t.Fatalf("ожидался IDENT \"e\" вторым токеном, получено %v", toks)
	}
}
