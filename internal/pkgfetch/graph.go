package pkgfetch

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// Graph - результат разрешения ВСЕГО дерева зависимостей, включая
// транзитивные (зависимость зависимости - через её собственный *.rechproj,
// если он есть). См. package doc про то, почему это дерево скопированных
// таблиц имён, а не одна общая плоская карта.
type Graph struct {
	// Scopes - каталог проекта (корневой .rechproj.Dir или каталог кеша
	// скачанной зависимости) -> его СОБСТВЕННАЯ карта "локальное имя
	// зависимости -> локальный каталог с исходниками". internal/module
	// резолвит "использовать" по карте ТОГО каталога, где лежит
	// импортирующий файл (см. internal/module.LoadWithGraph).
	Scopes map[string]map[string]string
	// Libraries - объединённый, дедуплицированный (в порядке первой
	// встречи) список внешних библиотек для линковки (-l<имя>), собранный
	// из "сборка.библиотеки" КАЖДОГО *.rechproj в графе (не только
	// корневого) - зависимость, оборачивающая системную C-библиотеку через
	// FFI (Шаг 7), заявляет требование к линковке тем же полем, что и
	// обычный проект, оно просто попадает в общий список.
	Libraries []string
	// LibraryPaths - объединённый, дедуплицированный (в порядке первой
	// встречи) список каталогов для поиска библиотек из Libraries выше
	// (-L<путь>/rpath) - собран автоматически, без единой строчки
	// "сборка.путь_библиотек" в манифесте ПОТРЕБИТЕЛЯ: у каждой
	// зависимости, объявившей "сборка.библиотеки", берётся её собственный
	// "сборка.путь_библиотек" (если она сама его указала - например,
	// подкаталог с бинарником под конкретную платформу), а без него - её
	// СОБСТВЕННЫЙ каталог (dir, тот же, что и в Scopes) как разумное
	// умолчание: закрытый (бинарный, без исходников) Rech-пакет,
	// публикующий готовый .so/.dylib/.dll рядом со своим *.rechproj, не
	// требует от потребителя вручную угадывать путь к чужому каталогу
	// кеша (internal/pkgfetch.CacheRoot) - тем же принципом, что и
	// Libraries, работает для ЛЮБОЙ зависимости с "сборка.библиотеки", не
	// только для специально оформленного "закрытого пакета" - обычная
	// FFI-обёртка системной C-библиотеки, случайно кладущая свой .so
	// рядом с исходниками, получает то же самое бесплатно.
	LibraryPaths []string
}

// ResolveGraph - как Resolve, но рекурсивно обходит транзитивные
// зависимости и собирает Libraries. rootDir - каталог КОРНЕВОГО .rechproj
// (Scopes[rootDir] - его собственная карта имён).
func ResolveGraph(rootDeps map[string]project.Dependency, rootDir, lockPath string, progress io.Writer) (*Graph, error) {
	root, err := CacheRoot()
	if err != nil {
		return nil, fmt.Errorf("каталог кеша зависимостей: %w", err)
	}
	return ResolveGraphWithRoot(rootDeps, root, rootDir, lockPath, progress)
}

// ResolveGraphWithRoot - ResolveGraph с явно заданным каталогом кеша -
// вынесено отдельно ради тестов, тем же принципом, что и ResolveWithRoot.
func ResolveGraphWithRoot(rootDeps map[string]project.Dependency, cacheRoot, rootDir, lockPath string, progress io.Writer) (*Graph, error) {
	if progress == nil {
		progress = io.Discard
	}
	g := &Graph{Scopes: map[string]map[string]string{}}
	if len(rootDeps) == 0 {
		return g, nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("для скачивания зависимостей нужен установленный git (не найден в PATH): %w", err)
	}
	lock, err := readLock(lockPath)
	if err != nil {
		return nil, fmt.Errorf("lock-файл %s: %w", lockPath, err)
	}
	changed := false
	seenLibs := map[string]bool{}
	seenLibPaths := map[string]bool{}
	if err := resolveScope(cacheRoot, rootDeps, rootDir, lock, &changed, g, seenLibs, seenLibPaths, nil, progress); err != nil {
		return nil, err
	}
	if changed {
		if err := writeLock(lockPath, lock); err != nil {
			return nil, fmt.Errorf("lock-файл %s: %w", lockPath, err)
		}
	}
	if err := MaterializeRoot(g, rootDir); err != nil {
		return nil, err
	}
	return g, nil
}

// materializedMarker - имя файла-маркера внутри каждого материализованного
// "<rootDir>/библиотеки/<имя>/" - хранит коммит, с которого зависимость
// была скопирована в последний раз (см. MaterializeRoot) - позволяет НЕ
// копировать заново на каждый "rech build/run", пока .rechlock указывает
// на тот же коммит.
const materializedMarker = ".rech-commit"

// MaterializeRoot копирует КАЖДУЮ ПРЯМУЮ зависимость проекта (объявленную
// в его *.rechproj, g.Scopes[rootDir]) из общего кеша ОС
// (internal/pkgfetch.CacheRoot) в "<rootDir>/библиотеки/<имя>/" -
// зависимости РЕАЛЬНО лежат в структуре проекта, а не только в кеше вне
// него (пользователь явно попросил: файлы, которые приносит зависимость,
// не должны быть "разбросаны" - выделенная папка "библиотеки/", по
// подкаталогу на пакет, сохраняя его внутреннюю структуру).
//
// ТОЛЬКО корневые зависимости - транзитивные (зависимости зависимостей)
// остаются в кеше как есть и резолвятся из него, как раньше: "использовать"
// внутри самой зависимости не видит разницы, откуда физически подгружен
// файл - только корневой уровень напрямую участвует в "использовать"
// САМОГО проекта и в rpath собранного бинарника, копировать глубже пришлось
// бы рекурсивно для каждой зависимости отдельно без дополнительной пользы.
//
// g мутируется на месте: g.Scopes[rootDir][имя] переезжает на новый путь, а
// с ним и КЛЮЧ g.Scopes для собственной карты имён этой зависимости (если
// она сама объявляет "зависимости:" - см. resolveScope, scopeDir) - код
// дальше по конвейеру (internal/module, rpath) не должен знать про кеш
// вовсе. g.LibraryPaths, указывавшие внутрь старого (кеш) каталога -
// переписываются тем же принципом (включая случай, когда зависимость сама
// объявила "путь_библиотек" ПОДКАТАЛОГОМ внутри себя - project.
// ПутиБиблиотек уже возвращает абсолютный путь, замена префикса переносит
// его на новое место без потери суффикса).
func MaterializeRoot(g *Graph, rootDir string) error {
	scope := g.Scopes[rootDir]
	if len(scope) == 0 {
		return nil
	}
	names := make([]string, 0, len(scope))
	for name := range scope {
		names = append(names, name)
	}
	sort.Strings(names)

	librariesDir := filepath.Join(rootDir, "библиотеки")
	for _, name := range names {
		oldDir := scope[name]
		newDir := filepath.Join(librariesDir, name)

		commit, err := headCommit(oldDir)
		if err != nil {
			return fmt.Errorf("зависимость %q: не удалось определить коммит %q: %w", name, oldDir, err)
		}
		if materializedCommit(newDir) != commit {
			if err := os.RemoveAll(newDir); err != nil {
				return fmt.Errorf("зависимость %q: не удалось очистить %q: %w", name, newDir, err)
			}
			if err := copyTree(oldDir, newDir); err != nil {
				return fmt.Errorf("зависимость %q: копирование в %q: %w", name, newDir, err)
			}
			if err := os.WriteFile(filepath.Join(newDir, materializedMarker), []byte(commit), 0o644); err != nil {
				return fmt.Errorf("зависимость %q: %w", name, err)
			}
		}

		scope[name] = newDir
		if sub, ok := g.Scopes[oldDir]; ok {
			g.Scopes[newDir] = sub
			delete(g.Scopes, oldDir)
		}
		for i, p := range g.LibraryPaths {
			if p == oldDir {
				g.LibraryPaths[i] = newDir
			} else if rest, ok := strings.CutPrefix(p, oldDir+string(filepath.Separator)); ok {
				g.LibraryPaths[i] = filepath.Join(newDir, rest)
			}
		}
	}
	return nil
}

// materializedCommit читает materializedMarker из уже (возможно)
// материализованного newDir - "" (никогда не равно настоящему хешу
// коммита), если каталога/маркера ещё нет - тогда MaterializeRoot копирует
// безусловно.
func materializedCommit(newDir string) string {
	data, err := os.ReadFile(filepath.Join(newDir, materializedMarker))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// copyTree копирует содержимое src в dst (создавая dst и недостающие
// подкаталоги) - обычные файлы и права доступа, БЕЗ ".git" (материализованная
// зависимость не должна выглядеть вложенным git-репозиторием внутри проекта
// потребителя, а сама история клонированного кеша потребителю не нужна).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			return fs.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

// platformLibraryPath - умолчание для "путь_библиотек" закрытой
// зависимости, НЕ указавшей его сама: "<dir>/сборка/<GOOS>-<GOARCH>/"
// ПОТРЕБИТЕЛЯ (runtime.GOOS/GOARCH этого процесса, не того, на котором
// собирался пакет) - соглашение о раскладке многоплатформенного закрытого
// пакета (docs/SPEC.md, "Закрытые (бинарные) пакеты") - автор кладёт
// бинарник под КАЖДУЮ ОС в свой подкаталог (собирает на ней САМ -
// кросс-компиляции самого Rech всё ещё нет), а `rech build/run` выбирает
// нужный подкаталог автоматически по платформе, на которой запущен. Если
// такого подкаталога физически нет на диске (однoплатформенный пакет,
// автор кладёт бинарник прямо в dir) - dir как раньше, без изменений.
func platformLibraryPath(dir string) string {
	platformDir := filepath.Join(dir, "сборка", runtime.GOOS+"-"+runtime.GOARCH)
	if info, err := os.Stat(platformDir); err == nil && info.IsDir() {
		return platformDir
	}
	return dir
}

// resolveScope резолвит deps (все объявленные в ОДНОМ *.rechproj, живущем
// в scopeDir) в g.Scopes[scopeDir], затем для каждой скачанной зависимости
// с собственным *.rechproj - рекурсивно резолвит её "зависимости:" тоже.
// urlChain - цепочка URL уже посещённых на пути от корня (обнаружение
// циклов - А зависит от Б, Б прямо или косвенно от А).
func resolveScope(cacheRoot string, deps map[string]project.Dependency, scopeDir string, lock map[string]LockEntry, changed *bool, g *Graph, seenLibs map[string]bool, seenLibPaths map[string]bool, urlChain []string, progress io.Writer) error {
	names := make([]string, 0, len(deps))
	for n := range deps {
		names = append(names, n)
	}
	sort.Strings(names)

	scope := map[string]string{}
	g.Scopes[scopeDir] = scope
	for _, name := range names {
		dep := deps[name]
		if slices.Contains(urlChain, dep.URL) {
			return fmt.Errorf("циклическая зависимость: %s -> %s", strings.Join(append(urlChain, dep.URL), " -> "), dep.URL)
		}
		dir, err := resolveOne(cacheRoot, dep, lock, changed, progress)
		if err != nil {
			return fmt.Errorf("зависимость %q: %w", name, err)
		}
		scope[name] = dir

		subProjPath, err := project.Find(dir)
		if err != nil {
			return fmt.Errorf("зависимость %q: %w", name, err)
		}
		if subProjPath == "" {
			continue // без собственного .rechproj - нет и собственных транзитивных зависимостей
		}
		subPf, err := project.Load(subProjPath)
		if err != nil {
			return fmt.Errorf("зависимость %q: манифест %s: %w", name, subProjPath, err)
		}
		libs := subPf.Библиотеки()
		for _, lib := range libs {
			if !seenLibs[lib] {
				seenLibs[lib] = true
				g.Libraries = append(g.Libraries, lib)
			}
		}
		if len(libs) > 0 {
			// Путь для линковки собранных выше библиотек - собственный
			// "путь_библиотек" зависимости, если она сама его указала
			// (например, подкаталог с бинарником под конкретную
			// платформу); без него - platformLibraryPath пробует
			// "сборка/<GOOS>-<GOARCH>/" ПОТРЕБИТЕЛЯ (соглашение о
			// раскладке многоплатформенного закрытого пакета - автор сам
			// собирает и кладёт бинарник под КАЖДУЮ ОС в свой подкаталог,
			// кросс-компиляции самого Rech всё ещё нет), а если такого
			// подкаталога физически нет - её СОБСТВЕННЫЙ каталог (dir) -
			// разумное умолчание для однoплатформенного закрытого пакета,
			// кладущего .so/.dylib/.dll прямо рядом со своим .rechproj
			// (см. package doc Graph.LibraryPaths).
			libPaths := subPf.ПутиБиблиотек()
			if len(libPaths) == 0 {
				libPaths = []string{platformLibraryPath(dir)}
			}
			for _, p := range libPaths {
				if !seenLibPaths[p] {
					seenLibPaths[p] = true
					g.LibraryPaths = append(g.LibraryPaths, p)
				}
			}
		}
		if len(subPf.Зависимости) == 0 {
			continue
		}
		if err := resolveScope(cacheRoot, subPf.Зависимости, dir, lock, changed, g, seenLibs, seenLibPaths, append(urlChain, dep.URL), progress); err != nil {
			return err
		}
	}
	return nil
}
