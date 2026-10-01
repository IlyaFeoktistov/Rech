package semantic

import (
	"errors"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

// TestUnreachableCodeIsWarning - недостижимый код не останавливает анализ:
// по предупреждению (Severity Warning) на каждый недостижимый кусок - после
// "вернуть", после "если/иначе" с выходом в обеих ветках, в лямбде; тело
// generic-функции, проанализированное на каждую инстанциацию, даёт одно
// предупреждение, а не по одному на инстанциацию.
func TestUnreachableCodeIsWarning(t *testing.T) {
	irProg := parseProgram(t, `функция а(): Целое
{
    вернуть 1
    вывести "а"
    вывести "б"
}

функция б(х: Целое): Целое
{
    если х > 0 {
        вернуть 1
    } иначе {
        вернуть 2
    }
    вывести "после"
    вернуть 3
}

функция тождество<T>(х: T): T
{
    вернуть х
    вывести "после"
}

функция главная
{
    Вызываемый() ф = функция() {
        вернуть
        вывести "в лямбде"
    }
    ф()
    вывести а()
    вывести б(1)
    вывести тождество<Целое>(1)
    вывести тождество<Строка>("с")
}
`)
	var lines []int
	for _, w := range irProg.Warnings {
		var d *diagnostics.Diagnostic
		if !errors.As(w, &d) || d.Severity != diagnostics.Warning {
			t.Fatalf("предупреждение должно быть диагностикой уровня Warning: %v", w)
		}
		lines = append(lines, d.Pos.Line)
	}
	want := map[int]bool{4: true, 15: true, 22: true, 29: true}
	if len(lines) != len(want) {
		t.Fatalf("ожидались предупреждения на строках 4, 15, 22, 29, получено %v", lines)
	}
	for _, l := range lines {
		if !want[l] {
			t.Fatalf("ожидались предупреждения на строках 4, 15, 22, 29, получено %v", lines)
		}
	}
}
