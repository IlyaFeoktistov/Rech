package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeIndexChain(t *testing.T) {
	if _, err := AnalyzeSource(t, "функция главная\n{\n    Список<Список<Целое>> л = [[1]]\n    Целое i = 0\n    ?Целое х = л[i][i + 0]\n    вывести `${х ?? 0}`\n}\n"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	_, err := AnalyzeSource(t, "функция ноль(): Целое\n{\n    вернуть 0\n}\n\nфункция главная\n{\n    Список<Список<Целое>> л = [[1]]\n    вывести `${л[ноль()][0] ?? 0}`\n}\n")
	if err == nil || !strings.Contains(err.Error(), "вынесите её в переменную") {
		t.Fatalf("ожидалась ошибка про вызов внутри цепочки, получено %v", err)
	}
}
