package pkgfetch

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/project"
)

func TestIsCommitHash(t *testing.T) {
	cases := map[string]bool{
		"a1b2c3d": true,
		"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2": true,
		"v1.2.0":    false,
		"main":      false,
		"feature/x": false,
	}
	for ref, want := range cases {
		if got := isCommitHash(ref); got != want {
			t.Fatalf("isCommitHash(%q) = %v, ожидалось %v", ref, got, want)
		}
	}
}

func TestCacheDirFor(t *testing.T) {
	got, err := cacheDirFor("/кеш", "https://github.com/user/repo", "v1.2.0")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := filepath.Join("/кеш", "github.com/user/repo@v1.2.0")
	if got != want {
		t.Fatalf("cacheDirFor = %q, ожидалось %q", got, want)
	}
}

func TestLockFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".rechlock")
	entries := map[string]LockEntry{
		"клиент": {URL: "https://github.com/user/repo", Версия: "v1.2.0", Commit: "abc123"},
	}
	if err := writeLock(path, entries); err != nil {
		t.Fatalf("writeLock: %v", err)
	}
	got, err := readLock(path)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if got["клиент"] != entries["клиент"] {
		t.Fatalf("readLock() = %+v, ожидалось %+v", got, entries)
	}
}

func TestReadLockMissingFileReturnsEmpty(t *testing.T) {
	got, err := readLock(filepath.Join(t.TempDir(), "нет-такого-файла"))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ожидалась пустая карта, получено %+v", got)
	}
}

// runGit - выполнить git-команду в dir, для подготовки тестового
// репозитория (без обращения к сети - только локальные операции).
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

// makeTestRepo создаёт локальный git-репозиторий с файлом "маркер.rech" на
// ветке main и тегом "v1.0.0" - используется как "URL" зависимости
// (git clone принимает обычный файловый путь без сети).
func makeTestRepo(t *testing.T) (repoPath string) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--quiet", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "маркер.rech"), []byte("функция главная { вывести \"привет\" }\n"), 0o644); err != nil {
		t.Fatalf("запись файла: %v", err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "--quiet", "-m", "первый коммит")
	runGit(t, dir, "tag", "v1.0.0")
	return dir
}

func TestResolveClonesAndWritesLock(t *testing.T) {
	repo := makeTestRepo(t)
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), ".rechlock")

	deps := map[string]project.Dependency{
		"клиент": {URL: repo, Версия: "v1.0.0"},
	}
	dirs, err := ResolveWithRoot(deps, cacheRoot, lockPath, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	depDir, ok := dirs["клиент"]
	if !ok {
		t.Fatalf("ожидался каталог для зависимости \"клиент\", получено %+v", dirs)
	}
	if _, err := os.Stat(filepath.Join(depDir, "маркер.rech")); err != nil {
		t.Fatalf("ожидался файл маркер.rech в скачанной зависимости: %v", err)
	}
	lock, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	entry, ok := lock["клиент"]
	if !ok || entry.URL != repo || entry.Версия != "v1.0.0" || entry.Commit == "" {
		t.Fatalf("ожидалась заполненная lock-запись, получено %+v", lock)
	}
}

func TestResolveReusesCacheWithoutTouchingSource(t *testing.T) {
	repo := makeTestRepo(t)
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), ".rechlock")
	deps := map[string]project.Dependency{
		"клиент": {URL: repo, Версия: "v1.0.0"},
	}
	if _, err := ResolveWithRoot(deps, cacheRoot, lockPath, nil); err != nil {
		t.Fatalf("первое Resolve: %v", err)
	}
	// Источник "исчез" (репозиторий больше не существует) - повторный
	// Resolve с ТЕМИ ЖЕ URL/версией из .rechproj и совпадающей lock-записью
	// обязан переиспользовать уже скачанный кеш, не обращаясь к репозиторию
	// снова.
	if err := os.RemoveAll(repo); err != nil {
		t.Fatalf("удаление тестового репозитория: %v", err)
	}
	dirs, err := ResolveWithRoot(deps, cacheRoot, lockPath, nil)
	if err != nil {
		t.Fatalf("второе Resolve (должно взять из кеша): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dirs["клиент"], "маркер.rech")); err != nil {
		t.Fatalf("ожидался файл из кеша: %v", err)
	}
}

func TestResolveBranchThenPinnedCommitViaLock(t *testing.T) {
	repo := makeTestRepo(t)
	cacheRoot := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), ".rechlock")
	deps := map[string]project.Dependency{
		"клиент": {URL: repo, Версия: "main"},
	}
	dirs, err := ResolveWithRoot(deps, cacheRoot, lockPath, nil)
	if err != nil {
		t.Fatalf("первое Resolve: %v", err)
	}
	firstCommit, err := headCommit(dirs["клиент"])
	if err != nil {
		t.Fatalf("headCommit: %v", err)
	}

	// Репозиторий продвигается дальше (новый коммит на main).
	if err := os.WriteFile(filepath.Join(repo, "второй_файл.rech"), []byte("функция вторая { вывести 1 }\n"), 0o644); err != nil {
		t.Fatalf("запись файла: %v", err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "--quiet", "-m", "второй коммит")

	// Повторный Resolve с ТЕМИ ЖЕ deps (main) обязан остаться на
	// ЗАКРЕПЛЁННОМ (lock-файлом) коммите, не тянуть новый - воспроизводимая
	// сборка не зависит от того, что сейчас лежит на ветке.
	dirs2, err := ResolveWithRoot(deps, cacheRoot, lockPath, nil)
	if err != nil {
		t.Fatalf("второе Resolve: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dirs2["клиент"], "второй_файл.rech")); err == nil {
		t.Fatalf("зависимость не должна была подтянуть новый коммит с ветки")
	}
	secondCommit, err := headCommit(dirs2["клиент"])
	if err != nil {
		t.Fatalf("headCommit: %v", err)
	}
	if secondCommit != firstCommit {
		t.Fatalf("коммит изменился: было %q, стало %q - lock-файл не удержал версию", firstCommit, secondCommit)
	}
}
