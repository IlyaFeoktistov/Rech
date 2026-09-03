package interp

import "testing"

// TestRunGetEnv - "получить_переменную_окружения" (internal/semantic/env.go) - Ничто
// для неустановленной переменной, ?Строка со значением иначе - t.Setenv
// изолирует переменную для теста (Go его сам сбрасывает после).
func TestRunGetEnv(t *testing.T) {
	t.Setenv("RECH_TEST_ENV_VAR", "значение")
	got := runSrc(t, `
функция главная
{
    переменная есть = получить_переменную_окружения("RECH_TEST_ENV_VAR")
    вывести есть ?? "?"
    переменная нет = получить_переменную_окружения("RECH_TEST_ENV_VAR_НЕ_СУЩЕСТВУЕТ")
    вывести нет == Ничто
}
`)
	want := "значение\nистина\n"
	if got != want {
		t.Fatalf("ожидалось %q, получено %q", want, got)
	}
}
