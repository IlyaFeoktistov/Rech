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
