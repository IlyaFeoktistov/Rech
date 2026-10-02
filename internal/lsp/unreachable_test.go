package lsp

import "testing"

// unreachableRegion - диапазон затенения недостижимого кода (строки
// начала/конца).
type unreachableRegion struct{ from, to int }

// unreachableRegions - затенённые куски d (hint + unnecessaryTag); у
// каждого должно быть парное предупреждение на первой строке.
func unreachableRegions(t *testing.T, d *doc) []unreachableRegion {
	t.Helper()
	var regions []unreachableRegion
	warnings := map[int]bool{}
	for _, dg := range d.unreachableDiagnostics() {
		switch {
		case dg.Severity == hintSeverity && len(dg.Tags) == 1 && dg.Tags[0] == unnecessaryTag:
			regions = append(regions, unreachableRegion{dg.Line, dg.EndLine})
		case dg.Severity == warningSeverity:
			warnings[dg.Line] = true
		default:
			t.Fatalf("неожиданная диагностика %+v", dg)
		}
	}
	for _, r := range regions {
		if !warnings[r.from] {
			t.Fatalf("у затенённого куска %v нет предупреждения на первой строке", r)
		}
	}
	return regions
}

func expectRegions(t *testing.T, src string, want ...unreachableRegion) {
	t.Helper()
	got := unreachableRegions(t, newDoc("test://a.rech", src))
	if len(got) != len(want) {
		t.Fatalf("ожидались куски %v, получено %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ожидались куски %v, получено %v", want, got)
		}
	}
}

func TestUnreachableAfterReturn(t *testing.T) {
	expectRegions(t, `
функция ф(): Целое
{
    вернуть 1
    вывести "недостижимо"
}
`, unreachableRegion{5, 5})
}

func TestUnreachableAfterThrow(t *testing.T) {
	expectRegions(t, `
функция ф()
{
    выбросить новое Исключение("ошибка")
    вывести "недостижимо"
}
`, unreachableRegion{5, 5})
}

func TestUnreachableAfterBreakInsideLoop(t *testing.T) {
	expectRegions(t, `
функция ф()
{
    переменная i = 0
    пока i < 10
    {
        прервать
        i++
    }
}
`, unreachableRegion{8, 8})
}

func TestUnreachableAfterContinueInsideLoop(t *testing.T) {
	expectRegions(t, `
функция ф()
{
    переменная i = 0
    пока i < 10
    {
        продолжить
        i++
    }
}
`, unreachableRegion{8, 8})
}

// TestUnreachableRegionCoversRestOfBlock - затеняется весь остаток блока
// одним куском: несколько инструкций, многострочный "если" и выражение с
// переносом строки.
func TestUnreachableRegionCoversRestOfBlock(t *testing.T) {
	expectRegions(t, `
функция ф(х: Целое): Целое
{
    вернуть 1
    вывести "а"
    если х > 0 {
        вывести "б"
    }
    переменная у =
        х + 1
}
`, unreachableRegion{5, 10})
}

// TestUnreachableNotFlaggedWhenReturnIsLast - "вернуть" последней
// инструкцией блока - ничего не помечается (нечему быть недостижимым).
func TestUnreachableNotFlaggedWhenReturnIsLast(t *testing.T) {
	expectRegions(t, `
функция ф(): Целое
{
    вывести "а"
    вернуть 1
}
`)
}

// TestUnreachableInsideLambdaCallback - недостижимый код внутри тела
// лямбды-колбэка (аргумент вызова).
func TestUnreachableInsideLambdaCallback(t *testing.T) {
	expectRegions(t, `
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
`, unreachableRegion{11, 11})
}

// TestUnreachableAfterIfElseBothBranchesReturn - обе ветки "если/иначе"
// выходят из функции - код после самого "если" недостижим.
func TestUnreachableAfterIfElseBothBranchesReturn(t *testing.T) {
	expectRegions(t, `
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
    вывести "после если"
    вернуть 2
}
`, unreachableRegion{12, 13})
}

// TestUnreachableAfterInfiniteLoop - код после "пока истина" без
// "прервать".
func TestUnreachableAfterInfiniteLoop(t *testing.T) {
	expectRegions(t, `
функция ф()
{
    пока истина {
        вывести "x"
    }
    вывести "после цикла"
}
`, unreachableRegion{7, 7})
}

// TestUnreachableShownWithOtherErrorsInFile - затенение работает и когда
// в другом месте файла есть ошибка (частичный анализ).
func TestUnreachableShownWithOtherErrorsInFile(t *testing.T) {
	expectRegions(t, `
функция сломанная()
{
    Целое т = "строка"
}

функция ф(): Целое
{
    вернуть 1
    вывести "недостижимо"
}
`, unreachableRegion{10, 10})
}
