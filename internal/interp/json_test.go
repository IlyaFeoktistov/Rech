package interp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

func TestRunParseExprEntityAndNested(t *testing.T) {
	got := runSrc(t, `
сущность Адрес
{
    имеет:
        город: Строка
}
сущность Пользователь
{
    имеет:
        имя: Строка
        возраст: Целое
        активен: Логическое
        адрес: Адрес
}
функция главная
{
    переменная текст = `+"`"+`{"имя": "Аня", "возраст": 30, "активен": true, "адрес": {"город": "Москва"}}`+"`"+`
    переменная п = разобрать JSON текст как Пользователь
    вывести `+"`${"+`п.имя}\n`+"`"+`
    вывести `+"`${"+`п.возраст}\n`+"`"+`
    вывести `+"`${"+`п.активен}\n`+"`"+`
    вывести `+"`${"+`п.адрес.город}\n`+"`"+`
}
`)
	want := "Аня\n30\nистина\nМосква\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

func TestRunParseExprPrimitiveAndList(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная возраст = разобрать JSON "42" как Целое
    вывести `+"`${"+`возраст}\n`+"`"+`

    переменная числа = разобрать JSON "[1, 2, 3]" как Список<Целое>
    вывести `+"`${"+`числа.длина()}\n`+"`"+`
    вывести `+"`${"+`числа.получить(1)}\n`+"`"+`
}
`)
	want := "42\n3\n2\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

func TestRunParseExprMissingFieldThrowsCatchable(t *testing.T) {
	got := runSrc(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
}
функция главная
{
    попытаться {
        переменная п = разобрать JSON "{}" как Пользователь
        вывести `+"`"+`не должно\n`+"`"+`
    } поймать(иск: ОшибкаРазбора) {
        вывести `+"`${"+`иск.сообщение}\n`+"`"+`
    }
}
`)
	want := "разобрать JSON: отсутствует поле \"имя\"\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

func TestRunParseExprInvalidJSONUncaughtMessage(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная п = разобрать JSON "не json" как Целое
    вывести ` + "`${" + `п}\n` + "`" + `
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		t.Fatalf("семантический анализ: %v", err)
	}
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil {
		t.Fatal("ожидалась ошибка (некорректный JSON, необработанное исключение)")
	}
	if !strings.Contains(err.Error(), "необработанное исключение ОшибкаРазбора") {
		t.Fatalf("ожидалось необработанное исключение ОшибкаРазбора, получили: %v", err)
	}
}
