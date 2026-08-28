// Package project - файл проекта Rech (.rechproj), манифест, описывающий
// границы проекта как единицы сборки: имя и версия проекта, главный файл,
// каталог выхода, режим сборки. Формат -
// ограниченное подмножество YAML: секции верхнего уровня без отступа
// ("проект:", "файлы:", "сборка:"), поля секции - один уровень отступа
// ("  ключ: значение"). "#" начинает комментарий (весь остаток строки),
// пустые строки игнорируются. Полноценный YAML (списки, вложенность глубже
// одного уровня, кавычки) не поддержан - текущему набору полей манифеста
// этого достаточно, а самодельный разбор не тянет в проект внешнюю
// зависимость.
//
// Отдельный импортируемый пакет (не package main, cmd/rech), потому что
// файл проекта нужен не только "rech build/run/test" - internal/
// debugadapter резолвит его так же для launch.program ("rech debug").
package project

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// File - разобранный .rechproj.
type File struct {
	// Dir - каталог, где лежит сам .rechproj; относительно него разрешаются
	// пути полей "главный" и "выход".
	Dir string

	Имя    string
	Версия string

	Исходники string // пока не используется сборкой - главный файл тянет остальные через "использовать" (см. module.Load); поле для будущей сборки каталога без единого входа и для инструментов вроде rech-vscode.
	Главный   string
	Выход     string

	Режим     string
	Платформа string
	// Тип - "программа" (по умолчанию, обычный исполняемый файл) или
	// "библиотека" (`rech build --lib` без явного флага - разделяемая
	// библиотека .so/.dylib/.dll + C-заголовок .h). См. IsLibrary.
	Тип string
}

// fields - соответствие "секция.ключ" -> адрес поля File, используется при
// разборе.
func (pf *File) fields() map[string]*string {
	return map[string]*string{
		"проект.имя":       &pf.Имя,
		"проект.версия":    &pf.Версия,
		"файлы.исходники":  &pf.Исходники,
		"файлы.главный":    &pf.Главный,
		"файлы.выход":      &pf.Выход,
		"сборка.режим":     &pf.Режим,
		"сборка.платформа": &pf.Платформа,
		"сборка.тип":       &pf.Тип,
	}
}

// Find ищет единственный "*.rechproj" в dir - для "rech build"/"rech run"
// без явного пути, по аналогии с тем, как "go build" без аргументов
// работает от go.mod текущего каталога. Возвращает "" (без ошибки), если
// файла проекта нет вовсе - вызывающий код продолжает обычным путём (ожидая
// явный <файл.rech>|<каталог>).
func Find(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.rechproj"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("в каталоге %q больше одного файла проекта: %s", dir, strings.Join(matches, ", "))
	}
	return matches[0], nil
}

// Load читает и разбирает .rechproj по path, подставляет значения по
// умолчанию для необязательных полей и проверяет допустимость
// "режим"/"платформа".
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл проекта: %w", err)
	}
	pf := &File{Dir: filepath.Dir(path)}
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
	if pf.Тип == "" {
		pf.Тип = "программа"
	}
	if pf.Тип != "программа" && pf.Тип != "библиотека" {
		return nil, fmt.Errorf("%s: сборка.тип %q не распознан - допустимо \"программа\" или \"библиотека\"", path, pf.Тип)
	}
	return pf, nil
}

// MainPath - путь к главному файлу, разрешённый относительно каталога
// файла проекта.
func (pf *File) MainPath() string {
	return filepath.Join(pf.Dir, pf.Главный)
}

// OutputDir - путь к каталогу выхода, разрешённый относительно каталога
// файла проекта.
func (pf *File) OutputDir() string {
	return filepath.Join(pf.Dir, pf.Выход)
}

// IsLibrary отражает "сборка.тип": "библиотека" - собирать разделяемую
// библиотеку (`rech build --lib`), не исполняемый файл.
func (pf *File) IsLibrary() bool {
	return pf.Тип == "библиотека"
}

// KeepC отражает "сборка.режим": "отладка" оставляет сгенерированный .c
// рядом с бинарником (как --keep-c), "релиз" - убирает его.
func (pf *File) KeepC() bool {
	return pf.Режим == "отладка"
}

// Template - содержимое нового .rechproj (rech init). "версия: 0.1.0" и
// "режим: отладка" - разумные значения для только что созданного проекта
// (ещё не выпущен, полезно видеть сгенерированный .c), а не значения по
// умолчанию Load (которые рассчитаны на уже существующий, зрелый манифест -
// "версия" там вообще не обязательна). %s - имя проекта.
const Template = `проект:
  имя: %s
  версия: 0.1.0

файлы:
  исходники: исходники/**/*.rech
  главный: исходники/главный.rech
  выход: сборка

сборка:
  режим: отладка
  платформа: текущая
  # тип: библиотека (по умолчанию - "программа", обычный исполняемый файл)
`
