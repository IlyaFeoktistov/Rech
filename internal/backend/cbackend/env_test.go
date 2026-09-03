package cbackend

import (
	"strings"
	"testing"
)

// TestGenerateFromIREnvBuiltinCall - "получить_переменную_окружения" (internal/
// semantic/env.go) - генерируется через полный конвейер (generateFromSrc,
// db_test.go), а не вручную собранный ir.Program - сигнатура зависит от
// "?Строка" (nullable), заполняемого семантическим анализом, а не Kind из
// internal/builtins.
func TestGenerateFromIREnvBuiltinCall(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная х = получить_переменную_окружения("PATH")
    вывести х ?? "?"
}
`)
	for _, want := range []string{"rech_getenv(", "static", "getenv(name)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutEnvOmitsHelper(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    вывести 1
}
`)
	if strings.Contains(out, "rech_getenv") {
		t.Fatalf("не ожидался rech_getenv без использования \"получить_переменную_окружения\", получено:\n%s", out)
	}
}
