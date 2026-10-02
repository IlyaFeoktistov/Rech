package semantic

import "testing"

func TestAnalyzeStringOrdering(t *testing.T) {
	if _, err := AnalyzeSource(t, "функция главная\n{\n    вывести \"а\" < \"б\" и \"в\" >= \"в\"\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
