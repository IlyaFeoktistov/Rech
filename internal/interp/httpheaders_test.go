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

// TestRunHeaderValueIsCaseInsensitive - RFC 7230 §3.2 - имя поля заголовка
// регистронезависимо (до 2026-09-20 значение_заголовка сравнивало точно, с
// учётом регистра - живая находка при аудите поддержки заголовков).
func TestRunHeaderValueIsCaseInsensitive(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная заголовки = `+"`"+`content-type: text/plain`+"`"+`
    вывести `+"`${"+`значение_заголовка(заголовки, "Content-Type") ?? "?"}\n`+"`"+`
}
`)
	if out != "text/plain\n" {
		t.Fatalf("получили %q, хотели \"text/plain\\n\"", out)
	}
}

// TestRunHeaderValuesReturnsAllMatchesInOrder - "значения_заголовка" -
// ВСЕ совпадения по порядку появления, не только первое (headerValue
// выше отдаёт только первое) - настоящие HTTP-заголовки МОГУТ
// повторяться (`X-Forwarded-For`, несколько `Accept` и т. п.).
func TestRunHeaderValuesReturnsAllMatchesInOrder(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная заголовки = `+"`"+`X-Tag: a\r\nHost: h\r\nX-Tag: b\r\n`+"`"+`
    переменная теги = значения_заголовка(заголовки, "x-tag")
    для i от 0 до теги.длина() - 1
    {
        вывести `+"`${"+`теги.получить(i)}\n`+"`"+`
    }
}
`)
	if out != "a\nb\n" {
		t.Fatalf("получили %q, хотели \"a\\nb\\n\"", out)
	}
}

// TestRunHeaderValuesEmptyWhenMissing - имя не встретилось ни разу -
// пустой список, не Ничто/паника.
func TestRunHeaderValuesEmptyWhenMissing(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная теги = значения_заголовка("Host: h", "X-Tag")
    вывести `+"`${"+`теги.длина()}\n`+"`"+`
}
`)
	if out != "0\n" {
		t.Fatalf("получили %q, хотели \"0\\n\"", out)
	}
}
