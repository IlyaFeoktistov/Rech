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

// resolveDeps скачивает/находит в кеше все зависимости pf.Зависимости (nil,
// если их нет - тогда сборка идёт как раньше, без внешних зависимостей) -
// общий код для "rech build/run" (перед компиляцией) и "rech install"
// (отдельно, без сборки).
func resolveDeps(pf *project.File) (map[string]string, error) {
	if len(pf.Зависимости) == 0 {
		return nil, nil
	}
	lockPath := filepath.Join(pf.Dir, pkgfetch.LockFileName)
	return pkgfetch.Resolve(pf.Зависимости, lockPath, os.Stdout)
}

// installDeps - "rech install": находит файл проекта (*.rechproj,
// единственный в текущем каталоге - project.Find, тем же принципом, что
// build/run без аргументов) и заранее скачивает/обновляет кеш каждой
// объявленной зависимости. "rech build/run" делают то же самое сами перед
// компиляцией - install не обязателен для сборки, но даёт явный,
// предсказуемый момент скачивания (прогреть кеш в CI, обновить lock-файл
// после правки .rechproj) без немедленной компиляции.
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
	dirs, err := resolveDeps(pf)
	if err != nil {
		return err
	}
	fmt.Printf("готово: %d завис. (lock-файл %s)\n", len(dirs), filepath.Join(pf.Dir, pkgfetch.LockFileName))
	return nil
}
