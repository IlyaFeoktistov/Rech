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

func TestRunStringOpsIndexByCodepointsNotBytes(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная текст = "привет, мир!"
    вывести длина_строки(текст)
    вывести подстрока(текст, 0, 6)
    вывести подстрока(текст, 8, 3)
    вывести позиция_в_строке(текст, "мир")
    вывести позиция_в_строке(текст, "нет такого")
}
`)
	want := "12\nпривет\nмир\n8\n-1\n"
	if got != want {
		t.Fatalf("ожидалось %q, получено %q", want, got)
	}
}

func TestRunStringSplit(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная слова = разбить("а,б,в", ",")
    вывести слова.длина()
    вывести слова.получить(0)
    вывести слова.получить(2)

    переменная с_пустыми = разбить(",,x", ",")
    вывести с_пустыми.длина()
    вывести с_пустыми.получить(0)

    переменная без_вхождения = разбить("абв", ",")
    вывести без_вхождения.длина()
}
`)
	want := "3\nа\nв\n3\n\n1\n"
	if got != want {
		t.Fatalf("ожидалось %q, получено %q", want, got)
	}
}

func TestRunStringSplitEmptySeparatorPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная слова = разбить("абв", "")
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
		t.Fatal("ожидалась ошибка времени выполнения (пустой разделитель)")
	}
	if !strings.Contains(err.Error(), rtmsg.SplitEmptySeparator) {
		t.Fatalf("ожидалось сообщение %q, получено: %v", rtmsg.SplitEmptySeparator, err)
	}
}

func TestRunSubstringOutOfRangePanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести подстрока("абв", 0, 100)
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
		t.Fatal("ожидалась ошибка времени выполнения (диапазон вне строки)")
	}
	if !strings.Contains(err.Error(), rtmsg.SubstringInvalidRange) {
		t.Fatalf("ожидалось сообщение %q, получено: %v", rtmsg.SubstringInvalidRange, err)
	}
}
