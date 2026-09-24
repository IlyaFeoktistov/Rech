package cbackend

import (
	"fmt"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestFFICallbackPoolEmitsDistinctTrampolinesPerSlot - каждая регистрация
// замыкания одной сигнатуры (два atexit подряд) занимает СВОЙ слот пула:
// rech_tocb_... берёт следующий индекс счётчика и отдаёт трамплин этого
// слота - не один и тот же адрес дважды. Весь пул (ir.FFICallbackPoolSize
// статических трамплинов + массив указателей на них) эмитится.
func TestFFICallbackPoolEmitsDistinctTrampolinesPerSlot(t *testing.T) {
	out := generateFromSrc(t, `
внешняя функция atexit(обработчик: Вызываемый())

функция главная
{
    переменная а = 1
    atexit(функция () { вывести а })
    переменная б = 2
    atexit(функция () { вывести б })
}
`)
	sig := "Вызываемый<>"
	wantTramp := fmt.Sprintf("static void %s(void) {", mangleCallbackTrampolineFn(sig, 0))
	if !strings.Contains(out, wantTramp) || !strings.Contains(out, fmt.Sprintf("static void %s(void) {", mangleCallbackTrampolineFn(sig, ir.FFICallbackPoolSize-1))) {
		t.Fatalf("ожидались трамплины с 0 по %d, получено:\n%s", ir.FFICallbackPoolSize-1, out)
	}
	for _, want := range []string{
		"int idx = " + mangleCallbackUsedVar(sig) + "++;",
		"return " + mangleCallbackTrampolinePtrs(sig) + "[idx];",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q (свой слот на каждую регистрацию), получено:\n%s", want, out)
		}
	}
	if strings.Count(out, "atexit("+mangleToCallback(sig)+"(") != 2 {
		t.Fatalf("обе регистрации идут через rech_tocb_..., получено:\n%s", out)
	}
}

// Указатель на функцию от C: прототип возвращает голый указатель,
// результат демаршалится (?Вызываемый - Ничто на NULL), обратно в C
// значение уходит тем же сырым указателем (rech_tocb_ проверяет thunk).
func TestGenerateFuncPointerFromCRoundTrip(t *testing.T) {
	out := generateFromSrc(t, `
внешняя функция signal(сигнал: Целое32, обработчик: Вызываемый(Целое32)): ?Вызываемый(Целое32)
внешняя функция dlsym как найти(модуль: Указатель<Байт>, имя: Строка): Вызываемый(Целое32)

функция главная
{
    переменная прежний = signal((Целое32) 2, функция (номер: Целое32) { })
    если прежний != Ничто {
        переменная обработчик: Вызываемый(Целое32) = прежний
        signal((Целое32) 2, обработчик)
    }
    переменная ф = найти(Ничто, "x")
}
`)
	sig := "Вызываемый<Целое32>"
	for _, want := range []string{
		"typedef void (*" + mangleRawFnType(sig) + ")(int32_t);",
		"extern " + mangleRawFnType(sig) + " signal(int32_t, void (*)(int32_t));",
		mangleFromRawOpt(sig) + "((" + mangleRawFnType(sig) + ")(signal(",
		mangleFromRaw(sig) + "((" + mangleRawFnType(sig) + ")(dlsym(",
		"вернула NULL вместо указателя на функцию",
		"if (c.fn == " + mangleRawThunk(sig) + ") {",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
}
