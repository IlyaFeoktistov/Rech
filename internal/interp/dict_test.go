package interp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

func TestRunDictSetGetContainsRemove(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная возраста = новый Словарь<Строка, Целое>()
    возраста.установить("Аня", 30)
    возраста.установить("Боря", 25)
    возраста.установить("Аня", 31)

    вывести возраста.длина()
    вывести возраста.получить("Аня")
    вывести возраста.содержит("Боря")
    вывести возраста.содержит("Вова")

    возраста.удалить("Боря")
    вывести возраста.длина()
    вывести возраста.содержит("Боря")
}
`)
	want := "2\n31\nистина\nложь\n1\nложь\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

// TestRunDictPassedToFunctionMutatesCallersCopy - спецификационный тест
// (не регрессионный для interp - здесь ссылочная семантика уже была верна,
// баг был в cbackend, docs/ROADMAP.md Шаг 28) - фиксирует КОНТРАКТ: Словарь,
// переданный обычным параметром функции, - ссылка на тот же объект, мутация
// внутри функции видна вызывающей стороне после возврата (как Список,
// Стопка, Очередь - все встроенные generic-коллекции).
func TestRunDictPassedToFunctionMutatesCallersCopy(t *testing.T) {
	got := runSrc(t, `
функция вставить_в(д: Словарь<Строка, Строка>)
{
    д.установить("а", "1")
}

функция главная
{
    переменная д = новый Словарь<Строка, Строка>()
    вставить_в(д)
    вывести д.содержит("а")
    вывести д.длина()
}
`)
	want := "истина\n1\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

func TestRunDictGetMissingKeyPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная д = новый Словарь<Целое, Строка>()
    вывести д.получить(1)
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
		t.Fatal("ожидалась ошибка времени выполнения (ключ не найден)")
	}
	if !strings.Contains(err.Error(), "ключ не найден") {
		t.Fatalf("ожидалось сообщение про отсутствующий ключ, получили: %v", err)
	}
}
