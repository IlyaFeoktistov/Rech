package interp

import (
	"strings"
	"testing"
)

// TestColorHexRoundTrip - "rgbцвет_из_hex"/"rgbцвет_в_hex" - настоящий
// прогон через интерпретатор, не только вызов Go-функций напрямую.
func TestColorHexRoundTrip(t *testing.T) {
	src := `функция главная
{
    переменная c = rgbцвет_из_hex("#FFD700")
    вывести (Целое) c.красный
    вывести (Целое) c.зелёный
    вывести (Целое) c.синий
    вывести rgbцвет_в_hex(c)
}
`
	out := runSrc(t, src)
	want := "255\n215\n0\n#FFD700\n"
	if out != want {
		t.Fatalf("получено:\n%q\nожидалось:\n%q", out, want)
	}
}

// TestColorHexInvalidFormatPanics - не 6 шестнадцатеричных цифр - паника,
// не тихо частично заполненный результат.
func TestColorHexInvalidFormatPanics(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `функция главная
{
    вывести rgbцвет_в_hex(rgbцвет_из_hex("не цвет"))
}
`)
	if err == nil {
		t.Fatal("ожидалась паника у некорректного hex")
	}
	if !strings.Contains(err.Error(), "шестнадцатеричных цифр") {
		t.Fatalf("ожидалось сообщение про формат hex, получено: %v", err)
	}
}
