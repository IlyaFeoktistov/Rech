package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// TestParseArrayTypeSugar - "Тип[]"/"Тип[][]" - синоним "Список<Тип>"/
// "Список<Список<Тип>>", раскрывается прямо в парсере (см.
// package doc parseTypeName).
func TestParseArrayTypeSugar(t *testing.T) {
	cases := map[string]string{
		"Целое[]":   "Список<Целое>",
		"Целое[][]": "Список<Список<Целое>>",
		"?Целое[]":  "?Список<Целое>",
	}
	for src, want := range cases {
		toks, err := lexer.New(src).Tokenize()
		if err != nil {
			t.Fatalf("%q: лексер: %v", src, err)
		}
		p := New(toks)
		got, err := p.parseTypeName()
		if err != nil {
			t.Fatalf("%q: неожиданная ошибка: %v", src, err)
		}
		if got != want {
			t.Fatalf("parseTypeName(%q) = %q, ожидалось %q", src, got, want)
		}
	}
}
