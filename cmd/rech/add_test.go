package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/project"
)

func TestInsertDependencyLineNoSection(t *testing.T) {
	text := "проект:\n  имя: тест\n\nфайлы:\n  главный: главный.rech\n"
	got := insertDependencyLine(text, "внешняя", "https://example.com/repo", "v1.0.0")
	want := text + "\nзависимости:\n  внешняя: https://example.com/repo v1.0.0\n"
	if got != want {
		t.Fatalf("insertDependencyLine() =\n%q\nхотели:\n%q", got, want)
	}
}

func TestInsertDependencyLineCommentedTemplateIgnored(t *testing.T) {
	text := "проект:\n  имя: тест\n\n# зависимости:\n#   имя: url версия\n"
	got := insertDependencyLine(text, "внешняя", "https://example.com/repo", "v1.0.0")
	if !strings.HasSuffix(got, "\nзависимости:\n  внешняя: https://example.com/repo v1.0.0\n") {
		t.Fatalf("закомментированная секция template была ошибочно принята за активную: %q", got)
	}
	if strings.Count(got, "зависимости:") != 2 { // "# зависимости:" + новая настоящая секция
		t.Fatalf("ожидалось ровно 2 вхождения (комментарий + новая секция): %q", got)
	}
}

func TestInsertDependencyLineAppendsAfterExisting(t *testing.T) {
	text := "проект:\n  имя: тест\n\nзависимости:\n  первая: https://example.com/a v1.0.0\n\nсборка:\n  режим: отладка\n"
	got := insertDependencyLine(text, "вторая", "https://example.com/b", "main")
	want := "проект:\n  имя: тест\n\nзависимости:\n  первая: https://example.com/a v1.0.0\n  вторая: https://example.com/b main\n\nсборка:\n  режим: отладка\n"
	if got != want {
		t.Fatalf("insertDependencyLine() =\n%q\nхотели:\n%q", got, want)
	}
}

func TestValidateDepName(t *testing.T) {
	if err := validateDepName(""); err == nil {
		t.Fatalf("пустое имя должно быть ошибкой")
	}
	if err := validateDepName("с пробелом"); err == nil {
		t.Fatalf("имя с пробелом должно быть ошибкой")
	}
	if err := validateDepName("плохое:имя"); err == nil {
		t.Fatalf("имя с \":\" должно быть ошибкой")
	}
	if err := validateDepName("внешняя"); err != nil {
		t.Fatalf("обычное имя не должно быть ошибкой: %v", err)
	}
}

// TestAddDependencyEndToEnd - "rech add" на реальном (локальном, без сети)
// git-репозитории: дописывает секцию "зависимости:" в .rechproj, скачивает
// зависимость и пишет lock-файл - тем же приёмом, что и
// TestInstallAndRunResolveExternalDependency (cmd/rech/deps_test.go).
func TestAddDependencyEndToEnd(t *testing.T) {
	depRepo := makeDepRepo(t)

	projDir := t.TempDir()
	writeTestFile(t, filepath.Join(projDir, "тест.rechproj"), "проект:\n  имя: тест\n\nфайлы:\n  главный: главный.rech\n")
	writeTestFile(t, filepath.Join(projDir, "главный.rech"), "функция главная\n{\n}\n")

	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(projDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	if err := addDependency("внешняя", depRepo, "v1.0.0"); err != nil {
		t.Fatalf("addDependency: %v", err)
	}

	pf, err := project.Load(filepath.Join(projDir, "тест.rechproj"))
	if err != nil {
		t.Fatalf("project.Load после \"rech add\": %v", err)
	}
	dep, ok := pf.Зависимости["внешняя"]
	if !ok {
		t.Fatalf("зависимость \"внешняя\" не попала в .rechproj: %+v", pf.Зависимости)
	}
	if dep.URL != depRepo || dep.Версия != "v1.0.0" {
		t.Fatalf("зависимость = %+v, ожидалось URL=%q Версия=v1.0.0", dep, depRepo)
	}

	if _, err := os.Stat(filepath.Join(projDir, ".rechlock")); err != nil {
		t.Fatalf("ожидался lock-файл после \"rech add\": %v", err)
	}

	if err := addDependency("внешняя", depRepo, "v1.0.0"); err == nil {
		t.Fatalf("повторное добавление той же зависимости должно быть ошибкой")
	}
}
