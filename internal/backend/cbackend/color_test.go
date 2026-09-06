package cbackend

import (
	"strings"
	"testing"
)

// TestGenerateFromIRColorEmittedOnlyWhenUsed - "rgbцвет_из_hex"/
// "rgbцвет_в_hex" должны подключить colorHelpers только если программа
// реально их вызывает - обычная программа без единого цвета не должна
// тащить rech_hex_to_color/rech_color_to_hex вовсе.
func TestGenerateFromIRColorEmittedOnlyWhenUsed(t *testing.T) {
	src := `функция главная
{
    вывести "привет"
}
`
	out := generateFromSrc(t, src)
	if strings.Contains(out, "rech_hex_to_color") {
		t.Fatalf("rech_hex_to_color не должен эмитироваться без вызова \"rgbцвет_из_hex\":\n%s", out)
	}
}

// TestGenerateFromIRColorRoundTrip - "rgbцвет_из_hex"/"rgbцвет_в_hex"
// эмитируют sscanf/snprintf-реализацию, использующую поля встроенной
// сущности "RGBЦвет".
func TestGenerateFromIRColorRoundTrip(t *testing.T) {
	src := `функция главная
{
    переменная c = rgbцвет_из_hex("#FFD700")
    вывести rgbцвет_в_hex(c)
}
`
	out := generateFromSrc(t, src)
	if !strings.Contains(out, "rech_hex_to_color") || !strings.Contains(out, "rech_color_to_hex") {
		t.Fatalf("ожидались обе функции rech_hex_to_color/rech_color_to_hex, получено:\n%s", out)
	}
	if !strings.Contains(out, "sscanf(") {
		t.Fatalf("ожидался разбор через sscanf, получено:\n%s", out)
	}
}
