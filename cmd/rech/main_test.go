package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout перехватывает os.Stdout на время вызова f - runTests
// печатает прогресс/сводку напрямую в os.Stdout, а не принимает io.Writer,
// так что для проверки вывода приходится подменить сам os.Stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(out)
}

func writeTestFile(t *testing.T, path, src string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestRunTestsComposerStyleOutput(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "а_тест.rech"), "функция главная { утверждать 1 == 1 }")
	writeTestFile(t, filepath.Join(dir, "б_тест.rech"), "функция главная { утверждать 1 == 2 }")

	var ok bool
	out := captureStdout(t, func() { ok = runTests(dir) })

	if ok {
		t.Fatal("ожидался хотя бы один провал - runTests должна вернуть false")
	}
	if !strings.Contains(out, ".") || !strings.Contains(out, "F") {
		t.Fatalf("ожидались точки прогресса \".\"/\"F\", получено:\n%s", out)
	}
	if !strings.Contains(out, "Провалившиеся тесты:") {
		t.Fatalf("ожидался список провалов, получено:\n%s", out)
	}
	if !strings.Contains(out, "Тестов: 2, провалено: 1") {
		t.Fatalf("ожидалась итоговая сводка, получено:\n%s", out)
	}
}

func TestRunTestsAllPassReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "а_тест.rech"), "функция главная { утверждать 1 == 1 }")

	var ok bool
	captureStdout(t, func() { ok = runTests(dir) })
	if !ok {
		t.Fatal("ожидалось true - все тесты прошли")
	}
}

func TestRunTestsNeverAbortsOnFirstFailure(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "а_тест.rech"), "функция главная { утверждать ложь }")
	writeTestFile(t, filepath.Join(dir, "б_тест.rech"), "функция главная { переменная а = 1 / 0 }")
	writeTestFile(t, filepath.Join(dir, "в_тест.rech"), "функция главная { утверждать истина }")

	out := captureStdout(t, func() { runTests(dir) })
	if !strings.Contains(out, "Тестов: 3, провалено: 2") {
		t.Fatalf("ожидалось, что все 3 файла запустятся, несмотря на 2 провала, получено:\n%s", out)
	}
}

func TestParseBuildArgsPlainPath(t *testing.T) {
	src, code, keepC, hasCode, err := parseBuildArgs([]string{"--keep-c", "прог.rech"})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if src != "прог.rech" || code != "" || !keepC || hasCode {
		t.Fatalf("получено src=%q code=%q keepC=%v hasCode=%v", src, code, keepC, hasCode)
	}
}

func TestParseBuildArgsCodeFlag(t *testing.T) {
	for _, flag := range []string{"-c", "--code"} {
		src, code, _, hasCode, err := parseBuildArgs([]string{flag, "вывести 1"})
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if !hasCode || code != "вывести 1" || src != "" {
			t.Fatalf("флаг %q: получено src=%q code=%q hasCode=%v", flag, src, code, hasCode)
		}
	}
}

func TestParseBuildArgsCodeWithoutValueFails(t *testing.T) {
	_, _, _, _, err := parseBuildArgs([]string{"-c"})
	if err == nil {
		t.Fatal("ожидалась ошибка - после -c нет значения")
	}
}

func TestParseBuildArgsCodeAndPathConflictFails(t *testing.T) {
	_, _, _, _, err := parseBuildArgs([]string{"-c", "вывести 1", "прог.rech"})
	if err == nil {
		t.Fatal("ожидалась ошибка - -c и путь одновременно")
	}
}
