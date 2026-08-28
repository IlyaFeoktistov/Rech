// Package pkgfetch скачивает и кеширует внешние зависимости, объявленные в
// секции "зависимости:" файла проекта (.rechproj, internal/project) - через
// "git clone" по URL. Не завязано на конкретный хостинг (GitHub/GitLab/
// gitverse.ru/самостоятельно размещённый сервер, что угодно, что умеет
// отвечать на git-протокол) - "git clone <url>" одинаково работает для
// любого из них, различие серверов git не видит вообще.
//
// Версия (тег/ветка/коммит из "зависимости:") резолвится в конкретный
// commit hash при первом скачивании и фиксируется в lock-файле рядом с
// .rechproj (тот же принцип, что go.sum/Cargo.lock - воспроизводимая
// сборка не зависит от того, что сейчас лежит на сервере под тем же именем
// ветки/тега). Единица публикации - весь репозиторий целиком, не отдельный
// файл: "использовать" внутри зависимости резолвится обычным способом
// (internal/module) относительно скачанного каталога.
//
// Не реализовано в этом срезе (известное ограничение, не забытый случай):
// зависимость по прямой ссылке на файл, а не на git-репозиторий ("любая
// ссылка" без git) - только git-репозитории.
package pkgfetch

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// LockFileName - имя lock-файла, всегда рядом с .rechproj.
const LockFileName = ".rechlock"

// commitHashRe - похоже ли значение "версия" на коммит (шестнадцатеричная
// строка длиной 7-40, обычная короткая/полная форма git-хеша), а не на имя
// ветки/тега - от этого зависит, можно ли сделать быстрый мелкий clone
// (--depth 1 --branch) или нужен полный clone с последующим checkout
// (произвольный коммит недостижим по имени ветки/тега для git clone).
var commitHashRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

func isCommitHash(ref string) bool {
	return commitHashRe.MatchString(ref)
}

// LockEntry - одна строка lock-файла: версия из .rechproj в момент
// разрешения + фактический commit hash, на который она указывала.
type LockEntry struct {
	URL    string
	Версия string
	Commit string
}

// CacheRoot - корневой каталог кеша зависимостей (аналог $GOPATH/pkg/mod) -
// в пользовательском кеше ОС; если он недоступен (например, контейнер без
// $HOME) - подкаталог в текущем рабочем каталоге как запасной вариант.
func CacheRoot() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Abs(".rech_кеш")
	}
	return filepath.Join(dir, "rech", "pkg"), nil
}

// cacheDirFor - каталог кеша конкретной (URL, версия) - URL без схемы,
// сегменты пути сохраняются как подкаталоги (тот же вид, что и у
// $GOPATH/pkg/mod/<host>/<path>@<версия>), версия - суффикс последнего
// сегмента ("/" в версии - как у ветки "feature/x" - заменяется на "_",
// чтобы не создать лишний уровень вложенности).
func cacheDirFor(root, url, version string) (string, error) {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://"), ".git")
	if trimmed == "" {
		return "", fmt.Errorf("пустой URL")
	}
	safeVersion := strings.ReplaceAll(version, "/", "_")
	return filepath.Join(root, trimmed+"@"+safeVersion), nil
}

func readLock(path string) (map[string]LockEntry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]LockEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]LockEntry{}
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("%s: не удалось разобрать строку %q", path, line)
		}
		parts := strings.Fields(rest)
		if len(parts) != 3 {
			return nil, fmt.Errorf("%s: не удалось разобрать строку %q - ожидалось \"имя: url версия коммит\"", path, line)
		}
		out[strings.TrimSpace(name)] = LockEntry{URL: parts[0], Версия: parts[1], Commit: parts[2]}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func writeLock(path string, entries map[string]LockEntry) error {
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString("# Автоматически сгенерировано rech (install/build) - не редактировать руками.\n")
	sb.WriteString("# Формат: имя: url версия коммит\n")
	for _, n := range names {
		e := entries[n]
		fmt.Fprintf(&sb, "%s: %s %s %s\n", n, e.URL, e.Версия, e.Commit)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func headCommit(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// cloneAt скачивает url в dir на ref: fullClone==false - мелкий clone
// (--depth 1 --branch ref), годится только для имени ветки/тега; fullClone
// == true - полный clone + отдельный "checkout ref" (единственный способ
// получить произвольный коммит - git clone --branch не принимает
// коммит-хеш). dir очищается перед скачиванием (git clone не пишет в
// непустой каталог).
func cloneAt(dir, url, ref string, fullClone bool) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("не удалось очистить каталог кеша %q: %w", dir, err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	var cmd *exec.Cmd
	if fullClone {
		cmd = exec.Command("git", "clone", "--quiet", url, dir)
	} else {
		cmd = exec.Command("git", "clone", "--quiet", "--depth", "1", "--branch", ref, url, dir)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone %s: %w\n%s", url, err, out)
	}
	if fullClone {
		checkout := exec.Command("git", "-C", dir, "checkout", "--quiet", ref)
		if out, err := checkout.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout %s (%s): %w\n%s", ref, url, err, out)
		}
	}
	return nil
}

// Resolve скачивает/находит в кеше каждую зависимость deps (обычно -
// project.File.Зависимости) и возвращает имя -> локальный каталог с уже
// готовыми исходниками. lockPath - путь к lock-файлу (см. LockFileName) -
// читается и, если появились новые/изменившиеся записи, перезаписывается.
// progress - куда писать статус скачивания (например, os.Stdout) - может
// быть nil (тогда ничего не печатается).
//
// Уже закешированная (по URL+версии из .rechproj, совпадающей с lock-
// записью) зависимость с уже закачанным нужным коммитом переиспользуется
// без обращения к сети вообще - воспроизводимость и скорость повторных
// сборок.
func Resolve(deps map[string]project.Dependency, lockPath string, progress io.Writer) (map[string]string, error) {
	root, err := CacheRoot()
	if err != nil {
		return nil, fmt.Errorf("каталог кеша зависимостей: %w", err)
	}
	return ResolveWithRoot(deps, root, lockPath, progress)
}

// ResolveWithRoot - Resolve с явно заданным каталогом кеша (root) вместо
// стандартного CacheRoot() - вынесено отдельно ради тестов (изолированный
// временный каталог вместо настоящего пользовательского кеша).
func ResolveWithRoot(deps map[string]project.Dependency, root, lockPath string, progress io.Writer) (map[string]string, error) {
	if len(deps) == 0 {
		return nil, nil
	}
	if progress == nil {
		progress = io.Discard
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("для скачивания зависимостей нужен установленный git (не найден в PATH): %w", err)
	}
	lock, err := readLock(lockPath)
	if err != nil {
		return nil, fmt.Errorf("lock-файл %s: %w", lockPath, err)
	}

	names := make([]string, 0, len(deps))
	for n := range deps {
		names = append(names, n)
	}
	sort.Strings(names)

	result := map[string]string{}
	changed := false
	for _, name := range names {
		dep := deps[name]
		dir, err := cacheDirFor(root, dep.URL, dep.Версия)
		if err != nil {
			return nil, fmt.Errorf("зависимость %q: %w", name, err)
		}

		if locked, ok := lock[name]; ok && locked.URL == dep.URL && locked.Версия == dep.Версия {
			if commit, err := headCommit(dir); err == nil && commit == locked.Commit {
				result[name] = dir
				continue
			}
			fmt.Fprintf(progress, "зависимость %q: скачивание %s (закреплённый коммит %s)...\n", name, dep.URL, locked.Commit)
			if err := cloneAt(dir, dep.URL, locked.Commit, true); err != nil {
				return nil, fmt.Errorf("зависимость %q: %w", name, err)
			}
			result[name] = dir
			continue
		}

		fmt.Fprintf(progress, "зависимость %q: скачивание %s@%s...\n", name, dep.URL, dep.Версия)
		if err := cloneAt(dir, dep.URL, dep.Версия, isCommitHash(dep.Версия)); err != nil {
			return nil, fmt.Errorf("зависимость %q: %w", name, err)
		}
		commit, err := headCommit(dir)
		if err != nil {
			return nil, fmt.Errorf("зависимость %q: не удалось определить скачанный коммит: %w", name, err)
		}
		lock[name] = LockEntry{URL: dep.URL, Версия: dep.Версия, Commit: commit}
		changed = true
		result[name] = dir
	}

	if changed {
		if err := writeLock(lockPath, lock); err != nil {
			return nil, fmt.Errorf("lock-файл %s: %w", lockPath, err)
		}
	}
	return result, nil
}
