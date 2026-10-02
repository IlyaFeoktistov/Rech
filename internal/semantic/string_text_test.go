package semantic

import "testing"

func TestAnalyzeStringTextFunctions(t *testing.T) {
	if _, err := AnalyzeSource(t, "функция главная\n{\n    Строка т = \"Ёж\"\n    вывести т.в_верхний_регистр() + в_нижний_регистр(т) + т.заменить(\"ж\", \"Ж\")\n    вывести т.начинается_с(\"Ё\") и заканчивается_на(т, \"ж\")\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
