package lsp

import (
	"strings"
	"testing"
)

const synonymHoverSrc = `
/** Набор целых чисел. */
публичный синоним Числа = Список<Целое>

/** Числа по названию. */
приватный синоним Группы = Словарь<Строка, Список<Числа>>

синоним Ф = Вызываемый(Целое): Строка

функция главная
{
    переменная г = новые Группы()
    переменная ч: Числа = []
    переменная ф: Ф = функция(х: Целое): Строка { вернуть "" }
}
`

func TestHoverSynonymShowsDeclarationAsWritten(t *testing.T) {
	d := newDoc("test://a.rech", synonymHoverSrc)
	if len(d.diagnostics) != 0 {
		t.Fatalf("неожиданные ошибки: %v", d.diagnostics)
	}
	cases := []struct{ line, word, want, doc string }{
		{"переменная г = новые Группы()", "Группы", "приватный синоним Группы = Словарь<Строка, Список<Числа>>", "Числа по названию."},
		{"переменная ч: Числа = []", "Числа", "публичный синоним Числа = Список<Целое>", "Набор целых чисел."},
		{"переменная ф: Ф = функция", "Ф", "синоним Ф = Вызываемый(Целое): Строка", ""},
		{"публичный синоним Числа", "Числа", "публичный синоним Числа = Список<Целое>", ""},
	}
	for _, c := range cases {
		text, ok := hoverAt(t, d, synonymHoverSrc, c.line, c.word)
		if !ok || !strings.Contains(text, "```rech\n"+c.want+"\n```") || !strings.Contains(text, c.doc) {
			t.Errorf("hover на %q в %q: %q", c.word, c.line, text)
		}
	}
}

func TestDefinitionOnSynonymUse(t *testing.T) {
	d := newDoc("test://a.rech", synonymHoverSrc)
	line := lineOf(t, synonymHoverSrc, "переменная ч: Числа = []")
	pos, ok := d.bareNameDeclPos("Числа", line, d.scopeAt(line))
	if !ok || pos.Line != lineOf(t, synonymHoverSrc, "публичный синоним Числа") {
		t.Fatalf("переход к объявлению синонима: %+v, %v", pos, ok)
	}
}

// TestHoverBuiltinTypeSynonym - hover на "Целое" и на "Целое64" (и на
// "Натуральное"/"Натуральное64") показывает один и тот же синоним и явно
// говорит, что это один тип.
func TestHoverBuiltinTypeSynonym(t *testing.T) {
	src := `функция ф(а: Целое64, б: Натуральное): Целое
{
    вернуть а
}

функция главная
{
    Натуральное64 н = 1
    Байт б = 1
    Натуральное8 м = б
    вывести ф(1, н)
}
`
	d := newDoc("test://a.rech", src)
	for _, c := range []struct{ line, needle, want string }{
		{"функция ф(", "Целое64", "синоним Целое = Целое64"},
		{"функция ф(", "): Целое", "синоним Целое = Целое64"},
		{"функция ф(", "Натуральное", "синоним Натуральное = Натуральное64"},
		{"Натуральное64 н", "Натуральное64", "синоним Натуральное = Натуральное64"},
		{"Байт б", "Байт", "синоним Байт = Натуральное8"},
		{"Натуральное8 м", "Натуральное8", "синоним Байт = Натуральное8"},
	} {
		lineNo := lineOf(t, src, c.line)
		lineText := strings.Split(src, "\n")[lineNo-1]
		col := runeCol(t, lineText, c.needle)
		if strings.HasPrefix(c.needle, "): ") {
			col += 3
		}
		text, ok := d.hover(lineNo, col)
		if !ok || !strings.Contains(text, c.want) || !strings.Contains(text, "один и тот же тип") {
			t.Fatalf("hover на %q: %q, %v", c.needle, text, ok)
		}
	}
}
