package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeExternVar(t *testing.T) {
	if _, err := AnalyzeSource(t, "внешняя Целое32 opterr как флаг\n\nфункция главная\n{\n    флаг = 0\n    флаг++\n    вывести флаг\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	_, err := AnalyzeSource(t, "внешняя Строка environ\n\nфункция главная\n{\n}\n")
	if err == nil || !strings.Contains(err.Error(), "не поддерживается") {
		t.Fatalf("ожидалась ошибка про тип внешней переменной, получено %v", err)
	}
}
