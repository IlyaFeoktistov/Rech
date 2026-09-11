package pkgfetch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// TestResolveGraphLibraryPathDefaultsToDependencyDir - зависимость со
// "сборка.библиотеки" без "путь_библиотек" - LibraryPaths получает
// СОБСТВЕННЫЙ каталог зависимости (dir) как разумное умолчание, без
// единой строчки в манифесте потребителя (закрытый (бинарный) Rech-пакет,
// кладущий .so рядом со своим *.rechproj - docs/SPEC.md).
func TestResolveGraphLibraryPathDefaultsToDependencyDir(t *testing.T) {
	repoA := makeGitRepoWithFiles(t, map[string]string{
		"тест.rechproj": "проект:\n  имя: repoA\nфайлы:\n  главный: главный.rech\nсборка:\n  библиотеки: точка\n",
		"главный.rech":  "функция ф() { вывести 1 }\n",
	})

	rootDir := t.TempDir()
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(rootDir, ".rechlock")
	rootDeps := map[string]project.Dependency{
		"точка": {URL: repoA, Версия: "v1.0.0"},
	}

	g, err := ResolveGraphWithRoot(rootDeps, cacheRoot, rootDir, lockPath, nil)
	if err != nil {
		t.Fatalf("ResolveGraph: %v", err)
	}
	depDir, ok := g.Scopes[rootDir]["точка"]
	if !ok {
		t.Fatalf("ожидалась зависимость \"точка\", получено %+v", g.Scopes[rootDir])
	}
	if len(g.LibraryPaths) != 1 || g.LibraryPaths[0] != depDir {
		t.Fatalf("ожидался LibraryPaths == [%q] (собственный каталог зависимости), получено %v", depDir, g.LibraryPaths)
	}
}

// TestResolveGraphLibraryPathHonorsDependencyOwnPath - зависимость,
// указавшая СВОЙ "сборка.путь_библиотек" (например, платформенный
// подкаталог с готовым бинарником) - LibraryPaths несёт этот путь, не
// каталог самого *.rechproj.
func TestResolveGraphLibraryPathHonorsDependencyOwnPath(t *testing.T) {
	repoA := makeGitRepoWithFiles(t, map[string]string{
		"тест.rechproj": "проект:\n  имя: repoA\nфайлы:\n  главный: главный.rech\nсборка:\n  библиотеки: точка\n  путь_библиотек: сборка/linux-amd64\n",
		"главный.rech":  "функция ф() { вывести 1 }\n",
	})

	rootDir := t.TempDir()
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(rootDir, ".rechlock")
	rootDeps := map[string]project.Dependency{
		"точка": {URL: repoA, Версия: "v1.0.0"},
	}

	g, err := ResolveGraphWithRoot(rootDeps, cacheRoot, rootDir, lockPath, nil)
	if err != nil {
		t.Fatalf("ResolveGraph: %v", err)
	}
	depDir := g.Scopes[rootDir]["точка"]
	want := filepath.Join(depDir, "сборка", "linux-amd64")
	if len(g.LibraryPaths) != 1 || g.LibraryPaths[0] != want {
		t.Fatalf("ожидался LibraryPaths == [%q], получено %v", want, g.LibraryPaths)
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

// TestResolveGraphMaterializesRootDependencyIntoProject - зависимость
// корневого уровня физически копируется из кеша ОС в
// "<rootDir>/библиотеки/<имя>/" ("Закрытые (бинарные)
// пакеты", доводка про материализацию) - файлы реально доступны внутри
// дерева проекта, не только в общем кеше вне его, а g.Scopes указывает на
// новое место (см. package doc MaterializeRoot).
func TestResolveGraphMaterializesRootDependencyIntoProject(t *testing.T) {
	repoA := makeGitRepoWithFiles(t, map[string]string{
		"тест.rechproj": "проект:\n  имя: repoA\nфайлы:\n  главный: главный.rech\n",
		"главный.rech":  "функция ф() { вывести 1 }\n",
	})

	rootDir := t.TempDir()
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(rootDir, ".rechlock")
	rootDeps := map[string]project.Dependency{
		"точка": {URL: repoA, Версия: "v1.0.0"},
	}

	g, err := ResolveGraphWithRoot(rootDeps, cacheRoot, rootDir, lockPath, nil)
	if err != nil {
		t.Fatalf("ResolveGraph: %v", err)
	}

	wantDir := filepath.Join(rootDir, "библиотеки", "точка")
	depDir, ok := g.Scopes[rootDir]["точка"]
	if !ok || depDir != wantDir {
		t.Fatalf("ожидался Scopes[rootDir][\"точка\"] == %q, получено %q (ok=%v)", wantDir, depDir, ok)
	}
	data, err := os.ReadFile(filepath.Join(wantDir, "главный.rech"))
	if err != nil {
		t.Fatalf("зависимость не материализована в %q: %v", wantDir, err)
	}
	if !strings.Contains(string(data), "вывести 1") {
		t.Fatalf("неверное содержимое материализованного файла: %s", data)
	}
	if _, err := os.Stat(filepath.Join(wantDir, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git не должен копироваться в материализованную зависимость")
	}
}

// TestResolveGraphMaterializeSkipsRecopyOnUnchangedCommit - повторный вызов
// с тем же (закреплённым в .rechlock) коммитом не перезаписывает уже
// материализованную зависимость - дорогое копирование не должно повторяться
// на каждый "rech build/run" без надобности.
func TestResolveGraphMaterializeSkipsRecopyOnUnchangedCommit(t *testing.T) {
	repoA := makeGitRepoWithFiles(t, map[string]string{
		"тест.rechproj": "проект:\n  имя: repoA\nфайлы:\n  главный: главный.rech\n",
		"главный.rech":  "функция ф() { вывести 1 }\n",
	})

	rootDir := t.TempDir()
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(rootDir, ".rechlock")
	rootDeps := map[string]project.Dependency{
		"точка": {URL: repoA, Версия: "v1.0.0"},
	}

	if _, err := ResolveGraphWithRoot(rootDeps, cacheRoot, rootDir, lockPath, nil); err != nil {
		t.Fatalf("ResolveGraph (первый раз): %v", err)
	}
	wantDir := filepath.Join(rootDir, "библиотеки", "точка")
	sentinel := filepath.Join(wantDir, "метка.txt")
	if err := os.WriteFile(sentinel, []byte("не должно исчезнуть"), 0o644); err != nil {
		t.Fatalf("запись метки: %v", err)
	}

	if _, err := ResolveGraphWithRoot(rootDeps, cacheRoot, rootDir, lockPath, nil); err != nil {
		t.Fatalf("ResolveGraph (второй раз): %v", err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("метка исчезла - зависимость была перекопирована, хотя коммит не менялся: %v", err)
	}
}

// TestResolveGraphLibraryPathAutoSelectsConsumerPlatform - зависимость без
// собственного "путь_библиотек", но с подкаталогом "сборка/<GOOS>-<GOARCH>/"
// ПОТРЕБИТЕЛЯ - LibraryPaths выбирает ИМЕННО его, не голый каталог
// зависимости (авто-выбор платформы).
func TestResolveGraphLibraryPathAutoSelectsConsumerPlatform(t *testing.T) {
	platformDir := runtime.GOOS + "-" + runtime.GOARCH
	repoA := makeGitRepoWithFiles(t, map[string]string{
		"тест.rechproj": "проект:\n  имя: repoA\nфайлы:\n  главный: главный.rech\nсборка:\n  библиотеки: точка\n",
		"главный.rech":  "функция ф() { вывести 1 }\n",
		"сборка/" + platformDir + "/маркер.txt": "бинарник для текущей платформы\n",
	})

	rootDir := t.TempDir()
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(rootDir, ".rechlock")
	rootDeps := map[string]project.Dependency{
		"точка": {URL: repoA, Версия: "v1.0.0"},
	}

	g, err := ResolveGraphWithRoot(rootDeps, cacheRoot, rootDir, lockPath, nil)
	if err != nil {
		t.Fatalf("ResolveGraph: %v", err)
	}
	depDir := g.Scopes[rootDir]["точка"]
	want := filepath.Join(depDir, "сборка", platformDir)
	if len(g.LibraryPaths) != 1 || g.LibraryPaths[0] != want {
		t.Fatalf("ожидался LibraryPaths == [%q] (платформенный подкаталог), получено %v", want, g.LibraryPaths)
	}
}

// TestResolveGraphLibraryPathIgnoresOtherPlatformDir - подкаталог под
// ЧУЖУЮ платформу не подходит - умолчание остаётся голым каталогом
// зависимости, как для однoплатформенного пакета.
func TestResolveGraphLibraryPathIgnoresOtherPlatformDir(t *testing.T) {
	repoA := makeGitRepoWithFiles(t, map[string]string{
		"тест.rechproj": "проект:\n  имя: repoA\nфайлы:\n  главный: главный.rech\nсборка:\n  библиотеки: точка\n",
		"главный.rech":  "функция ф() { вывести 1 }\n",
		"сборка/совсем-не-такая-платформа/маркер.txt": "чужая платформа\n",
	})

	rootDir := t.TempDir()
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(rootDir, ".rechlock")
	rootDeps := map[string]project.Dependency{
		"точка": {URL: repoA, Версия: "v1.0.0"},
	}

	g, err := ResolveGraphWithRoot(rootDeps, cacheRoot, rootDir, lockPath, nil)
	if err != nil {
		t.Fatalf("ResolveGraph: %v", err)
	}
	depDir := g.Scopes[rootDir]["точка"]
	if len(g.LibraryPaths) != 1 || g.LibraryPaths[0] != depDir {
		t.Fatalf("ожидался LibraryPaths == [%q] (голый каталог зависимости), получено %v", depDir, g.LibraryPaths)
	}
}
