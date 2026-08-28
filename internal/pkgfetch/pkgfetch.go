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
// Транзитивные зависимости (ResolveGraph) - если у скачанной зависимости
// есть СОБСТВЕННЫЙ *.rechproj с "зависимости:", он резолвится тем же путём,
// рекурсивно. Локальное имя зависимости - алиас, приватный для проекта,
// который его объявил (как в самом Rech - "использовать X из "имя""), а не
// глобальный путь, как в Go modules - поэтому граф разрешения СКОПИРОВАН
// по проектам (Graph.Scopes), не одна общая плоская таблица имён: два
// разных проекта в графе МОГУТ дать одному и тому же локальному имени
// разные зависимости, коллизии в этом нет. lock-файл поэтому ключуется по
// (URL, версия), не по имени - оно приватно для объявившего его проекта и
// не годится как глобальный ключ.
//
// Не реализовано в этом срезе (известные ограничения, не забытые случаи):
// зависимость по прямой ссылке на файл, а не на git-репозиторий; настоящее
// разрешение конфликтов версий (semver/MVS) - один и тот же URL,
// запрошенный на РАЗНЫХ версиях в разных точках графа, - явная ошибка, а
// не автоматический выбор "старшей" версии.
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

// LockFileName - имя lock-файла, всегда рядом с корневым .rechproj.
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

// lockKey - ключ lock-файла: (URL, версия), НЕ локальное имя зависимости -
// имя приватно для объявившего его проекта (см. package doc), одна и та же
// (URL, версия) может быть объявлена под разными именами в разных
// проектах графа и должна резолвиться в ОДИН и тот же закреплённый коммит.
func lockKey(url, version string) string {
	return url + "@" + version
}

// LockEntry - одна строка lock-файла: URL/версия (для читаемости - реальный
// ключ см. lockKey) + фактический commit hash, на который они указывали.
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
		parts := strings.Fields(line)
		if len(parts) != 3 {
			return nil, fmt.Errorf("%s: не удалось разобрать строку %q - ожидалось \"url версия коммит\"", path, line)
		}
		entry := LockEntry{URL: parts[0], Версия: parts[1], Commit: parts[2]}
		out[lockKey(entry.URL, entry.Версия)] = entry
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func writeLock(path string, entries map[string]LockEntry) error {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString("# Автоматически сгенерировано rech (install/build) - не редактировать руками.\n")
	sb.WriteString("# Формат: url версия коммит\n")
	for _, k := range keys {
		e := entries[k]
		fmt.Fprintf(&sb, "%s %s %s\n", e.URL, e.Версия, e.Commit)
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

// resolveOne скачивает/находит в кеше ровно одну (URL, версия) - общий код
// для плоского Resolve и рекурсивного ResolveGraph. lock - разделяемая
// карта на весь граф (читается один раз в начале, пишется один раз в конце
// вызывающей стороной) - resolveOne только читает/дополняет её в памяти.
func resolveOne(root string, dep project.Dependency, lock map[string]LockEntry, changed *bool, progress io.Writer) (string, error) {
	dir, err := cacheDirFor(root, dep.URL, dep.Версия)
	if err != nil {
		return "", err
	}
	key := lockKey(dep.URL, dep.Версия)

	if locked, ok := lock[key]; ok {
		if commit, err := headCommit(dir); err == nil && commit == locked.Commit {
			return dir, nil
		}
		fmt.Fprintf(progress, "скачивание %s (закреплённый коммит %s)...\n", dep.URL, locked.Commit)
		if err := cloneAt(dir, dep.URL, locked.Commit, true); err != nil {
			return "", err
		}
		return dir, nil
	}

	fmt.Fprintf(progress, "скачивание %s@%s...\n", dep.URL, dep.Версия)
	if err := cloneAt(dir, dep.URL, dep.Версия, isCommitHash(dep.Версия)); err != nil {
		return "", err
	}
	commit, err := headCommit(dir)
	if err != nil {
		return "", fmt.Errorf("не удалось определить скачанный коммит: %w", err)
	}
	lock[key] = LockEntry{URL: dep.URL, Версия: dep.Версия, Commit: commit}
	*changed = true
	return dir, nil
}

// Resolve скачивает/находит в кеше каждую зависимость deps (обычно -
// project.File.Зависимости) и возвращает имя -> локальный каталог с уже
// готовыми исходниками - БЕЗ транзитивных зависимостей (см. ResolveGraph
// для полного графа, включая FFI-библиотеки скачанных зависимостей).
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
		dir, err := resolveOne(root, deps[name], lock, &changed, progress)
		if err != nil {
			return nil, fmt.Errorf("зависимость %q: %w", name, err)
		}
		result[name] = dir
	}

	if changed {
		if err := writeLock(lockPath, lock); err != nil {
			return nil, fmt.Errorf("lock-файл %s: %w", lockPath, err)
		}
	}
	return result, nil
}
