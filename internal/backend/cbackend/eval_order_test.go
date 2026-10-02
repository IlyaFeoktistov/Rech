package cbackend

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedProgramEvaluatesLeftToRight - аргументы вызова, вставки
// "${...}" и операнды бинарного оператора вычисляются слева направо, как
// в interp, а не в неопределённом порядке C.
func TestGeneratedProgramEvaluatesLeftToRight(t *testing.T) {
	out := generateFromSrc(t, `Целое счётчик = 0

функция шаг(): Целое
{
    счётчик++
    вернуть счётчик
}

функция пара(а: Целое, б: Целое): Строка
{
    вернуть `+"`${а},${б}`"+`
}

функция главная
{
    вывести `+"`${шаг()} ${счётчик} ${пара(шаг(), шаг())} ${счётчик - шаг()}\\n`"+`
}
`)
	ccPath := findTestCCompiler(t)
	dir := t.TempDir()
	cPath, binPath := filepath.Join(dir, "order.c"), filepath.Join(dir, "order")
	if err := os.WriteFile(cPath, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- ccPath найден локальным поиском (CC/cc/clang/gcc), пути - файлы этого теста.
	if msg, err := exec.Command(ccPath, cPath, "-o", binPath, "-lgc", "-lm").CombinedOutput(); err != nil {
		t.Fatalf("не собирается: %v\n%s", err, msg)
	}
	// #nosec G204 -- binPath только что собран этим тестом.
	got, err := exec.Command(binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("запуск: %v\n%s", err, got)
	}
	if want := "1 1 2,3 -1"; strings.TrimSpace(string(got)) != want {
		t.Fatalf("получено %q, ожидалось %q", strings.TrimSpace(string(got)), want)
	}
}

// TestGeneratedProgramEvaluatesReceiverOnce - получатель метода
// интерфейса - результат вызова - вычисляется один раз и раньше
// аргументов (диспетчеризация раньше вычисляла его дважды).
func TestGeneratedProgramEvaluatesReceiverOnce(t *testing.T) {
	out := generateFromSrc(t, `Целое создано = 0

интерфейс Фигура
{
    площадь(множитель: Целое): Целое
}

сущность Квадрат реализует Фигуру
{
    имеет:
        Целое сторона

    конструктор(сторона: Целое)
    {
        этот.сторона = сторона
    }

    публичный площадь(множитель: Целое): Целое
    {
        вернуть этот.сторона * множитель
    }
}

функция создать(): Фигура
{
    создано++
    вернуть новый Квадрат(создано)
}

функция главная
{
    Целое р = создать().площадь(создано)
    вывести `+"`${р} ${создано}\\n`"+`
}
`)
	if got := runGeneratedC(t, out); got != "1 1" {
		t.Fatalf("получено %q, ожидалось \"1 1\"", got)
	}
}

// runGeneratedC - собирает и запускает сгенерированный C, возвращает вывод
// без пробелов по краям.
func runGeneratedC(t *testing.T, src string) string {
	t.Helper()
	ccPath := findTestCCompiler(t)
	dir := t.TempDir()
	cPath, binPath := filepath.Join(dir, "prog.c"), filepath.Join(dir, "prog")
	if err := os.WriteFile(cPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- ccPath найден локальным поиском (CC/cc/clang/gcc), пути - файлы этого теста.
	if msg, err := exec.Command(ccPath, cPath, "-o", binPath, "-lgc", "-lm").CombinedOutput(); err != nil {
		t.Fatalf("не собирается: %v\n%s", err, msg)
	}
	// #nosec G204 -- binPath только что собран этим тестом.
	got, err := exec.Command(binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("запуск: %v\n%s", err, got)
	}
	return strings.TrimSpace(string(got))
}
