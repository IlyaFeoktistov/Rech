package interp

import (
	"runtime"
	"strings"
	"testing"
)

// dlsymSrc - поиск функции по имени через dlsym (POSIX): тип результата
// подставляется в {{тип}}.
const dlsymSrc = `
внешняя функция dlopen(путь: Указатель<Байт>, флаги: Целое32): Указатель<Байт>
внешняя функция dlsym как найти(модуль: Указатель<Байт>, имя: Строка): {{тип}}

функция главная
{
    переменная ф = найти(dlopen(Ничто, (Целое32) 2), "нет_такой_функции")
    вывести ` + "`${ф == Ничто}\\n`" + `
}
`

func TestFFIFuncPointerResultNullIsNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dlopen/dlsym - POSIX")
	}
	src := strings.Replace(dlsymSrc, "{{тип}}", "?Вызываемый(Целое32): Целое32", 1)
	if got := runSrc(t, src); got != "истина\n" {
		t.Fatalf("NULL у \"?Вызываемый\" - Ничто, получено %q", got)
	}
}

func TestFFIFuncPointerResultNullPanicsWithoutQuestion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dlopen/dlsym - POSIX")
	}
	src := strings.Replace(dlsymSrc, "{{тип}}", "Вызываемый(Целое32): Целое32", 1)
	src = strings.Replace(src, "${ф == Ничто}", "${ф((Целое32) 1)}", 1)
	err := runSrcExpectRuntimeError(t, src)
	if err == nil || !strings.Contains(err.Error(), "dlsym вернула NULL") {
		t.Fatalf("ожидалась паника про NULL от dlsym, получено %v", err)
	}
}
