package cbackend

import (
	"fmt"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestFFICallbackPoolEmitsDistinctTrampolinesPerSlot - найденное живьём
// ограничение ("один активный колбэк на сигнатуру" - вторая регистрация
// ТИХО заменяла первую) закрыто пулом слотов (см. package doc
// ffi_callback.go) - два места регистрации одной и той же
// "Вызываемый()"-сигнатуры (два atexit подряд) должны получить РАЗНЫЕ
// индексы пула (rech_cbidx_tmp...) и, значит, РАЗНЫЕ выражения адреса
// трамплина (rech_cbtrampptrs_...[индекс]) - не один и тот же C-идентификатор
// дважды. Заодно проверяет, что весь пул (ir.FFICallbackPoolSize
// статических трамплинов + массив указателей на них) реально эмитится.
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
	if strings.Count(out, "rech_cbused_") == 0 {
		t.Fatalf("ожидался счётчик занятых слотов пула, получено:\n%s", out)
	}
	wantTramp := fmt.Sprintf("static void %s(void) {", "rech_cbtramp_"+transliterate("Вызываемый()")+"_0")
	if !strings.Contains(out, "_0(void) {") || !strings.Contains(out, fmt.Sprintf("_%d(void) {", ir.FFICallbackPoolSize-1)) {
		t.Fatalf("ожидались трамплины с 0 по %d, wantTramp пример %q, получено:\n%s", ir.FFICallbackPoolSize-1, wantTramp, out)
	}
	if strings.Count(out, "rech_cbidx_tmp") < 2 {
		t.Fatalf("ожидались ДВЕ отдельные переменные индекса слота (по одной на регистрацию), получено:\n%s", out)
	}
	// Сами два индекса (rech_cbidx_tmpN/tmpM) обязаны быть РАЗНЫМИ
	// идентификаторами - иначе обе регистрации делили бы один слот.
	idxVars := map[string]bool{}
	for line := range strings.SplitSeq(out, "\n") {
		if i := strings.Index(line, "int rech_cbidx_tmp"); i >= 0 {
			name := strings.Fields(line[i+4:])[0]
			idxVars[name] = true
		}
	}
	if len(idxVars) < 2 {
		t.Fatalf("ожидались два РАЗНЫХ идентификатора индекса слота, получено %v в:\n%s", idxVars, out)
	}
}
