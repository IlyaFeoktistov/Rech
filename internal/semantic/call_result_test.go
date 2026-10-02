package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeCallOfCallResult(t *testing.T) {
	if _, err := AnalyzeSource(t, `
функция создать(): Вызываемый(Целое): Целое
{
    вернуть функция (x: Целое): Целое { вернуть x * 2 }
}

функция главная
{
    Список<Вызываемый(Целое): Целое> сп = [создать()]
    вывести сп.получить(0)(3) + создать()(1)
}
`); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	_, err := AnalyzeSource(t, "функция ф(): Целое\n{\n    вернуть 1\n}\n\nфункция главная\n{\n    вывести ф()(2)\n}\n")
	if err == nil || !strings.Contains(err.Error(), "вызвать можно только значение типа \"Вызываемый\"") {
		t.Fatalf("ожидалась ошибка про не-Вызываемый, получено %v", err)
	}
}
