package cbackend

import (
	"strings"
	"testing"
)

// TestGenerateFromIRHeaderValueEmitsHelper - "значение_заголовка" эмитит
// rech_header_value и объявление ?Строка, не привязываясь к Запрос/Ответ.
func TestGenerateFromIRHeaderValueEmitsHelper(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная заголовки = "Connection: keep-alive"
    переменная х = значение_заголовка(заголовки, "Connection")
    вывести х ?? "нет"
}
`)
	if !strings.Contains(out, "rech_header_value(") {
		t.Fatalf("ожидался вызов rech_header_value, получено:\n%s", out)
	}
	if !strings.Contains(out, "static "+mangleType("?Строка")+" rech_header_value(") {
		t.Fatalf("не найден хелпер rech_header_value, получено:\n%s", out)
	}
}
