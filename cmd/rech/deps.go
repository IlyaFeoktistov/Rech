// "rech install" и разрешение внешних зависимостей файла проекта
// (.rechproj, секция "зависимости:") - см. internal/pkgfetch.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/IlyaFeoktistov/Rech/internal/pkgfetch"
	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// resolveDeps скачивает/находит в кеше все зависимости pf.Зависимости,
// включая ТРАНЗИТИВНЫЕ (зависимости зависимостей - через их собственные
// *.rechproj, если есть), внешние библиотеки, которые они объявляют
// (сборка.библиотеки - для линковки FFI-обёрток, включая закрытые
// (бинарные) Rech-пакеты, см. docs/SPEC.md) и путь для их поиска -
// libPaths (internal/pkgfetch.Graph.LibraryPaths - собственный каталог
// зависимости, если она сама не указала иначе, БЕЗ единой строчки
// "сборка.путь_библиотек" в манифесте потребителя) - общий код для "rech
// build/run" (перед компиляцией) и "rech install" (отдельно, без сборки).
// nil-результаты, если pf вообще не объявляет зависимостей - тогда сборка
// идёт как раньше, без внешних зависимостей.
func resolveDeps(pf *project.File) (deps map[string]string, scopes map[string]map[string]string, libs, libPaths []string, err error) {
	if len(pf.Зависимости) == 0 {
		return nil, nil, nil, nil, nil
	}
	lockPath := filepath.Join(pf.Dir, pkgfetch.LockFileName)
	g, err := pkgfetch.ResolveGraph(pf.Зависимости, pf.Dir, lockPath, os.Stdout)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return g.Scopes[pf.Dir], g.Scopes, g.Libraries, g.LibraryPaths, nil
}

// mergeUnique объединяет список самого проекта (own) со списком,
// собранным из зависимостей (fromDeps - internal/pkgfetch.Graph.Libraries
// или .LibraryPaths, у обоих одна и та же форма "плоский список без
// гарантированной уникальности"), без повторов, в порядке "свои сначала,
// потом чужие по первой встрече".
func mergeUnique(own, fromDeps []string) []string {
	if len(fromDeps) == 0 {
		return own
	}
	seen := make(map[string]bool, len(own)+len(fromDeps))
	out := make([]string, 0, len(own)+len(fromDeps))
	for _, v := range own {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, v := range fromDeps {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// installDeps - "rech install": находит файл проекта (*.rechproj,
// единственный в текущем каталоге - project.Find, тем же принципом, что
// build/run без аргументов) и заранее скачивает/обновляет кеш каждой
// объявленной зависимости (включая транзитивные). "rech build/run" делают
// то же самое сами перед компиляцией - install не обязателен для сборки,
// но даёт явный, предсказуемый момент скачивания (прогреть кеш в CI,
// обновить lock-файл после правки .rechproj) без немедленной компиляции.
func installDeps() error {
	projPath, err := project.Find(".")
	if err != nil {
		return err
	}
	if projPath == "" {
		return fmt.Errorf("в текущем каталоге не найден файл проекта (*.rechproj) - зависимости объявляются там, в секции \"зависимости:\"")
	}
	pf, err := project.Load(projPath)
	if err != nil {
		return err
	}
	if len(pf.Зависимости) == 0 {
		fmt.Println("зависимости не объявлены (в", projPath, "нет секции \"зависимости:\")")
		return nil
	}
	_, scopes, libs, _, err := resolveDeps(pf)
	if err != nil {
		return err
	}
	total := 0
	for _, s := range scopes {
		total += len(s)
	}
	fmt.Printf("готово: %d завис. (lock-файл %s)\n", total, filepath.Join(pf.Dir, pkgfetch.LockFileName))
	if len(libs) > 0 {
		fmt.Println("библиотеки для линковки (из зависимостей):", libs)
	}
	return nil
}
