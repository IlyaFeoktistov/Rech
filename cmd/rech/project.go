// Файл проекта Rech (.rechproj) - манифест, описывающий границы проекта как
// единицы сборки (docs/ROADMAP.md, Шаг 11): имя и версия проекта, главный
// файл, каталог выхода, режим сборки. Формат - ограниченное подмножество
// YAML: секции верхнего уровня без отступа ("проект:", "файлы:", "сборка:"),
// поля секции - один уровень отступа ("  ключ: значение"). "#" начинает
// комментарий (весь остаток строки), пустые строки игнорируются. Полноценный
// YAML (списки, вложенность глубже одного уровня, кавычки) не поддержан -
// текущему набору полей манифеста этого достаточно, а самодельный разбор не
// тянет в проект внешнюю зависимость.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectFile - разобранный .rechproj.
type ProjectFile struct {
	// Dir - каталог, где лежит сам .rechproj; относительно него разрешаются
	// пути полей "главный" и "выход".
	Dir string

	Имя    string
	Версия string

	Исходники string // пока не используется сборкой - главный файл тянет остальные через "использовать" (см. module.Load); поле для будущего Шага (сборка каталога без единого входа) и для инструментов вроде rech-vscode.
	Главный   string
	Выход     string

	Режим     string
	Платформа string
}

// projectFields - соответствие "секция.ключ" -> адрес поля ProjectFile,
// используется при разборе.
func (pf *ProjectFile) fields() map[string]*string {
	return map[string]*string{
		"проект.имя":       &pf.Имя,
		"проект.версия":    &pf.Версия,
		"файлы.исходники":  &pf.Исходники,
		"файлы.главный":    &pf.Главный,
		"файлы.выход":      &pf.Выход,
		"сборка.режим":     &pf.Режим,
		"сборка.платформа": &pf.Платформа,
	}
}

// findProjectFile ищет единственный "*.rechproj" в текущем каталоге - для
// "rech build"/"rech run" без явного пути, по аналогии с тем, как "go build"
// без аргументов работает от go.mod текущего каталога. Возвращает "" (без
// ошибки), если файла проекта нет вовсе - тогда main.go продолжает обычным
// путём (ожидая явный <файл.rech>|<каталог>).
func findProjectFile() (string, error) {
	matches, err := filepath.Glob("*.rechproj")
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("в текущем каталоге больше одного файла проекта: %s", strings.Join(matches, ", "))
	}
	return matches[0], nil
}

// loadProjectFile читает и разбирает .rechproj по path, подставляет
// значения по умолчанию для необязательных полей и проверяет допустимость
// "режим"/"платформа".
func loadProjectFile(path string) (*ProjectFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл проекта: %w", err)
	}
	pf := &ProjectFile{Dir: filepath.Dir(path)}
	fields := pf.fields()

	section := ""
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := sc.Text()
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, fmt.Errorf("%s: не удалось разобрать строку %q - ожидалось \"ключ: значение\"", path, line)
		}
		full := section + "." + strings.TrimSpace(key)
		target, known := fields[full]
		if !known {
			return nil, fmt.Errorf("%s: неизвестное поле %q", path, full)
		}
		*target = strings.TrimSpace(value)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	if pf.Имя == "" {
		return nil, fmt.Errorf("%s: не указано обязательное поле проект.имя", path)
	}
	if pf.Главный == "" {
		return nil, fmt.Errorf("%s: не указано обязательное поле файлы.главный", path)
	}
	if pf.Выход == "" {
		pf.Выход = "сборка"
	}
	if pf.Режим == "" {
		pf.Режим = "релиз"
	}
	if pf.Платформа == "" {
		pf.Платформа = "текущая"
	}
	if pf.Режим != "отладка" && pf.Режим != "релиз" {
		return nil, fmt.Errorf("%s: сборка.режим %q не распознан - допустимо \"отладка\" или \"релиз\"", path, pf.Режим)
	}
	if pf.Платформа != "текущая" {
		return nil, fmt.Errorf("%s: сборка.платформа %q не поддержана - кросс-компиляция пока не реализована, допустимо только \"текущая\"", path, pf.Платформа)
	}
	return pf, nil
}

// MainPath - путь к главному файлу, разрешённый относительно каталога
// файла проекта.
func (pf *ProjectFile) MainPath() string {
	return filepath.Join(pf.Dir, pf.Главный)
}

// OutputDir - путь к каталогу выхода, разрешённый относительно каталога
// файла проекта.
func (pf *ProjectFile) OutputDir() string {
	return filepath.Join(pf.Dir, pf.Выход)
}

// KeepC отражает "сборка.режим": "отладка" оставляет сгенерированный .c
// рядом с бинарником (как --keep-c), "релиз" - убирает его.
func (pf *ProjectFile) KeepC() bool {
	return pf.Режим == "отладка"
}

// projectTemplate - содержимое нового .rechproj (rech init). "версия: 0.1.0"
// и "режим: отладка" - разумные значения для только что созданного проекта
// (ещё не выпущен, полезно видеть сгенерированный .c), а не значения по
// умолчанию loadProjectFile (которые рассчитаны на уже существующий, зрелый
// манифест - "версия" там вообще не обязательна).
const projectTemplate = `проект:
  имя: %s
  версия: 0.1.0

файлы:
  исходники: исходники/**/*.rech
  главный: исходники/главный.rech
  выход: сборка

сборка:
  режим: отладка
  платформа: текущая
`

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

	if err := os.WriteFile(projPath, fmt.Appendf(nil, projectTemplate, name), 0o644); err != nil {
		return fmt.Errorf("не удалось создать %s: %w", projPath, err)
	}
	fmt.Println("создан", projPath)
	return nil
}
