package semantic

import (
	"strings"
	"testing"
)

// TestAnalyzeUnboxCastHintsAs - "(Целое) з" у "Значение" подсказывает
// "з как Целое".
func TestAnalyzeUnboxCastHintsAs(t *testing.T) {
	_, err := AnalyzeSource(t, "функция главная\n{\n    Значение з = (Значение) 5\n    Целое ц = (Целое) з\n}\n")
	if err == nil || !strings.Contains(err.Error(), "з как Целое") {
		t.Fatalf("ожидалась подсказка \"з как Целое\", получено %v", err)
	}
}

// TestAnalyzeListLiteralIntoNullableList - "?Тип[] х = [...]": тип элемента
// берётся из цели, литерал заворачивается в "?".
func TestAnalyzeListLiteralIntoNullableList(t *testing.T) {
	if _, err := AnalyzeSource(t, "функция главная\n{\n    ?Целое[] есть = [1, 2]\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	_, err := AnalyzeSource(t, "функция главная\n{\n    ?Целое[] нет = [Ничто]\n}\n")
	if err == nil || !strings.Contains(err.Error(), "не nullable-тип") {
		t.Fatalf("ожидалась ошибка про элемент не nullable-типа, получено %v", err)
	}
}
