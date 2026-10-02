package semantic

import (
	"strings"
	"testing"
)

// TestCtorReturnWithValueRejected - "вернуть <значение>" в конструкторе -
// ошибка компиляции, текст называет конструктор, не функцию; голый
// "вернуть" допустим.
func TestCtorReturnWithValueRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `сущность Точка
{
    имеет:
        Целое x

    конструктор(x: Целое)
    {
        вернуть 5
    }
}

функция главная()
{
    Точка т = новая Точка(1)
}
`)
	if err == nil || !strings.Contains(err.Error(), "конструктор: не возвращает значения") {
		t.Fatalf("ожидалась ошибка про значение в конструкторе, получено: %v", err)
	}
}

// TestCtorBareReturnAllowed - голый "вернуть" в конструкторе - досрочный
// выход из тела.
func TestCtorBareReturnAllowed(t *testing.T) {
	parseProgram(t, `сущность Точка
{
    имеет:
        Целое x

    конструктор(x: Целое)
    {
        если x > 5 {
            вернуть
        }
        этот.x = x
    }
}

функция главная()
{
    Точка т = новая Точка(1)
}
`)
}
