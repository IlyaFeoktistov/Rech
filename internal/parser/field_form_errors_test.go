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
		{"старая форма в имеет", "    имеет:\n        публичное начало: Целое = 100",
			`поле объявляется в форме "Тип имя", не "имя: Тип"`, "публичное Целое начало"},
		{"старая форма generic", "    имеет:\n        имена: Список<Строка>",
			`не "имя: Тип"`, "Список<Строка> имена"},
		{"поле вне имеет", "    публичное статическое Целое начало = 100",
			`поле объявляется в разделе "имеет:"`, "имеет:\n    публичное статическое Целое начало"},
		{"поле вне имеет через двоеточие", "    приватные имена: Строка",
			`поле объявляется в разделе "имеет:"`, "имеет:\n    приватные Строка имена"},
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

func TestParseMemberModifiersAnyOrder(t *testing.T) {
	prog, err := parseSource(t, `
сущность Счётчик
{
    имеет:
        публичное статическое Целое начало = 1
        статическое публичное Целое конец = 2

    публичный статический первый(): Целое
    {
        вернуть Счётчик::начало
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	for _, f := range ent.Fields {
		if !f.IsStatic {
			t.Fatalf("поле %q должно быть статическим", f.Name)
		}
	}
	if !ent.Methods[0].IsStatic {
		t.Fatalf("метод должен быть статическим")
	}
}
