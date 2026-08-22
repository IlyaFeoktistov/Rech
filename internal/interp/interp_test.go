package interp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

func runSrc(t *testing.T, src string) string {
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
	if err := Run(irProg, &buf); err != nil {
		t.Fatalf("исполнение: %v", err)
	}
	return buf.String()
}

// TestRunAllExamplesMatchCompiledOutput - сквозная проверка интерпретатора
// на каждом примере из examples/: ожидаемый вывод захвачен вручную реальным
// запуском скомпилированной (через cbackend + настоящий cc) программы -
// байт в байт совпадение с интерпретатором подтверждает, что второй backend
// поверх IR даёт то же наблюдаемое поведение, что и первый. В отличие от
// одноразовых zz_smoke_test.go у cbackend (не коммитятся - зависят от
// настоящего cc), этот тест постоянный: интерпретатор не вызывает системный
// компилятор вообще.
func TestRunAllExamplesMatchCompiledOutput(t *testing.T) {
	root, err := filepath.Abs("../../examples")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	cases := map[string]string{
		"коллекции/очередь.rech":          "истина\n3\n1\n1\n2\n1\nложь\n",
		"коллекции/список.rech":           "3\n10\n30\n99\nложь\n10\n99\n30\n",
		"коллекции/стопка.rech":           "истина\n3\n3\n3\n2\n1\nложь\n",
		"основы/выражения.rech":           "Привет, Илья!\n13\n7\n30\n3\n-10\n4\nистина\nистина\nложь\nложь\nистина\n11\n",
		"основы/привет.rech":              "Привет, мир!\n",
		"основы/строки.rech":              "строка1\nстрока2\nколонка1\tколонка2\nон сказал \"привет\"\nпуть: C:\\rech\nистина\nистина\nxy\n",
		"основы/управление.rech":          "больше пяти\n15\n120\nистина\n",
		"сущности/вложенные.rech":         "Илья\nМосква\nСанкт-Петербург\nКазань\n",
		"сущности/класс.rech":             "1\n2\n4\n6\n",
		"сущности/конструктор.rech":       "Илья\n30\nМудрая\n800\n600\n1\n",
		"сущности/состояния.rech":         "Профиль: Илья\n",
		"сущности/сущности.rech":          "Илья\n30\n31\nИлья\n32\n",
		"функции/естественный_вызов.rech": "Привет, Илья! Добро пожаловать в чат\n5\n",
		"функции/параметры.rech":          "Оформлен: Илья\n1\nОформлен: Илья\n1\n",
		"функции/строгий_вызов.rech":      "Привет, Илья! Добро пожаловать в чат\n5\n",
	}
	for rel, want := range cases {
		t.Run(rel, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			got := runSrc(t, string(src))
			if got != want {
				t.Fatalf("вывод не совпадает с эталонным (скомпилированным) -\nхотели:\n%q\nполучили:\n%q", want, got)
			}
		})
	}
}

func TestRunEntityFieldMutationVisibleAfterMethodReturn(t *testing.T) {
	got := runSrc(t, `
сущность Счётчик
{
    имеет:
        значение: Целое

    увеличить()
    {
        этот.значение = этот.значение + 1
    }
}

функция главная
{
    переменная c = новый Счётчик(0)
    c.увеличить()
    c.увеличить()
    вывести c.значение
}
`)
	if got != "2\n" {
		t.Fatalf("получили %q, хотели \"2\\n\"", got)
	}
}

func TestRunEntityValueSemanticsCopyOnAssign(t *testing.T) {
	got := runSrc(t, `
сущность Точка
{
    имеет:
        x: Целое
}

функция главная
{
    переменная a = новая Точка(1)
    переменная b = a
    b.x = 99
    вывести a.x
    вывести b.x
}
`)
	if got != "1\n99\n" {
		t.Fatalf("сущности должны копироваться при присваивании (значение, не ссылка), получили %q", got)
	}
}

// TestRunCollectionOutOfRangePanics проверяет, что ошибка времени
// выполнения (docs/SPEC.md, раздел 8) возвращается из Run как обычная
// error (runtimeError, см. interp.go) - в отличие от rech_panic в
// сгенерированном C (там процесс завершается сам, exit(1)), интерпретатор
// не решает сам, что делать с ошибкой выполнения программы Rech - это
// нужно "rech test", чтобы продолжить проверку остальных тестов после
// одного неудачного, а не завершить весь процесс.
func TestRunIntDivisionByZeroReturnsErrorNotCrash(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести "до"
    переменная а = 1 / 0
    вывести а
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
	err = Run(irProg, &buf)
	if err == nil || !strings.Contains(err.Error(), "деление на ноль") {
		t.Fatalf("ожидалась ошибка про деление на ноль, получено: %v", err)
	}
	if buf.String() != "до\n" {
		t.Fatalf("получили вывод %q", buf.String())
	}
}

func TestRunFloatDivisionByZeroDoesNotPanic(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная а = 1.0 / 0.0
    вывести а
}
`)
	if got != "inf\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunCollectionOutOfRangePanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная числа = новый Список<Целое>()
    вывести числа.получить(0)
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
	err = Run(irProg, &buf)
	if err == nil {
		t.Fatal("ожидалась ошибка времени выполнения (индекс вне границ)")
	}
	if !strings.Contains(err.Error(), "индекс вне границ") {
		t.Fatalf("ожидалось сообщение про индекс вне границ, получили: %v", err)
	}
}

func TestRunAssertPasses(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная а = 2
    утверждать а + 3 == 5
    вывести "ок"
}
`)
	if got != "ок\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunAssertFailsWithLineNumber(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести "до"
    утверждать 1 == 2
    вывести "после"
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
	err = Run(irProg, &buf)
	if err == nil {
		t.Fatal("ожидалась ошибка - утверждение ложно")
	}
	if !strings.Contains(err.Error(), "строка 5") {
		t.Fatalf("ожидался номер строки в сообщении, получили: %v", err)
	}
	if buf.String() != "до\n" {
		t.Fatalf("после неудачного \"утверждать\" выполнение должно остановиться - получили вывод %q", buf.String())
	}
}

func TestRunMutualRecursion(t *testing.T) {
	got := runSrc(t, `
функция чётное(n: Целое): Логическое
{
    если n == 0
    {
        вернуть истина
    }
    вернуть нечётное(n - 1)
}

функция нечётное(n: Целое): Логическое
{
    если n == 0
    {
        вернуть ложь
    }
    вернуть чётное(n - 1)
}

функция главная
{
    вывести чётное(10)
    вывести нечётное(10)
}
`)
	if got != "истина\nложь\n" {
		t.Fatalf("получили %q", got)
	}
}
