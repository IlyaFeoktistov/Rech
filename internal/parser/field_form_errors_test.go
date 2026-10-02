package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

func TestParseFieldFormErrors(t *testing.T) {
	cases := []struct {
		name, body, wantMsg, wantHint string
	}{
		{"тип первым в имеет", "    имеет:\n        публичное Целое начало = 100",
			`поле объявляется в форме "имя: Тип", не "Тип имя"`, "публичное начало: Целое"},
		{"generic тип первым", "    имеет:\n        Список<Строка> имена",
			`не "Тип имя"`, "имена: Список<Строка>"},
		{"видимость перед статическим", "    имеет:\n        публичное статическое начало: Целое",
			`модификатор "статическое" пишется перед модификатором видимости`, "статическое публичное ..."},
		{"поле вне имеет", "    публичное статическое Целое начало = 100",
			`поле объявляется в разделе "имеет:"`, "имеет:\n    статическое публичное начало: Целое"},
		{"поле вне имеет через двоеточие", "    приватные имена: Строка",
			`поле объявляется в разделе "имеет:"`, "имеет:\n    приватные имена: ..."},
	}
	for _, c := range cases {
		_, err := parseSource(t, "сущность Счётчик\n{\n"+c.body+"\n}\nфункция главная { вывести \"ок\" }\n")
		var d *diagnostics.Diagnostic
		if !errors.As(err, &d) {
			t.Fatalf("%s: ожидалась диагностика, получено %v", c.name, err)
		}
		if !strings.Contains(d.Message, c.wantMsg) || d.Hint != c.wantHint {
			t.Fatalf("%s: получено %q / подсказка %q", c.name, d.Message, d.Hint)
		}
	}
}

func TestParseMethodWithoutParensKeepsMethodError(t *testing.T) {
	_, err := parseSource(t, "сущность Счётчик\n{\n    публичный считать\n    {\n    }\n}\nфункция главная { вывести \"ок\" }\n")
	if err == nil || !strings.Contains(err.Error(), `после имени метода "считать" ожидалось "("`) {
		t.Fatalf("ожидалась ошибка про \"(\" у метода, получено %v", err)
	}
}
