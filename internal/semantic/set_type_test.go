package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeSetCollection(t *testing.T) {
	if _, err := AnalyzeSource(t, `
функция главная
{
    Множество<Строка> м = новое Множество<Строка>()
    м.добавить("а")
    для x в м {
        вывести x + "!"
    }
    вывести м.содержит("а") и м.равен(новое Множество<Строка>())
    вывести м.значения().длина()
}
`); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	_, err := AnalyzeSource(t, "структура П\n{\n    имеет:\n        Целое a\n}\n\nфункция главная\n{\n    Множество<П> м = новое Множество<П>()\n}\n")
	if err == nil || !strings.Contains(err.Error(), "тип элемента") {
		t.Fatalf("ожидалась ошибка про тип элемента, получено %v", err)
	}
}
