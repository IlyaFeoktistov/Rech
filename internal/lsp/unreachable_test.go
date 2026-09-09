package lsp

import "testing"

func TestUnreachableAfterReturn(t *testing.T) {
	d := newDoc("test://a.rech", `
функция ф(): Целое
{
    вернуть 1
    вывести "недостижимо"
}
`)
	diags := d.unreachableDiagnostics()
	if len(diags) != 1 {
		t.Fatalf("ожидалась 1 диагностика недостижимого кода, получено %+v", diags)
	}
	if diags[0].Line != 5 {
		t.Fatalf("ожидалась строка 5, получено %+v", diags[0])
	}
	if diags[0].Severity != hintSeverity || len(diags[0].Tags) != 1 || diags[0].Tags[0] != unnecessaryTag {
		t.Fatalf("ожидался hintSeverity+unnecessaryTag, получено %+v", diags[0])
	}
}

func TestUnreachableAfterThrow(t *testing.T) {
	d := newDoc("test://a.rech", `
функция ф()
{
    выбросить новое Исключение("ошибка")
    вывести "недостижимо"
}
`)
	diags := d.unreachableDiagnostics()
	if len(diags) != 1 {
		t.Fatalf("ожидалась 1 диагностика, получено %+v", diags)
	}
}

func TestUnreachableAfterBreakInsideLoop(t *testing.T) {
	d := newDoc("test://a.rech", `
функция ф()
{
    переменная i = 0
    пока i < 10
    {
        прервать
        i++
    }
}
`)
	diags := d.unreachableDiagnostics()
	if len(diags) != 1 {
		t.Fatalf("ожидалась 1 диагностика, получено %+v", diags)
	}
}

func TestUnreachableAfterContinueInsideLoop(t *testing.T) {
	d := newDoc("test://a.rech", `
функция ф()
{
    переменная i = 0
    пока i < 10
    {
        продолжить
        i++
    }
}
`)
	diags := d.unreachableDiagnostics()
	if len(diags) != 1 {
		t.Fatalf("ожидалась 1 диагностика, получено %+v", diags)
	}
}

// TestUnreachableMultipleStatementsAllMarked - ДВЕ инструкции после
// "вернуть" - обе помечены (не только первая).
func TestUnreachableMultipleStatementsAllMarked(t *testing.T) {
	d := newDoc("test://a.rech", `
функция ф(): Целое
{
    вернуть 1
    вывести "а"
    вывести "б"
}
`)
	diags := d.unreachableDiagnostics()
	if len(diags) != 2 {
		t.Fatalf("ожидались 2 диагностики, получено %+v", diags)
	}
}

// TestUnreachableNotFlaggedWhenReturnIsLast - "вернуть" последней
// инструкцией блока - ничего не помечается (нечему быть недостижимым).
func TestUnreachableNotFlaggedWhenReturnIsLast(t *testing.T) {
	d := newDoc("test://a.rech", `
функция ф(): Целое
{
    вывести "а"
    вернуть 1
}
`)
	if diags := d.unreachableDiagnostics(); len(diags) != 0 {
		t.Fatalf("не ожидалось диагностик, получено %+v", diags)
	}
}

// TestUnreachableInsideLambdaCallback - недостижимый код внутри тела
// лямбды-колбэка (аргумент вызова) - та же дыра, что нашёл пользователь
// для hints.go (лямбды раньше не обходились вообще).
func TestUnreachableInsideLambdaCallback(t *testing.T) {
	d := newDoc("test://a.rech", `
функция чанковать(обработчик: Вызываемый(Целое))
{
    обработчик(1)
}
функция главная
{
    чанковать(функция(x: Целое)
    {
        вернуть
        вывести "недостижимо"
    })
}
`)
	if diags := d.unreachableDiagnostics(); len(diags) != 1 {
		t.Fatalf("ожидалась 1 диагностика внутри лямбды, получено %+v", diags)
	}
}

// TestUnreachableNotFlaggedInsideIfElseBothBranches - обе ветки "если"
// заканчиваются "вернуть", но код ПОСЛЕ самого "если" не анализируется
// (см. package doc unreachable.go про границы этого среза) - не ошибка,
// просто не покрыто.
func TestUnreachableNotFlaggedInsideIfElseBothBranches(t *testing.T) {
	d := newDoc("test://a.rech", `
функция ф(x: Целое): Целое
{
    если x > 0
    {
        вернуть 1
    }
    иначе
    {
        вернуть 0
    }
    вывести "не анализируется этим срезом"
}
`)
	if diags := d.unreachableDiagnostics(); len(diags) != 0 {
		t.Fatalf("не ожидалось диагностик (вне объёма этого среза), получено %+v", diags)
	}
}
