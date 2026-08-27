// "rech init" - создаёт заготовку проекта (.rechproj + исходники/главный.rech)
// в текущем каталоге. Сам формат .rechproj и его разбор - internal/project
// (используется не только отсюда, см. package doc там).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/IlyaFeoktistov/Rech/internal/project"
)

const mainTemplate = `функция главная
{
    вывести "привет, мир"
}
`

// initProject создаёт <имя>.rechproj и (если его ещё нет)
// исходники/главный.rech в текущем каталоге - "rech init [имя]". Каталог и
// имя файла - на русском (как и весь остальной язык, а не только код в
// файлах) - это касается и путей, которые пишет пользователь, раз имена
// файлов/каталогов на диске у него тоже будут русскими. Пустой name -
// берётся имя текущего каталога, как "go mod init" без аргумента в
// некоторых обёртках. Не перезаписывает уже существующие файлы - init
// должен быть безопасен для повторного запуска в непустом каталоге.
func initProject(name string) error {
	if name == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("не удалось определить имя проекта: %w", err)
		}
		name = filepath.Base(cwd)
	}

	projPath := name + ".rechproj"
	if _, err := os.Stat(projPath); err == nil {
		return fmt.Errorf("файл проекта %q уже существует", projPath)
	}

	mainPath := filepath.Join("исходники", "главный.rech")
	if _, err := os.Stat(mainPath); err != nil {
		if err := os.MkdirAll("исходники", 0o755); err != nil {
			return fmt.Errorf("не удалось создать каталог исходники: %w", err)
		}
		if err := os.WriteFile(mainPath, []byte(mainTemplate), 0o644); err != nil {
			return fmt.Errorf("не удалось создать %s: %w", mainPath, err)
		}
		fmt.Println("создан", mainPath)
	}

	if err := os.WriteFile(projPath, fmt.Appendf(nil, project.Template, name), 0o644); err != nil {
		return fmt.Errorf("не удалось создать %s: %w", projPath, err)
	}
	fmt.Println("создан", projPath)
	return nil
}
