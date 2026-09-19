package interp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

func TestRunRandomIntFixedRangeAndBothEndsReachable(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести `+"`${"+`случайное_целое(5, 5)}\n`+"`"+`
    переменная видел_ноль = ложь
    переменная видел_один = ложь
    переменная i = 0
    пока i < 300
    {
        переменная х = случайное_целое(0, 1)
        если х == 0 { видел_ноль = истина }
        если х == 1 { видел_один = истина }
        i = i + 1
    }
    вывести `+"`${"+`видел_ноль}\n`+"`"+`
    вывести `+"`${"+`видел_один}\n`+"`"+`
}
`)
	want := "5\nистина\nистина\n"
	if got != want {
		t.Fatalf("ожидалось %q, получено %q", want, got)
	}
}

func TestRunRandomFloatInUnitRange(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная а = случайное()
    вывести `+"`${"+`а >= 0.0 и а < 1.0}\n`+"`"+`
}
`)
	if got != "истина\n" {
		t.Fatalf("ожидалось \"истина\", получено %q", got)
	}
}

func TestRunRandomIntInvalidRangePanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести ` + "`${" + `случайное_целое(10, 1)}\n` + "`" + `
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
		t.Fatal("ожидалась ошибка времени выполнения (пустой диапазон)")
	}
	if !strings.Contains(err.Error(), rtmsg.RandomIntInvalidRange) {
		t.Fatalf("ожидалось сообщение %q, получено: %v", rtmsg.RandomIntInvalidRange, err)
	}
}
