package interp

import "testing"

func TestRunHeaderValueFindsAndMissesName(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная заголовки = `+"`"+`Host: example.com\r\nConnection: keep-alive\r\nX-Custom:   с пробелами  `+"`"+`
    вывести `+"`${"+`значение_заголовка(заголовки, "Connection") ?? "?"}\n`+"`"+`
    вывести `+"`${"+`значение_заголовка(заголовки, "Host") ?? "?"}\n`+"`"+`
    вывести `+"`${"+`значение_заголовка(заголовки, "X-Custom") ?? "?"}\n`+"`"+`
    вывести `+"`${"+`значение_заголовка(заголовки, "Accept") ?? "?"}\n`+"`"+`
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
    вывести `+"`${"+`значение_заголовка("", "Connection") ?? "нет"}\n`+"`"+`
}
`)
	if out != "нет\n" {
		t.Fatalf("получили %q, хотели \"нет\\n\"", out)
	}
}
