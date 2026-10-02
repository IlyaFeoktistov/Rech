package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeCVariadicExternFunc(t *testing.T) {
	const decl = "внешняя функция printf(формат: Строка, ...): Целое32\n\n"
	if _, err := AnalyzeSource(t, decl+"функция главная\n{\n    Дробное д = 1.5\n    printf(\"%d %f %s\", 1, д, \"x\")\n    printf(\"просто\")\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	cases := []struct{ src, want string }{
		{decl + "функция главная\n{\n    printf()\n}\n", "минимум 1"},
		{decl + "функция главная\n{\n    Список<Целое> л = [1]\n    printf(\"%p\", л)\n}\n", "на месте \"...\""},
		{decl + "функция главная\n{\n    Список<Целое> л = [1]\n    printf(\"%d\", ...л)\n}\n", "не раскрывается"},
		{"внешняя функция f(...)\n\nфункция главная\n{\n}\n", "хотя бы один обычный параметр"},
	}
	for _, c := range cases {
		_, err := AnalyzeSource(t, c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: ожидалась ошибка с %q, получено %v", c.src, c.want, err)
		}
	}
}
