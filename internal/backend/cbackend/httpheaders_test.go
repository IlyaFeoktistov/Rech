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
	if !strings.Contains(out, "rech_header_line_matches(") {
		t.Fatalf("ожидался общий регистронезависимый rech_header_line_matches, получено:\n%s", out)
	}
}

// TestGenerateFromIRHeaderValuesEmitsHelper - "значения_заголовка" эмитит
// rech_header_values (Список<Строка> - ВСЕ совпадения, не только первое).
func TestGenerateFromIRHeaderValuesEmitsHelper(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная теги = значения_заголовка("X-Tag: a\r\nX-Tag: b", "X-Tag")
    вывести теги.длина()
}
`)
	if !strings.Contains(out, "rech_header_values(") {
		t.Fatalf("ожидался вызов rech_header_values, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_header_line_matches(") {
		t.Fatalf("ожидался общий регистронезависимый rech_header_line_matches, получено:\n%s", out)
	}
}

// TestGenerateFromIRHeaderLineMatchesEmittedOnce - и значение_заголовка, и
// значения_заголовка используются в одной программе - общий
// rech_header_line_matches должен появиться РОВНО ОДИН раз (иначе
// повторное определение - ошибка компиляции настоящего cc).
func TestGenerateFromIRHeaderLineMatchesEmittedOnce(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная заголовки = "X-Tag: a"
    вывести значение_заголовка(заголовки, "X-Tag") ?? "?"
    переменная теги = значения_заголовка(заголовки, "X-Tag")
    вывести теги.длина()
}
`)
	if n := strings.Count(out, "static int rech_header_line_matches("); n != 1 {
		t.Fatalf("ожидалось ровно одно определение rech_header_line_matches, найдено %d, получено:\n%s", n, out)
	}
}

// TestGenerateFromIRWithoutHeaderFuncsOmitsHelper - ни значение_заголовка,
// ни значения_заголовка не вызваны - rech_header_line_matches не эмитится.
func TestGenerateFromIRWithoutHeaderFuncsOmitsHelper(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    вывести 1
}
`)
	if strings.Contains(out, "rech_header_line_matches") {
		t.Fatalf("не ожидался rech_header_line_matches без вызова значение_заголовка/значения_заголовка, получено:\n%s", out)
	}
}
