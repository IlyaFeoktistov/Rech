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
	if err := Run(irProg, &buf, strings.NewReader("")); err != nil {
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
	// "открыть" в основы/файлы.rech читает путь относительно рабочего
	// каталога процесса (как и настоящий rech run) - тесты по умолчанию
	// запускаются с рабочим каталогом самого пакета, а не корня репозитория,
	// поэтому здесь он временно переключается на корень репозитория (тот же
	// каталог, откуда "rech run" ожидается запущенным - см. комментарий в
	// самом примере).
	repoRoot := filepath.Dir(root)
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	cases := map[string]string{
		"коллекции/очередь.rech":          "истина\n3\n1\n1\n2\n1\nложь\n",
		"коллекции/список.rech":           "3\n10\n30\n99\nложь\n10\n99\n30\n",
		"коллекции/стопка.rech":           "истина\n3\n3\n3\n2\n1\nложь\n",
		"основы/выражения.rech":           "Привет, Илья!\n13\n7\n30\n3\n-10\n4\nистина\nистина\nложь\nложь\nистина\nложь\nистина\n11\n",
		"основы/файлы.rech":               "привет\n, файл\n\nистина\nпервая строка\nвторая строка\nперезаписано целиком\nи дописано\n\n",
		"основы/побитовые.rech":           "7\n2\n5\n-1\n8\n8\n5\n",
		"основы/инкремент.rech":           "5\n6\n7\n7\n8\n7\n1.5\n2.5\n",
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

// TestRunFileReadPastEOFReturnsEmptyString - открыть/прочитать/закрыть
// (шаг 8, файловый ввод-вывод) через временный файл, не зависящий от
// examples/ (в отличие от TestRunAllExamplesMatchCompiledOutput выше) -
// повторное "прочитать" после конца файла должно возвращать "" каждый раз,
// не панику (в отличие от прочитать_строку/прочитать_целое/прочитать_число
// на конце стандартного ввода).
func TestRunFileReadPastEOFReturnsEmptyString(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "rech-file-*.txt")
	if err != nil {
		t.Fatalf("создать временный файл: %v", err)
	}
	if _, err := f.WriteString("ab"); err != nil {
		t.Fatalf("записать во временный файл: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("закрыть временный файл: %v", err)
	}
	path := strings.ReplaceAll(f.Name(), `\`, `\\`)
	got := runSrc(t, `
функция главная
{
    переменная ф = открыть("`+path+`")
    вывести прочитать(ф, 1)
    вывести прочитать(ф, 10)
    вывести прочитать(ф, 10)
    закрыть(ф)
}
`)
	if got != "a\nb\n\n" {
		t.Fatalf("получили %q, хотели \"a\\nb\\n\\n\"", got)
	}
}

// TestRunFileWriteAppendAndForEachLine - открыть_для_записи (обрезает),
// открыть_для_дозаписи (дописывает в конец) и "для <строка> в <Файл>"
// (потоковое построчное чтение, шаг 8) через один и тот же временный файл.
func TestRunFileWriteAppendAndForEachLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	pathLit := strings.ReplaceAll(path, `\`, `\\`)
	got := runSrc(t, `
функция главная
{
    переменная запись = открыть_для_записи("`+pathLit+`")
    записать(запись, "первая\n")
    закрыть(запись)

    переменная дозапись = открыть_для_дозаписи("`+pathLit+`")
    записать(дозапись, "вторая\n")
    закрыть(дозапись)

    переменная файл = открыть("`+pathLit+`")
    для строка в файле
    {
        вывести строка
    }
    закрыть(файл)
}
`)
	if got != "первая\nвторая\n" {
		t.Fatalf("получили %q, хотели \"первая\\nвторая\\n\"", got)
	}
}

func TestRunWholeFileBuiltins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	pathLit := strings.ReplaceAll(path, `\`, `\\`)
	got := runSrc(t, `
функция главная
{
    записать_в_файл("`+pathLit+`", "первая")
    дописать_в_файл("`+pathLit+`", "\nвторая")
    вывести прочитать_файл("`+pathLit+`")
}
`)
	if got != "первая\nвторая\n" {
		t.Fatalf("получили %q, хотели \"первая\\nвторая\\n\"", got)
	}
}

func TestRunContainsBuiltin(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести содержит("привет мир", "мир")
    вывести содержит("привет мир", "нет")
}
`)
	if got != "истина\nложь\n" {
		t.Fatalf("получили %q, хотели \"истина\\nложь\\n\"", got)
	}
}

// TestRunArgsBuiltinsHaveNoProgramArgs - "rech test" не передаёт
// интерпретируемой программе аргументов - "количество_аргументов" всегда
// 0, "аргумент" любого индекса паникует диапазоном.
func TestRunArgsBuiltinsHaveNoProgramArgs(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести количество_аргументов()
}
`)
	if got != "0\n" {
		t.Fatalf("получили %q, хотели \"0\\n\"", got)
	}

	toks, err := lexer.New(`
функция главная
{
    вывести аргумент(0)
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
	if err == nil || !strings.Contains(err.Error(), "аргумент: индекс 0 вне диапазона") {
		t.Fatalf("ожидалась паника про индекс вне диапазона, получено: %v", err)
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
	err = Run(irProg, &buf, strings.NewReader(""))
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
	err = Run(irProg, &buf, strings.NewReader(""))
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
	err = Run(irProg, &buf, strings.NewReader(""))
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

func TestRunBitwiseOperators(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести 6 | 3
    вывести 6 & 3
    вывести 6 ^ 3
    вывести ~0
    вывести 1 << 3
    вывести 128 >> 4
}
`)
	if got != "7\n2\n5\n-1\n8\n8\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunIncDecExprPrefixAndPostfixValues(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная а = 5
    вывести а++
    вывести а
    вывести ++а
    вывести а
}
`)
	if got != "5\n6\n7\n7\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunIncDecOnFloatVariable(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная а = 1.5
    вывести а++
    вывести а
}
`)
	if got != "1.5\n2.5\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunAndOrSymbolSynonyms(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести истина && ложь
    вывести истина || ложь
}
`)
	if got != "ложь\nистина\n" {
		t.Fatalf("получили %q", got)
	}
}

func runSrcWithInput(t *testing.T, src, input string) string {
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
	if err := Run(irProg, &buf, strings.NewReader(input)); err != nil {
		t.Fatalf("исполнение: %v", err)
	}
	return buf.String()
}

func TestRunMathBuiltins(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести корень(16.0)
    вывести степень(2.0, 10.0)
    вывести округлить(3.6)
    вывести потолок(3.1)
    вывести пол(3.9)
    вывести модуль(-5.5)
}
`)
	want := "4\n1024\n4\n4\n3\n5.5\n"
	if got != want {
		t.Fatalf("получили %q, ожидалось %q", got, want)
	}
}

func TestRunConsoleInputBuiltins(t *testing.T) {
	got := runSrcWithInput(t, `
функция главная
{
    переменная имя = прочитать_строку()
    переменная возраст = прочитать_целое()
    переменная рост = прочитать_число()
    вывести имя
    вывести возраст
    вывести рост
}
`, "Аня\n30\n1.75\n")
	want := "Аня\n30\n1.75\n"
	if got != want {
		t.Fatalf("получили %q, ожидалось %q", got, want)
	}
}

func TestRunConsoleInputInvalidIntFormatPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная х = прочитать_целое()
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
	err = Run(irProg, &buf, strings.NewReader("не число"))
	if err == nil || !strings.Contains(err.Error(), "не является целым числом") {
		t.Fatalf("ожидалась ошибка формата, получено: %v", err)
	}
}

func TestRunConsoleInputEOFPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная х = прочитать_строку()
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
	if err == nil || !strings.Contains(err.Error(), "конец ввода") {
		t.Fatalf("ожидалась ошибка конца ввода, получено: %v", err)
	}
}

// TestRunRejectsExternFunc - Run сам отказывается исполнять программу с
// внешними функциями (semantic.Analyze их производит в IR как обычно, см.
// TestAnalyzeExternFuncBuildsIR в internal/semantic - отказ ради того, что
// у интерпретатора нет способа вызвать произвольный C-символ без настоящей
// динамической линковки, находится здесь, а не в Analyze).
func TestRunRejectsExternFunc(t *testing.T) {
	toks, err := lexer.New(`
внешняя функция abs(n: Целое): Целое
функция главная { вывести abs(0 - 5) }
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
	if err == nil || !strings.Contains(err.Error(), "не поддерживаются интерпретатором") {
		t.Fatalf("ожидалась ошибка про неподдержку внешних функций интерпретатором, получено: %v", err)
	}
}
