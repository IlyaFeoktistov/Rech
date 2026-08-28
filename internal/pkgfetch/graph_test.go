package pkgfetch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// makeGitRepoWithFiles - как makeTestRepo (pkgfetch_test.go), но с
// произвольным набором файлов (для .rechproj/.rech зависимости) и без
// фиксированного имени файла-маркера.
func makeGitRepoWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeFileHelper(t, dir, name, content)
	}
	runGit(t, dir, "init", "--quiet", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "--quiet", "-m", "первый коммит")
	runGit(t, dir, "tag", "v1.0.0")
	return dir
}

func writeFileHelper(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir для %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("запись файла %q: %v", path, err)
	}
}

// TestResolveGraphTransitiveDependencyAndLibraries - зависимость (repoA) со
// СВОИМ *.rechproj, объявляющим и собственную зависимость ("внутр" ->
// repoB), и сборка.библиотеки - обе транзитивно попадают в граф: Scopes
// получает ОТДЕЛЬНУЮ запись для каталога repoA (со своим именем "внутр"),
// Libraries несёт библиотеку repoA.
func TestResolveGraphTransitiveDependencyAndLibraries(t *testing.T) {
	repoB := makeGitRepoWithFiles(t, map[string]string{
		"файл.rech": "функция изB(): Целое\n{\n    вернуть 7\n}\n",
	})
	repoA := makeGitRepoWithFiles(t, map[string]string{
		"тест.rechproj": "проект:\n  имя: repoA\nфайлы:\n  главный: главный.rech\nсборка:\n  библиотеки: pq\nзависимости:\n  внутр: " + repoB + " v1.0.0\n",
		"главный.rech":  "использовать изB из \"внутр/файл\"\n\nфункция главная\n{\n    вывести изB()\n}\n",
	})

	rootDir := t.TempDir()
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(rootDir, ".rechlock")
	rootDeps := map[string]project.Dependency{
		"клиент": {URL: repoA, Версия: "v1.0.0"},
	}

	g, err := ResolveGraphWithRoot(rootDeps, cacheRoot, rootDir, lockPath, nil)
	if err != nil {
		t.Fatalf("ResolveGraph: %v", err)
	}
	repoADir, ok := g.Scopes[rootDir]["клиент"]
	if !ok {
		t.Fatalf("ожидалась зависимость \"клиент\" в корневой области, получено %+v", g.Scopes[rootDir])
	}
	repoBDir, ok := g.Scopes[repoADir]["внутр"]
	if !ok {
		t.Fatalf("ожидалась транзитивная зависимость \"внутр\" в области %q, получено %+v", repoADir, g.Scopes[repoADir])
	}
	if repoBDir == "" {
		t.Fatalf("пустой каталог транзитивной зависимости")
	}
	if len(g.Libraries) != 1 || g.Libraries[0] != "pq" {
		t.Fatalf("ожидалась библиотека \"pq\" из .rechproj зависимости, получено %v", g.Libraries)
	}
}

// TestResolveGraphCyclicDependencyRejected - repoX зависит от repoY, repoY
// зависит от repoX - явная ошибка, не бесконечная рекурсия.
func TestResolveGraphCyclicDependencyRejected(t *testing.T) {
	// URL заранее не известны (репозитории создаются каждый в своём
	// временном каталоге) - создаём оба, потом переписываем .rechproj с
	// реальными путями и коммитим ещё раз.
	repoX := makeGitRepoWithFiles(t, map[string]string{
		"тест.rechproj": "проект:\n  имя: x\nфайлы:\n  главный: а.rech\n",
		"а.rech":        "функция а() { вывести 1 }\n",
	})
	repoY := makeGitRepoWithFiles(t, map[string]string{
		"тест.rechproj": "проект:\n  имя: y\nфайлы:\n  главный: б.rech\nзависимости:\n  x: " + repoX + " v1.0.0\n",
		"б.rech":        "функция б() { вывести 2 }\n",
	})
	writeFileHelper(t, repoX, "тест.rechproj", "проект:\n  имя: x\nфайлы:\n  главный: а.rech\nзависимости:\n  y: "+repoY+" v1.0.0\n")
	runGit(t, repoX, "add", ".")
	runGit(t, repoX, "commit", "--quiet", "-m", "добавить цикл")
	runGit(t, repoX, "tag", "-f", "v1.0.0")

	rootDir := t.TempDir()
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(rootDir, ".rechlock")
	rootDeps := map[string]project.Dependency{
		"x": {URL: repoX, Версия: "v1.0.0"},
	}
	if _, err := ResolveGraphWithRoot(rootDeps, cacheRoot, rootDir, lockPath, nil); err == nil {
		t.Fatalf("ожидалась ошибка циклической зависимости")
	}
}
