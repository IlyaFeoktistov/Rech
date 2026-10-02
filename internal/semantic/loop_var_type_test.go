package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeTypedLoopVars(t *testing.T) {
	if _, err := AnalyzeSource(t, "функция главная\n{\n    для Байт б от 0 до 255 {\n    }\n    Список<Байт> байты = [1]\n    для i, Байт x в байтах {\n    }\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	cases := []struct{ src, want string }{
		{"функция главная\n{\n    Список<Байт> байты = [1]\n    для Целое x в байтах {\n    }\n}\n", "а элементы - типа Байт"},
		{"функция главная\n{\n    для Дробное д от 1 до 2 {\n    }\n}\n", "целого типа, Байт или Символ"},
		{"функция главная\n{\n    Целое н = 3\n    для Целое32 i от 0 до н {\n    }\n}\n", "как переменная цикла"},
	}
	for _, c := range cases {
		_, err := AnalyzeSource(t, c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: ожидалась ошибка с %q, получено %v", c.src, c.want, err)
		}
	}
}
