package pkgfetch

import (
	"fmt"
	"io"
	"os/exec"
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
	if err := resolveScope(cacheRoot, rootDeps, rootDir, lock, &changed, g, seenLibs, nil, progress); err != nil {
		return nil, err
	}
	if changed {
		if err := writeLock(lockPath, lock); err != nil {
			return nil, fmt.Errorf("lock-файл %s: %w", lockPath, err)
		}
	}
	return g, nil
}

// resolveScope резолвит deps (все объявленные в ОДНОМ *.rechproj, живущем
// в scopeDir) в g.Scopes[scopeDir], затем для каждой скачанной зависимости
// с собственным *.rechproj - рекурсивно резолвит её "зависимости:" тоже.
// urlChain - цепочка URL уже посещённых на пути от корня (обнаружение
// циклов - А зависит от Б, Б прямо или косвенно от А).
func resolveScope(cacheRoot string, deps map[string]project.Dependency, scopeDir string, lock map[string]LockEntry, changed *bool, g *Graph, seenLibs map[string]bool, urlChain []string, progress io.Writer) error {
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
		for _, lib := range subPf.Библиотеки() {
			if !seenLibs[lib] {
				seenLibs[lib] = true
				g.Libraries = append(g.Libraries, lib)
			}
		}
		if len(subPf.Зависимости) == 0 {
			continue
		}
		if err := resolveScope(cacheRoot, subPf.Зависимости, dir, lock, changed, g, seenLibs, append(urlChain, dep.URL), progress); err != nil {
			return err
		}
	}
	return nil
}
