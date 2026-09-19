package testrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestDiscoverFindsSuffixedFilesRecursively(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "а_тест.rech"), "функция главная { утверждать истина }")
	write(t, filepath.Join(dir, "подпапка", "б_тест.rech"), "функция главная { утверждать истина }")
	write(t, filepath.Join(dir, "обычный.rech"), "функция главная { вывести \"не тест\" }")
	write(t, filepath.Join(dir, ".git", "в_тест.rech"), "функция главная { утверждать истина }")

	files, err := Discover(dir)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("ожидалось 2 файла, получено %d: %v", len(files), files)
	}
}

// TestDiscoverFindsCyrillicExtensionSynonyms - "_тест.речь"/"_тест.р"
// узнаются наравне с "_тест.rech" (module.SourceExtensions).
func TestDiscoverFindsCyrillicExtensionSynonyms(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "а_тест.речь"), "функция главная { утверждать истина }")
	write(t, filepath.Join(dir, "б_тест.р"), "функция главная { утверждать истина }")
	write(t, filepath.Join(dir, "обычный.речь"), "функция главная { вывести \"не тест\" }")

	files, err := Discover(dir)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("ожидалось 2 файла, получено %d: %v", len(files), files)
	}
}

func TestDiscoverSingleFileIgnoresNamingConvention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "любое_имя.rech")
	write(t, path, "функция главная { утверждать истина }")

	files, err := Discover(path)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(files) != 1 || files[0] != path {
		t.Fatalf("ожидался ровно этот файл, получено %v", files)
	}
}

func TestRunFilePassAndFail(t *testing.T) {
	dir := t.TempDir()
	okPath := filepath.Join(dir, "ок_тест.rech")
	write(t, okPath, `
функция главная
{
    утверждать 1 + 1 == 2
}
`)
	failPath := filepath.Join(dir, "провал_тест.rech")
	write(t, failPath, `
функция главная
{
    утверждать 1 == 2
}
`)

	okResult := RunFile(okPath)
	if !okResult.Passed || okResult.Err != nil {
		t.Fatalf("ожидался пройденный тест, получено %+v", okResult)
	}

	failResult := RunFile(failPath)
	if failResult.Passed || failResult.Err == nil {
		t.Fatalf("ожидался непройденный тест, получено %+v", failResult)
	}
}

func TestRunFileCompileErrorCountsAsFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "битый_тест.rech")
	write(t, path, "функция главная { утверждать }")

	result := RunFile(path)
	if result.Passed || result.Err == nil {
		t.Fatalf("ожидалась ошибка компиляции как провал теста, получено %+v", result)
	}
}

func TestRunAcrossDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "а_тест.rech"), "функция главная { утверждать истина }")
	write(t, filepath.Join(dir, "б_тест.rech"), "функция главная { утверждать ложь }")

	results, err := Run(dir)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("ожидалось 2 результата, получено %d", len(results))
	}
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	if passed != 1 {
		t.Fatalf("ожидался ровно один пройденный тест, получено %d", passed)
	}
}
