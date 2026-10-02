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
