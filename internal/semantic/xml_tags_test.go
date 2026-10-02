package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeXMLAttrMustBePrimitive(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность А
{
    имеет:
        Список<Целое> х `+"`xml:@x`"+`
}
функция главная
{
    переменная а = разобрать XML "<a/>" как А
}
`)
	if err == nil || !strings.Contains(err.Error(), "не может быть XML-атрибутом") {
		t.Fatalf("ожидалась ошибка - атрибутом может быть только примитив, получено: %v", err)
	}
}

func TestAnalyzeXMLDuplicateAttrNames(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность А
{
    имеет:
        Целое х `+"`xml:@k`"+`
        Целое у `+"`xml:@k`"+`
}
функция главная
{
    переменная а = разобрать XML "<a/>" как А
}
`)
	if err == nil || !strings.Contains(err.Error(), "имя XML-атрибута \"k\"") {
		t.Fatalf("ожидалась ошибка - одинаковое имя атрибута, получено: %v", err)
	}
}

// TestAnalyzeXMLAttrAndElementSameNameAllowed - атрибут и дочерний
// элемент с одним именем не конфликтуют.
func TestAnalyzeXMLAttrAndElementSameNameAllowed(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность А
{
    имеет:
        Целое х `+"`xml:@k`"+`
        Целое у `+"`xml:k`"+`
}
функция главная
{
    переменная а = разобрать XML "<a/>" как А
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
