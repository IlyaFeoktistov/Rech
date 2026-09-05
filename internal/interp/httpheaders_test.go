package interp

import "testing"

func TestRunHeaderValueFindsAndMissesName(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная заголовки = "Host: example.com\r\nConnection: keep-alive\r\nX-Custom:   с пробелами  "
    вывести значение_заголовка(заголовки, "Connection") ?? "?"
    вывести значение_заголовка(заголовки, "Host") ?? "?"
    вывести значение_заголовка(заголовки, "X-Custom") ?? "?"
    вывести значение_заголовка(заголовки, "Accept") ?? "?"
}
`)
	want := "keep-alive\nexample.com\nс пробелами\n?\n"
	if out != want {
		t.Fatalf("получили %q, хотели %q", out, want)
	}
}

func TestRunHeaderValueOnEmptyHeaders(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    вывести значение_заголовка("", "Connection") ?? "нет"
}
`)
	if out != "нет\n" {
		t.Fatalf("получили %q, хотели \"нет\\n\"", out)
	}
}
