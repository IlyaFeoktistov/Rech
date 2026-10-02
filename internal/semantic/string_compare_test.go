package semantic

import "testing"

func TestAnalyzeStringOrdering(t *testing.T) {
	if _, err := AnalyzeSource(t, "функция главная\n{\n    вывести \"а\" < \"б\" и \"в\" >= \"в\"\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeStringMethods - функции строк вызываются как методы строки.
func TestAnalyzeStringMethods(t *testing.T) {
	if _, err := AnalyzeSource(t, "функция главная\n{\n    Строка т = \" а,б \"\n    вывести т.обрезать().разбить(\",\").длина() + \"ёж\".длина()\n    вывести т.содержит(\"а\")\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
