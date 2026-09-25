package semantic

import (
	"strings"
	"testing"
)

const declaredTypeBase = `
интерфейс Фигура
{
    площадь(): Целое
}

сущность Квадрат реализует Фигуру
{
    имеет:
        публичная сторона: Целое

    публичный площадь(): Целое
    {
        вернуть 4
    }
}

сущность Круг реализует Фигуру
{
    публичный площадь(): Целое
    {
        вернуть 3
    }
}

функция главная
{
%s
}
`

// TestDeclaredTypeIsVariableType - тип переменной - аннотация, не тип
// значения: переменной интерфейса можно присвоить другую реализацию, а
// поля конкретной реализации через неё не видны.
func TestDeclaredTypeIsVariableType(t *testing.T) {
	ok := []string{
		"    Фигура ф = новый Квадрат()\n    ф = новый Круг()",
		"    Список<Фигура> фигуры = [новый Квадрат(), новый Круг()]",
		"    ?Фигура ф = Ничто\n    ф = новый Квадрат()",
		"    Список<?Фигура> фигуры = [новый Квадрат(), Ничто]",
	}
	for _, body := range ok {
		if _, err := AnalyzeSource(t, strings.Replace(declaredTypeBase, "%s", body, 1)); err != nil {
			t.Errorf("%q: неожиданная ошибка: %v", body, err)
		}
	}
	_, err := AnalyzeSource(t, strings.Replace(declaredTypeBase, "%s", "    Фигура ф = новый Квадрат()\n    вывести ф.сторона", 1))
	if err == nil || !strings.Contains(err.Error(), "сторона") {
		t.Fatalf("поле реализации через переменную интерфейса должно быть ошибкой, получено %v", err)
	}
}
