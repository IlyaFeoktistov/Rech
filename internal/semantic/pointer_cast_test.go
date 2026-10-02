package semantic

import (
	"errors"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

func TestAnalyzePointerCastForms(t *testing.T) {
	_, err := AnalyzeSource(t, `
внешняя сущность FILE как Поток

структура Точка
{
    имеет:
        Целое x
}

функция главная
{
    Целое а = 5
    *Целое п = &а
    *Байт б = (*Байт) п
    *Целое обратно = (Указатель<Целое>) б
    Целое адрес = (Целое) п
    Натуральное без_знака = (Натуральное) п
    *Целое из_числа = (*Целое) адрес
    Поток поток = (Поток) п
    *Байт из_потока = (*Байт) поток
    Целое произведение = (*п) * 2
    Точка т = новая Точка(1)
    *Точка пт = &т
    Целое x = (*пт).x
    *Байт от_адреса = (*Байт) &а
    вывести произведение + x
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzePointerCastRejected(t *testing.T) {
	cases := map[string]string{
		"строка в указатель": `
функция главная
{
    *Целое п = (*Целое) "x"
}
`,
		"указатель в Целое32": `
функция главная
{
    Целое а = 5
    Целое32 н = (Целое32) &а
}
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := AnalyzeSource(t, src)
			if err == nil || !strings.Contains(err.Error(), "нельзя привести") {
				t.Fatalf("ожидалось \"нельзя привести\", получено %v", err)
			}
		})
	}
}

// TestAnalyzeNamedCastHint - "(Пара) з" у "Значение" подсказывает
// "з как Пара".
func TestAnalyzeNamedCastHint(t *testing.T) {
	cases := []struct{ src, wantMsg, wantHint string }{
		{"структура Пара\n{\n    имеет:\n        Целое а\n}\n\nфункция главная\n{\n    переменная з = (Значение) 1\n    переменная н = (Пара) з\n}\n",
			`приведение "(Пара) значение" есть только у примитивов`, "з как Пара"},
		{"функция главная\n{\n    переменная з = (Значение) 1\n    переменная н = (Список<Целое>) з\n}\n",
			`"(Список<Целое>) значение"`, "з как Список<Целое>"},
	}
	for _, c := range cases {
		_, err := AnalyzeSource(t, c.src)
		var d *diagnostics.Diagnostic
		if !errors.As(err, &d) || !strings.Contains(d.Message, c.wantMsg) || d.Hint != c.wantHint {
			t.Errorf("%q: получено %v", c.src, err)
		}
	}
}
