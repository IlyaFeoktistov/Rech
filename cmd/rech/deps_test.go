package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/interp"
	"github.com/IlyaFeoktistov/Rech/internal/project"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, buf.String())
	}
}

// makeDepRepo - локальный git-репозиторий с одной функцией, тегированный
// "v1.0.0" - играет роль внешней зависимости (git clone принимает обычный
// файловый путь без сети, тест не обращается в интернет).
func makeDepRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--quiet", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	writeTestFile(t, filepath.Join(dir, "математика.rech"), "функция удвоить(x: Целое): Целое\n{\n    вернуть x * 2\n}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "--quiet", "-m", "первый коммит")
	runGit(t, dir, "tag", "v1.0.0")
	return dir
}

// TestInstallAndRunResolveExternalDependency - "rech install" скачивает
// объявленную в .rechproj зависимость и пишет lock-файл, "rech run" (без
// аргументов, через тот же .rechproj) использует уже скачанную зависимость
// через "использовать ... из "<имя>/<путь>"" - сквозная проверка всего пути
// от .rechproj до реального исполнения собранной программы.
func TestInstallAndRunResolveExternalDependency(t *testing.T) {
	depRepo := makeDepRepo(t)

	projDir := t.TempDir()
	writeTestFile(t, filepath.Join(projDir, "тест.rechproj"), "проект:\n  имя: тест\nфайлы:\n  главный: главный.rech\nзависимости:\n  внешняя: "+depRepo+" v1.0.0\n")
	writeTestFile(t, filepath.Join(projDir, "главный.rech"), "использовать удвоить из \"внешняя/математика\"\n\nфункция главная\n{\n    вывести удвоить(21)\n}\n")

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

	if err := installDeps(); err != nil {
		t.Fatalf("installDeps: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projDir, ".rechlock")); err != nil {
		t.Fatalf("ожидался lock-файл: %v", err)
	}

	// Дальше - тот же путь, что и "rech run" без аргументов в main() до
	// самой C-кодогенерации: найти/загрузить .rechproj, разрешить
	// зависимости, разобрать программу (module.LoadWithDeps внутри
	// ParseSource) - исполняется через internal/interp, не настоящий cc
	// (тесты этого репозитория от него не зависят, см. internal/compiler).
	projPath, err := project.Find(".")
	if err != nil || projPath == "" {
		t.Fatalf("project.Find: %v (path=%q)", err, projPath)
	}
	pf, err := project.Load(projPath)
	if err != nil {
		t.Fatalf("project.Load: %v", err)
	}
	deps, scopes, _, err := resolveDeps(pf)
	if err != nil {
		t.Fatalf("resolveDeps: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("ожидалась 1 разрешённая зависимость, получено %+v", deps)
	}

	comp := InitializeCompiler()
	comp.Deps = deps
	comp.DepScopes = scopes
	prog, _, err := comp.ParseSource(pf.MainPath())
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		t.Fatalf("semantic.Analyze: %v", err)
	}
	var out bytes.Buffer
	if err := interp.Run(irProg, &out, strings.NewReader("")); err != nil {
		t.Fatalf("interp.Run: %v", err)
	}
	if got := out.String(); got != "42\n" {
		t.Fatalf("вывод программы = %q, ожидалось \"42\\n\"", got)
	}
}
