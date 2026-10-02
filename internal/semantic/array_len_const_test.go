package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeArrayLenConst(t *testing.T) {
	if _, err := AnalyzeSource(t, "константа Н = 2\nконстанта М = Н * 3\n\nфункция главная\n{\n    Целое[М] а\n    Целое[6] б = а\n    вывести `${б.длина()}`\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	cases := []struct{ src, want string }{
		{"константа Р = \"x\"\n\nфункция главная\n{\n    Целое[Р] м\n}\n", "не целая константа модуля"},
		{"функция главная\n{\n    Целое[НЕТ] м\n}\n", "не целая константа модуля"},
		{"константа Н = 0\n\nфункция главная\n{\n    Целое[Н] м\n}\n", "больше нуля"},
	}
	for _, c := range cases {
		_, err := AnalyzeSource(t, c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: ожидалась ошибка с %q, получено %v", c.src, c.want, err)
		}
	}
}
