package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

// TestParseHintErrors - "(Пара) з" подсказывает "з как Пара", служебное
// слово на месте имени ("в", "с", "к") названо служебным словом.
func TestParseHintErrors(t *testing.T) {
	cases := []struct{ src, wantMsg, wantHint string }{
		{"функция главная\n{\n    переменная з = (Значение) 1\n    переменная н = (Пара) з\n}\n",
			`приведение "(Пара) значение" есть только у примитивов`, "з как Пара"},
		{"функция главная\n{\n    переменная з = (Значение) 1\n    переменная н = (Список<Целое>) з\n}\n",
			`"(Список<Целое>) значение"`, "з как Список<Целое>"},
		{"функция ф(в: Целое): Целое\n{\n    вернуть 1\n}\n", `служебное слово "в"`, `выберите другое имя, например "в_"`},
		{"функция главная\n{\n    Список<Целое> с = []\n}\n", `служебное слово "с"`, `выберите другое имя, например "с_"`},
	}
	for _, c := range cases {
		_, err := parseSource(t, c.src)
		var d *diagnostics.Diagnostic
		if !errors.As(err, &d) || !strings.Contains(d.Message, c.wantMsg) || d.Hint != c.wantHint {
			t.Errorf("%q: получено %v", c.src, err)
		}
	}
}
