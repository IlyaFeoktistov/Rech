// "rech init" - создаёт заготовку проекта (.rechproj + исходники/главный.rech)
// в текущем каталоге. Сам формат .rechproj и его разбор - internal/project
// (используется не только отсюда, см. package doc там).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// initProject создаёт <имя>.rechproj и (если его ещё нет) главный файл в
// исходники/ в текущем каталоге - "rech init [--console|--gui|--library|
// --static-library] [имя]" (initKinds). Каталог и
// имя файла - на русском (как и весь остальной язык, а не только код в
// файлах) - это касается и путей, которые пишет пользователь, раз имена
// файлов/каталогов на диске у него тоже будут русскими. Пустой name -
// берётся имя текущего каталога, как "go mod init" без аргумента в
// некоторых обёртках. Не перезаписывает уже существующие файлы - init
// должен быть безопасен для повторного запуска в непустом каталоге.
func initProject(args []string) error {
	flag, name, err := parseInitArgs(args)
	if err != nil {
		return err
	}
	kind := initKinds[flag]
	if name == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("не удалось определить имя проекта: %w", err)
		}
		name = filepath.Base(cwd)
	}
	if kind.libPrefix && !strings.HasPrefix(name, libNamePrefix) {
		name = libNamePrefix + name
	}

	projPath := name + ".rechproj"
	if _, err := os.Stat(projPath); err == nil {
		return fmt.Errorf("файл проекта %q уже существует", projPath)
	}

	mainPath := filepath.Join("исходники", kind.mainFile)
	if _, err := os.Stat(mainPath); err != nil {
		if err := os.MkdirAll("исходники", 0o755); err != nil {
			return fmt.Errorf("не удалось создать каталог исходники: %w", err)
		}
		if err := os.WriteFile(mainPath, []byte(kind.source), 0o644); err != nil {
			return fmt.Errorf("не удалось создать %s: %w", mainPath, err)
		}
		fmt.Println("создан", mainPath)
	}

	if err := os.WriteFile(projPath, []byte(project.RenderTemplate(name, kind.mainFile, kind.buildLines)), 0o644); err != nil {
		return fmt.Errorf("не удалось создать %s: %w", projPath, err)
	}
	fmt.Println("создан", projPath)
	return nil
}

// libNamePrefix - см. initKind.libPrefix.
const libNamePrefix = "lib"

// parseInitArgs - флаг типа проекта (defaultInitFlag, если не указан) и
// имя; неизвестный флаг или больше одного имени - ошибка.
func parseInitArgs(args []string) (flag, name string, err error) {
	flag = defaultInitFlag
	flagSet := false
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			if _, ok := initKinds[a]; !ok {
				return "", "", fmt.Errorf("неизвестный тип проекта %q - допустимо %s", a, strings.Join(initFlags(), ", "))
			}
			if flagSet {
				return "", "", fmt.Errorf("тип проекта указан дважды: %s и %s", flag, a)
			}
			flag, flagSet = a, true
			continue
		}
		if name != "" {
			return "", "", fmt.Errorf("лишний аргумент %q - ожидалось одно имя проекта", a)
		}
		name = a
	}
	return flag, name, nil
}

// initFlags - флаги initKinds по алфавиту (для сообщений).
func initFlags() []string {
	flags := make([]string, 0, len(initKinds))
	for f := range initKinds {
		flags = append(flags, f)
	}
	sort.Strings(flags)
	return flags
}
