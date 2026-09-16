package interp

import "testing"

func TestRunFuncRefExpr(t *testing.T) {
	out := runSrc(t, `
функция удвоить(x: Целое): Целое { вернуть x * 2 }
функция главная
{
    переменная ф = &удвоить
    вывести ф(21)
}
`)
	if out != "42\n" {
		t.Fatalf("ожидалось \"42\\n\", получено %q", out)
	}
}

func TestRunLambdaCaptureIsSnapshotAtCreation(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная база = 10
    переменная сложить = функция(x: Целое): Целое { вернуть x + база }
    вывести сложить(5)
    база = 100
    вывести сложить(5)
}
`)
	if out != "15\n15\n" {
		t.Fatalf("ожидалось \"15\\n15\\n\", получено %q", out)
	}
}

func TestRunLambdaMutationPersistsAcrossCalls(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная счётчик = 0
    переменная увеличить = функция()
    {
        счётчик = счётчик + 1
        вывести счётчик
    }
    увеличить()
    увеличить()
    увеличить()
    вывести счётчик
}
`)
	if out != "1\n2\n3\n0\n" {
		t.Fatalf("ожидалось \"1\\n2\\n3\\n0\\n\", получено %q", out)
	}
}

// TestRunLambdaExplicitCaptureByValueAndByRef - явный список захвата
// "[x, &y]": x - копия (мутация СНАРУЖИ после вызова не видна изнутри - та
// же семантика, что и у неявного захвата, TestRunLambdaCaptureIsSnapshotAtCreation
// выше), y - по ссылке (мутация ИЗНУТРИ видна снаружи, без ручного
// Указатель<T> - см. package doc ir.CaptureVar).
func TestRunLambdaExplicitCaptureByValueAndByRef(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная x = 10
    переменная y = 100

    переменная ф = функция(): Пусто [x, &y]
    {
        y = y + 1
        вывести x
    }

    ф()
    ф()
    вывести "снаружи"
    вывести x
    вывести y
}
`)
	if out != "10\n10\nснаружи\n10\n102\n" {
		t.Fatalf("получено %q", out)
	}
}

func TestRunCallableAsArgument(t *testing.T) {
	out := runSrc(t, `
функция применить(значение: Целое, ф: Вызываемый(Целое): Целое): Целое
{
    вернуть ф(значение)
}
функция главная
{
    вывести применить(7, функция(x: Целое): Целое { вернуть x * 3 })
}
`)
	if out != "21\n" {
		t.Fatalf("ожидалось \"21\\n\", получено %q", out)
	}
}

func TestRunLambdaReturningLambda(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная внешний = 5
    переменная сделать_сумматор = функция(): Вызываемый(Целое): Целое
    {
        вернуть функция(x: Целое): Целое { вернуть x + внешний }
    }
    переменная сумматор = сделать_сумматор()
    вывести сумматор(10)
}
`)
	if out != "15\n" {
		t.Fatalf("ожидалось \"15\\n\", получено %q", out)
	}
}
