package interp

import "testing"

func TestRunCookieValueFindsAndMissesName(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная запрос = новый Запрос("GET", "", 0, "/", "Cookie: сессия=abc123; тема=тёмная", "")
    вывести значение_куки(запрос, "сессия") ?? "?"
    вывести значение_куки(запрос, "тема") ?? "?"
    вывести значение_куки(запрос, "нет_такой") ?? "?"
}
`)
	if out != "abc123\nтёмная\n?\n" {
		t.Fatalf("ожидалось \"abc123\\nтёмная\\n?\\n\", получено %q", out)
	}
}

func TestRunCookieValueWithoutCookieHeader(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная запрос = новый Запрос("GET", "", 0, "/", "X-Foo: bar", "")
    вывести значение_куки(запрос, "сессия") ?? "нет"
}
`)
	if out != "нет\n" {
		t.Fatalf("ожидалось \"нет\\n\", получено %q", out)
	}
}

func TestRunCookieStringBuildsSetCookieHeader(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    вывести строка_куки("сессия", "abc", новый ОпцииКуки(путь: "/", только_http: истина, максимальный_возраст: 3600))
    вывести строка_куки("выход", "", новый ОпцииКуки(максимальный_возраст: 0))
    вывести строка_куки("простая", "1", новый ОпцииКуки())
}
`)
	want := "Set-Cookie: сессия=abc; Path=/; Max-Age=3600; HttpOnly\n" +
		"Set-Cookie: выход=; Max-Age=0\n" +
		"Set-Cookie: простая=1\n"
	if out != want {
		t.Fatalf("ожидалось %q, получено %q", want, out)
	}
}
