package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// buildEchoEnvBinary - настоящий нативный исполняемый файл (не shell-
// скрипт), печатающий значение $RECH_TEST_VAR - runBinary (main.go) в
// продакшене всегда запускает настоящий бинарник, собранный cbackend'ом
// (.exe на Windows), поэтому и тест должен проверять его именно на таком
// файле: ".sh" со сработавшим "#!/bin/sh" на Windows не запускается вовсе
// (CreateProcess не понимает shebang, "%1 is not a valid Win32 application").
// "go build" даёт один и тот же настоящий нативный бинарник на любой ОС без
// платформенных развилок в самом тесте.
func buildEchoEnvBinary(t *testing.T, dir string) string {
	t.Helper()
	srcPath := filepath.Join(dir, "echo_env.go")
	writeTestFile(t, srcPath, `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Getenv("RECH_TEST_VAR"))
}
`)
	binPath := filepath.Join(dir, "echo_env")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", binPath, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build echo_env: %v\n%s", err, out)
	}
	return binPath
}

func TestRunTestsComposerStyleOutput(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "а_тест.rech"), "функция главная { утверждать 1 == 1 }")
	writeTestFile(t, filepath.Join(dir, "б_тест.rech"), "функция главная { утверждать 1 == 2 }")

	var ok bool
	out := captureStdout(t, func() { ok = runTests(dir, 2) })

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
	captureStdout(t, func() { ok = runTests(dir, 2) })
	if !ok {
		t.Fatal("ожидалось true - все тесты прошли")
	}
}

func TestRunTestsNeverAbortsOnFirstFailure(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "а_тест.rech"), "функция главная { утверждать ложь }")
	writeTestFile(t, filepath.Join(dir, "б_тест.rech"), "функция главная { переменная а = 1 / 0 }")
	writeTestFile(t, filepath.Join(dir, "в_тест.rech"), "функция главная { утверждать истина }")

	out := captureStdout(t, func() { runTests(dir, 2) })
	if !strings.Contains(out, "Тестов: 3, провалено: 2") {
		t.Fatalf("ожидалось, что все 3 файла запустятся, несмотря на 2 провала, получено:\n%s", out)
	}
}

func TestParseBuildArgsPlainPath(t *testing.T) {
	a, err := parseBuildArgs([]string{"--keep-c", "прог.rech"})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if a.srcPath != "прог.rech" || a.code != "" || !a.keepC || a.hasCode || a.release {
		t.Fatalf("получено %+v", a)
	}
}

func TestParseBuildArgsLibFlag(t *testing.T) {
	a, err := parseBuildArgs([]string{"--lib", "прог.rech"})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if a.srcPath != "прог.rech" || !a.isLib {
		t.Fatalf("получено %+v", a)
	}
}

func TestParseBuildArgsReleaseFlag(t *testing.T) {
	a, err := parseBuildArgs([]string{"прог.rech", "--release"})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if a.srcPath != "прог.rech" || !a.release {
		t.Fatalf("получено %+v", a)
	}
}

func TestParseBuildArgsCodeFlag(t *testing.T) {
	for _, flag := range []string{"-c", "--code"} {
		a, err := parseBuildArgs([]string{flag, "вывести 1"})
		if err != nil {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
		if !a.hasCode || a.code != "вывести 1" || a.srcPath != "" {
			t.Fatalf("флаг %q: получено %+v", flag, a)
		}
	}
}

func TestParseBuildArgsCodeWithoutValueFails(t *testing.T) {
	_, err := parseBuildArgs([]string{"-c"})
	if err == nil {
		t.Fatal("ожидалась ошибка - после -c нет значения")
	}
}

func TestParseBuildArgsCodeAndPathConflictFails(t *testing.T) {
	_, err := parseBuildArgs([]string{"-c", "вывести 1", "прог.rech"})
	if err == nil {
		t.Fatal("ожидалась ошибка - -c и путь одновременно")
	}
}

// TestRunBinaryPassesEnv - "окружение:" из .rechproj (project.File.
// Окружение) должно реально дойти до процесса, если переменная снаружи не
// задана вовсе.
func TestRunBinaryPassesEnv(t *testing.T) {
	dir := t.TempDir()
	binPath := buildEchoEnvBinary(t, dir)

	os.Unsetenv("RECH_TEST_VAR")
	out := captureStdout(t, func() {
		if err := runBinary(binPath, map[string]string{"RECH_TEST_VAR": "из_манифеста"}, nil); err != nil {
			t.Fatalf("runBinary: %v", err)
		}
	})
	if strings.TrimSpace(out) != "из_манифеста" {
		t.Fatalf("получено %q, ожидалось значение из .rechproj", out)
	}
}

// TestRunBinaryExternalEnvOverridesManifest - НАЙДЕННОЕ И ИСПРАВЛЕННОЕ (по
// ходу обсуждения, до первого релиза фичи) поведение: "окружение:" из
// .rechproj - значение ПО УМОЛЧАНИЮ, не безусловное - уже заданная снаружи
// переменная (Docker "-e"/docker-compose/CI/шелл) должна побеждать, иначе
// задать порт контейнеру извне стало бы невозможно.
func TestRunBinaryExternalEnvOverridesManifest(t *testing.T) {
	dir := t.TempDir()
	binPath := buildEchoEnvBinary(t, dir)

	t.Setenv("RECH_TEST_VAR", "снаружи")
	out := captureStdout(t, func() {
		if err := runBinary(binPath, map[string]string{"RECH_TEST_VAR": "из_манифеста"}, nil); err != nil {
			t.Fatalf("runBinary: %v", err)
		}
	})
	if strings.TrimSpace(out) != "снаружи" {
		t.Fatalf("получено %q, внешнее окружение должно побеждать значение из .rechproj", out)
	}
}

// "выйти()" в тесте завершает только свой рабочий процесс: тест провален,
// остальные исполняются.
func TestRunTestsExitInTestFailsOnlyThatTest(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "а_тест.rech"), "функция главная { выйти(3) }")
	writeTestFile(t, filepath.Join(dir, "б_тест.rech"), "функция главная { утверждать истина }")

	out := captureStdout(t, func() { runTests(dir, 1) })
	if !strings.Contains(out, "Тестов: 2, провалено: 1") || !strings.Contains(out, "код выхода 3") {
		t.Fatalf("получено:\n%s", out)
	}
}

func TestParseTestArgs(t *testing.T) {
	if p, j, err := parseTestArgs([]string{"-j", "3", "тесты"}); err != nil || p != "тесты" || j != 3 {
		t.Fatalf("получено %q %d %v", p, j, err)
	}
	if _, j, _ := parseTestArgs([]string{"тесты"}); j < 1 {
		t.Fatalf("по умолчанию - число ядер, получено %d", j)
	}
	for _, bad := range [][]string{{"-j", "0", "т"}, {"-j"}, {}, {"а", "б"}} {
		if _, _, err := parseTestArgs(bad); err == nil {
			t.Errorf("%q: ожидалась ошибка", bad)
		}
	}
}
