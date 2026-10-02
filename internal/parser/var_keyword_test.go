package parser

import (
	"errors"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

// TestParseVariableKeywordForms - "переменная" только с значением (тип
// выводится); без значения или с ": Тип" - ошибка с подсказкой формы "Тип имя".
func TestParseVariableKeywordForms(t *testing.T) {
	cases := []struct{ stmt, wantMsg, wantHint string }{
		{"переменная х", `у "переменная х" без значения не вычислить тип`, "<Тип> х, например: Целое х"},
		{"переменная х: Целое = 1", `"переменная" выводит тип из значения`, "Целое х = ..."},
		{"переменная х: ?Строка", `"переменная" выводит тип из значения`, "?Строка х"},
	}
	for _, c := range cases {
		_, err := parseSource(t, "функция главная {\n    "+c.stmt+"\n}\n")
		var d *diagnostics.Diagnostic
		if !errors.As(err, &d) || !strings.Contains(d.Message, c.wantMsg) || d.Hint != c.wantHint {
			t.Fatalf("%s: получено %v", c.stmt, err)
		}
	}
}
