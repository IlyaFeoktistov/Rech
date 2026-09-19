package interp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// runSrcExpectRuntimeError - как runSrc, но для программ, которые должны
// провалиться во время исполнения (Run возвращает error, не панику Go -
// см. panicRuntime/runtimeError, interp.go) - тот же принцип, что и у
// TestRunIntDivisionByZeroReturnsErrorNotCrash/
// TestRunCollectionOutOfRangePanics выше в этом пакете.
func runSrcExpectRuntimeError(t *testing.T, src string) error {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
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
	return Run(irProg, &buf, strings.NewReader(""))
}

func TestRunCastNumericConversions(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести `+"`${"+`(Дробное) 5}\n`+"`"+`
    вывести `+"`${"+`(Действительное) 5}\n`+"`"+`
    вывести `+"`${"+`(Целое) 3.9}\n`+"`"+`
    вывести `+"`${"+`(Целое) (Дробное) 3.9}\n`+"`"+`
}
`)
	want := "5\n5\n3\n3\n"
	if got != want {
		t.Fatalf("хотели %q, получили %q", want, got)
	}
}

func TestRunCastStringRoundTrip(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести `+"`${"+`(Строка) 42}\n`+"`"+`
    вывести `+"`${"+`(Целое) "42"}\n`+"`"+`
    вывести `+"`${"+`(Строка) 2.5}\n`+"`"+`
    вывести `+"`${"+`(Действительное) "2.5"}\n`+"`"+`
}
`)
	want := "42\n42\n2.5\n2.5\n"
	if got != want {
		t.Fatalf("хотели %q, получили %q", want, got)
	}
}

// TestRunCastBoolStringRoundTrip - найденный пробел: "(Строка) булево" не
// поддерживался вовсе (allowedCasts не содержал Логическое ни в одну
// сторону) - "текст + (Строка) отладка" не работал, обнаружено при
// добавлении шаблонных строк (интерполяция ${булево}). "истина"/"ложь" -
// тот же текст, что и вывод "вывести"/"показать".
func TestRunCastBoolStringRoundTrip(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести `+"`${"+`(Строка) истина}\n`+"`"+`
    вывести `+"`${"+`(Строка) ложь}\n`+"`"+`
    вывести `+"`${"+`(Логическое) "истина"}\n`+"`"+`
    вывести `+"`${"+`(Логическое) "ложь"}\n`+"`"+`
}
`)
	want := "истина\nложь\nистина\nложь\n"
	if got != want {
		t.Fatalf("хотели %q, получили %q", want, got)
	}
}

func TestRunCastCharAndByte(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести `+"`${"+`(Целое) 'a'}\n`+"`"+`
    вывести `+"`${"+`(Символ) 97}\n`+"`"+`
    вывести `+"`${"+`(Байт) 300}\n`+"`"+`
    вывести `+"`${"+`(Целое) (Байт) 300}\n`+"`"+`
    вывести `+"`${"+`(Символ) (Байт) 97}\n`+"`"+`
}
`)
	want := "97\na\n44\n44\na\n"
	if got != want {
		t.Fatalf("хотели %q, получили %q", want, got)
	}
}

func TestRunCastStringToIntInvalidFormatFails(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `
функция главная
{
    вывести `+"`${"+`(Целое) "abc"}\n`+"`"+`
}
`)
	if err == nil || !strings.Contains(err.Error(), `каст (Целое): "abc" не является целым числом`) {
		t.Fatalf("ожидалась ошибка про некорректный формат каста, получено: %v", err)
	}
}

func TestRunCastStringToFloatInvalidFormatFails(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `
функция главная
{
    вывести `+"`${"+`(Действительное) "abc"}\n`+"`"+`
}
`)
	if err == nil || !strings.Contains(err.Error(), `каст (Действительное): "abc" не является числом`) {
		t.Fatalf("ожидалась ошибка про некорректный формат каста, получено: %v", err)
	}
}

func TestRunCastStringToBoolInvalidFormatFails(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `
функция главная
{
    вывести `+"`${"+`(Логическое) "true"}\n`+"`"+`
}
`)
	if err == nil || !strings.Contains(err.Error(), `каст (Логическое): "true" не является "истина" или "ложь"`) {
		t.Fatalf("ожидалась ошибка про некорректный формат каста, получено: %v", err)
	}
}

func TestRunCastStringToCharRequiresExactlyOneRune(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `
функция главная
{
    вывести `+"`${"+`(Символ) "ab"}\n`+"`"+`
}
`)
	if err == nil || !strings.Contains(err.Error(), "не состоит ровно из одного символа") {
		t.Fatalf("ожидалась ошибка - строка не из одного символа, получено: %v", err)
	}
}
