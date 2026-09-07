package interp

import (
	"strings"
	"testing"
)

// TestNumBaseRoundTrip - "целое_в_hex"/"целое_из_hex" и то же для "_oct"/
// "_bin" - настоящий прогон через интерпретатор. Отрицательное число -
// битовый паттерн (uint64), не ошибка - см. package doc internal/semantic/
// numbase.go.
func TestNumBaseRoundTrip(t *testing.T) {
	src := `функция главная
{
    вывести целое_в_hex(255)
    вывести целое_в_oct(15)
    вывести целое_в_bin(10)
    вывести целое_в_hex(-1)
    вывести целое_в_hex(0)
    вывести (Строка) целое_из_hex("0xFF")
    вывести (Строка) целое_из_hex("ff")
    вывести (Строка) целое_из_oct("0o17")
    вывести (Строка) целое_из_bin("0b1010")
    вывести (Строка) целое_из_hex(целое_в_hex(-1))
}
`
	out := runSrc(t, src)
	want := "0xff\n0o17\n0b1010\n0xffffffffffffffff\n0x0\n255\n255\n15\n10\n-1\n"
	if out != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", out, want)
	}
}

// TestNumBaseInvalidStringPanics - недопустимый символ в digits (даже без
// префикса) - паника, не тихий частичный разбор.
func TestNumBaseInvalidStringPanics(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `функция главная
{
    вывести (Строка) целое_из_hex("не число")
}
`)
	if err == nil {
		t.Fatal("ожидалась паника у некорректной hex-строки")
	}
	if !strings.Contains(err.Error(), "не является допустимым числом") {
		t.Fatalf("ожидалось сообщение про формат числа, получено: %v", err)
	}
}
