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
	"slices"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// Dependency - одна внешняя зависимость из секции "зависимости:" -
// произвольный git-репозиторий (любой хостинг - GitHub/GitLab/gitverse.ru/
// самостоятельно размещённый - "git clone" не завязан на конкретный сервис)
// по URL, зафиксированной версией (тег/ветка/коммит) - см.
// internal/pkgfetch, который её реально скачивает/кеширует.
type Dependency struct {
	URL    string
	Версия string
}

// File - разобранный .rechproj.
type File struct {
	// Dir - каталог, где лежит сам .rechproj; относительно него разрешаются
	// пути полей "главный" и "выход".
	Dir string

	Имя    string
	Версия string

	// Зависимости - секция "зависимости:" (имя -> Dependency), в отличие от
	// остальных полей файла НЕ входит в fields() ниже (значение - не
	// одиночная строка, а составная запись с произвольным именем ключа) -
	// разбирается отдельной веткой в Load.
	Зависимости map[string]Dependency

	// Окружение - секция "окружение:" (имя переменной окружения ->
	// значение) - переменные, с которыми "rech run" запускает собранный
	// бинарник (см. cmd/rech/main.go, runBinary) - тем же принципом, что и
	// у Зависимости, произвольный ключ, не входит в fields(). Не влияет на
	// "rech build" (там нечего запускать). Добавляются ПОВЕРХ os.Environ()
	// самого "rech" (не заменяют его целиком) - удобно задать порт/путь БД
	// один раз в манифесте вместо "ПЕРЕМЕННАЯ=значение rech run" перед
	// каждым запуском.
	Окружение map[string]string

	// Опции - секция "опции:", СЫРОЙ текст (не разобран этим пакетом
	// вообще, в отличие от остальных полей) - единственное место в
	// .rechproj с настоящим, не ограниченным YAML (произвольная глубина
	// вложенности, списки, кавычки) - извлекается verbatim до попадания в
	// основной построчный сканер ниже (см. extractOptionsSection), потому что
	// остальной Load разбирает только один уровень отступа "ключ:
	// значение" и споткнулся бы на вложенности глубже. Доступен программе
	// через встроенную "опции_проекта()" (internal/builtins) -
	// разбирается уже НАСТОЯЩИМ YAML-декодером (internal/backend/cbackend/
	// yaml_extract.go, internal/interp/yaml_serialize.go), тем же, что и у
	// "разобрать YAML ... как <Тип>" - этот пакет специально не пытается
	// понять структуру сам, только вырезает текст.
	Опции string

	// Исходники - "файлы.исходники", glob-путь до .rech-файлов проекта
	// ("исходники/**/*.rech", "*.rech" - без кавычек, формат-минимум
	// пакета их не поддерживает, package doc File). Сама сборка по нему
	// не собирает файлы (главный файл тянет остальные через
	// "использовать", см. module.Load) - поле остаётся для инструментов
	// вроде rech-vscode. Часть БЕЗ подстановочных символов ("*"/"?"/"[") -
	// корень дерева исходников проекта, см. SourcesRoot - точка отсчёта
	// "использовать ... из "@путь"".
	Исходники string
	Главный   string
	Выход     string

	Режим     string
	Платформа string
	// Тип - "консольное" (по умолчанию, исполняемый файл с консолью),
	// "оконное" (исполняемый файл без консоли на Windows - см. IsWindowed)
	// или "библиотека" (`rech build --lib` без явного флага - разделяемая
	// библиотека .so/.dylib/.dll + C-заголовок .h, см. IsLibrary).
	Тип string
	// librariesRaw - "сборка.библиотеки" как есть, через запятую ("curl,
	// ssl") - см. Библиотеки.
	librariesRaw string

	// cCompiler*/cFlags*/linkFlags* - "сборка.компилятор_c"/"флаги_c"/
	// "флаги_линковки" и их варианты для платформ, как есть (см.
	// КомпиляторC/ФлагиC/ФлагиЛинковки).
	cCompilerRaw, cCompilerWindowsRaw, cCompilerLinuxRaw, cCompilerMacosRaw string
	cFlagsRaw, cFlagsWindowsRaw, cFlagsLinuxRaw, cFlagsMacosRaw             string
	linkFlagsRaw, linkFlagsWindowsRaw, linkFlagsLinuxRaw, linkFlagsMacosRaw string
	// present - полные имена полей, записанные в файле (даже с пустым
	// значением): "флаги_c:" без значения - тоже замена встроенных флагов
	// (на пустой список), не их отсутствие.
	present map[string]bool
	// libraryPathsRaw - "сборка.путь_библиотек" как есть, через запятую
	// ("../экспорт_библиотеки/сборка, /общий/каталог/библиотек") - см.
	// ПутиБиблиотек.
	libraryPathsRaw string
	// librariesWindowsRaw/librariesLinuxRaw/librariesMacosRaw -
	// "сборка.библиотеки_windows"/"_linux"/"_macos" - тот же формат, что
	// librariesRaw выше, но добавляются к общему списку ТОЛЬКО на
	// соответствующей платформе (docs/SPEC.md, "#если ОС.<Член>") - нужны,
	// когда объявления внешней функции для разных ОС приходят из разных
	// библиотек (например, `user32`/`gdi32` только на Windows, `X11`
	// только на Linux) - без разбивки по платформе линковщик пытался бы
	// найти ВСЕ библиотеки сразу, а не только те, что реально нужны
	// объявлениям программы для ЭТОЙ платформы.
	librariesWindowsRaw string
	librariesLinuxRaw   string
	librariesMacosRaw   string
	// headersRaw/headersWindowsRaw/headersLinuxRaw/headersMacosRaw -
	// "сборка.заголовки"/"_windows"/"_linux"/"_macos" - тот же
	// принцип/формат, что librariesRaw/librariesWindowsRaw и т. д. выше, но
	// для C-заголовков (`#include <...>`), не библиотек для линковки - см.
	// Заголовки.
	headersRaw        string
	headersWindowsRaw string
	headersLinuxRaw   string
	headersMacosRaw   string
	// libraryPathsWindowsRaw/libraryPathsLinuxRaw/libraryPathsMacosRaw -
	// тот же принцип для "путь_библиотек_windows"/"_linux"/"_macos".
	libraryPathsWindowsRaw string
	libraryPathsLinuxRaw   string
	libraryPathsMacosRaw   string
	// dbPoolSizeRaw - "бд.размер_пула" как есть (ещё не разобрано в int) -
	// см. DBPoolSize.
	dbPoolSizeRaw string
	// dbPoolTimeoutMsRaw - "бд.таймаут_пула_мс" как есть - см.
	// DBPoolTimeoutMs.
	dbPoolTimeoutMsRaw string
	// dbSQLiteBusyTimeoutMsRaw - "бд.sqlite_таймаут_блокировки_мс" как есть -
	// см. DBSQLiteBusyTimeoutMs.
	dbSQLiteBusyTimeoutMsRaw string
	// poolShrinkRaw - "потоки.сжатие_пула" как есть ("истина"/"ложь") - см.
	// PoolShrinkEnabled.
	poolShrinkRaw string
	// poolIdleTimeoutMsRaw - "потоки.таймаут_простоя_мс" как есть - см.
	// PoolIdleTimeoutMs.
	poolIdleTimeoutMsRaw string
	// httpMaxRequestBytesRaw - "http.макс_размер_запроса_байт" как есть - см.
	// HTTPMaxRequestBytes.
	httpMaxRequestBytesRaw string
}

// fields - соответствие "секция.ключ" -> адрес поля File, используется при
// разборе.
func (pf *File) fields() map[string]*string {
	return map[string]*string{
		"проект.имя":                      &pf.Имя,
		"проект.версия":                   &pf.Версия,
		"файлы.исходники":                 &pf.Исходники,
		"файлы.главный":                   &pf.Главный,
		"файлы.выход":                     &pf.Выход,
		"сборка.режим":                    &pf.Режим,
		"сборка.платформа":                &pf.Платформа,
		"сборка.тип":                      &pf.Тип,
		"сборка.библиотеки":               &pf.librariesRaw,
		"сборка.компилятор_c":             &pf.cCompilerRaw,
		"сборка.компилятор_c_windows":     &pf.cCompilerWindowsRaw,
		"сборка.компилятор_c_linux":       &pf.cCompilerLinuxRaw,
		"сборка.компилятор_c_macos":       &pf.cCompilerMacosRaw,
		"сборка.флаги_c":                  &pf.cFlagsRaw,
		"сборка.флаги_c_windows":          &pf.cFlagsWindowsRaw,
		"сборка.флаги_c_linux":            &pf.cFlagsLinuxRaw,
		"сборка.флаги_c_macos":            &pf.cFlagsMacosRaw,
		"сборка.флаги_линковки":           &pf.linkFlagsRaw,
		"сборка.флаги_линковки_windows":   &pf.linkFlagsWindowsRaw,
		"сборка.флаги_линковки_linux":     &pf.linkFlagsLinuxRaw,
		"сборка.флаги_линковки_macos":     &pf.linkFlagsMacosRaw,
		"сборка.путь_библиотек":           &pf.libraryPathsRaw,
		"сборка.библиотеки_windows":       &pf.librariesWindowsRaw,
		"сборка.библиотеки_linux":         &pf.librariesLinuxRaw,
		"сборка.библиотеки_macos":         &pf.librariesMacosRaw,
		"сборка.заголовки":                &pf.headersRaw,
		"сборка.заголовки_windows":        &pf.headersWindowsRaw,
		"сборка.заголовки_linux":          &pf.headersLinuxRaw,
		"сборка.заголовки_macos":          &pf.headersMacosRaw,
		"сборка.путь_библиотек_windows":   &pf.libraryPathsWindowsRaw,
		"сборка.путь_библиотек_linux":     &pf.libraryPathsLinuxRaw,
		"сборка.путь_библиотек_macos":     &pf.libraryPathsMacosRaw,
		"бд.размер_пула":                  &pf.dbPoolSizeRaw,
		"бд.таймаут_пула_мс":              &pf.dbPoolTimeoutMsRaw,
		"бд.sqlite_таймаут_блокировки_мс": &pf.dbSQLiteBusyTimeoutMsRaw,
		"потоки.сжатие_пула":              &pf.poolShrinkRaw,
		"потоки.таймаут_простоя_мс":       &pf.poolIdleTimeoutMsRaw,
		"http.макс_размер_запроса_байт":   &pf.httpMaxRequestBytesRaw,
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

// FindUp ищет "*.rechproj" в dir, а если там его нет - в каждом
// родительском каталоге по очереди, вплоть до корня файловой системы (тот
// же принцип, что и поиск ".git"/"go.mod" вверх по дереву) - в отличие от
// Find (ровно dir, для "rech build"/"rech run" без аргумента, где текущий
// каталог заведомо каталог проекта), нужен коду, которому дан ПРОИЗВОЛЬНЫЙ
// файл на любой глубине дерева проекта (LSP - открытый документ, "rech
// test" - отдельный тестовый файл) и нужно найти .rechproj, под которым он
// лежит. "", nil - .rechproj не найден до самого корня (обычный файл вне
// какого-либо проекта, не ошибка).
func FindUp(dir string) (string, error) {
	dir = filepath.Clean(dir)
	for {
		path, err := Find(dir)
		if err != nil {
			return "", err
		}
		if path != "" {
			return path, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// RootFor находит .rechproj над srcPath (FindUp от его каталога) и
// возвращает её SourcesRoot() - то, что internal/module.LoadWithRoot
// называет projectRoot: точка отсчёта "использовать ... из "@путь""
// (package doc module/resolveImportPath) для srcPath, который сам не
// обязательно и есть главный файл проекта ("rech build"/"rech run" без
// аргумента этой проблемы не знают - они всегда резолвят srcPath =
// pf.MainPath(), см. cmd/rech/main.go). "", nil - .rechproj не найден
// (вызывающая сторона использует прежнее поведение module.LoadWithGraph -
// каталог srcPath).
func RootFor(srcPath string) (string, error) {
	pf, err := LoadFor(srcPath)
	if err != nil {
		return "", err
	}
	if pf == nil {
		return "", nil
	}
	return pf.SourcesRoot(), nil
}

// LoadFor находит и разбирает .rechproj над srcPath тем же способом, что
// RootFor (FindUp от его каталога) - нужен вызывающей стороне, которой
// нужен сам File (например, "сборка.библиотеки"/"путь_библиотек" для FFI
// интерпретатора, internal/testrunner), а не только каталог главного
// файла. nil, nil - .rechproj не найден.
func LoadFor(srcPath string) (*File, error) {
	projPath, err := FindUp(filepath.Dir(filepath.Clean(srcPath)))
	if err != nil {
		return nil, err
	}
	if projPath == "" {
		return nil, nil
	}
	return Load(projPath)
}

// extractOptionsSection вырезает верхнеуровневую секцию "опции:" ИЗ СЫРОГО
// текста .rechproj ДО того, как data попадёт в построчный сканер Load ниже -
// единственный способ дать ей настоящую, не ограниченную вложенность
// (списки, "- элемент", отступ глубже одного уровня) без переписывания
// остального формата: сканер Load режет каждую строку СЕКЦИИ РОВНО одним
// "ключ: значение" и не умеет глубже. Возвращает (текст секции без
// заголовка "опции:" и общего отступа, остаток файла БЕЗ этой секции -
// его получает обычный сканер). Строки этой секции НЕ проходят через
// "#" - вырезание комментария Load (ниже) - настоящий YAML сам знает,
// что такое комментарий, а наивное вырезание "#" сломало бы, например,
// значение `"#ff0000"` в кавычках.
func extractOptionsSection(data string) (options string, rest string) {
	lines := strings.Split(data, "\n")
	var restLines []string
	var block []string
	baseIndent := -1
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		topLevel := trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t")
		if inSection {
			if trimmed == "" {
				block = append(block, "")
				continue
			}
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if baseIndent == -1 {
				baseIndent = indent // отступ ПЕРВОЙ содержательной строки секции - вычитается из всех остальных
			}
			if topLevel || indent < baseIndent {
				inSection = false // конец секции "опции:" - эта строка сама уже не её часть, падает ниже как обычная
			} else {
				block = append(block, line[baseIndent:])
				continue
			}
		}
		if topLevel && strings.TrimSuffix(trimmed, ":") == "опции" {
			inSection = true
			block = nil
			baseIndent = -1
			continue
		}
		restLines = append(restLines, line)
	}
	return strings.TrimRight(strings.Join(block, "\n"), "\n"), strings.Join(restLines, "\n")
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
	options, rest := extractOptionsSection(string(data))
	pf.Опции = options

	fields := pf.fields()

	section := ""
	sc := bufio.NewScanner(strings.NewReader(rest))
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
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if section == "зависимости" {
			parts := strings.Fields(value)
			if len(parts) != 2 {
				return nil, fmt.Errorf("%s: зависимость %q - ожидалось \"URL версия\" (например, \"https://github.com/user/repo v1.2.0\"), получено %q", path, key, value)
			}
			if pf.Зависимости == nil {
				pf.Зависимости = map[string]Dependency{}
			}
			if _, exists := pf.Зависимости[key]; exists {
				return nil, fmt.Errorf("%s: зависимость %q объявлена более одного раза", path, key)
			}
			pf.Зависимости[key] = Dependency{URL: parts[0], Версия: parts[1]}
			continue
		}
		if section == "окружение" {
			if pf.Окружение == nil {
				pf.Окружение = map[string]string{}
			}
			if _, exists := pf.Окружение[key]; exists {
				return nil, fmt.Errorf("%s: переменная окружения %q объявлена более одного раза", path, key)
			}
			pf.Окружение[key] = value
			continue
		}
		full := section + "." + key
		target, known := fields[full]
		if !known {
			return nil, fmt.Errorf("%s: неизвестное поле %q", path, full)
		}
		*target = value
		if pf.present == nil {
			pf.present = map[string]bool{}
		}
		pf.present[full] = true
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
	if _, known := platformOSNames[pf.Платформа]; !known {
		return nil, fmt.Errorf("%s: сборка.платформа %q не распознана - допустимо \"текущая\", \"linux\", \"windows\", \"macos\" или \"unix\"", path, pf.Платформа)
	}
	if pf.Тип == "" {
		pf.Тип = buildTypeConsole
	}
	if !buildTypes[pf.Тип] {
		return nil, fmt.Errorf("%s: сборка.тип %q не распознан - допустимо \"консольное\", \"оконное\" или \"библиотека\"", path, pf.Тип)
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

// sourcesRootFromGlob - каталог-префикс glob-пути raw ДО первого сегмента
// с подстановочным символом ("*"/"?"/"[") - "." (сам каталог .rechproj),
// если такого префикса нет вовсе (например, raw == "*.rech" или "") - см.
// package doc File.Исходники/SourcesRoot ниже.
func sourcesRootFromGlob(raw string) string {
	if raw == "" {
		return "."
	}
	segments := strings.Split(filepath.ToSlash(raw), "/")
	prefix := 0
	for _, seg := range segments {
		if strings.ContainsAny(seg, "*?[") {
			break
		}
		prefix++
	}
	if prefix == 0 {
		return "."
	}
	return filepath.Join(segments[:prefix]...)
}

// SourcesRoot - корень дерева исходников проекта, разрешённый относительно
// каталога .rechproj - точка отсчёта "использовать ... из "@путь""
// (internal/module, resolveImportPath/LoadWithRoot; project.RootFor
// передаёт результат этой функции дальше). Источник - "файлы.исходники"
// (sourcesRootFromGlob выше); если поле не указано вовсе - каталог
// главного файла (filepath.Dir(pf.MainPath())), прежнее поведение для
// проектов без явного "файлы.исходники" (обратная совместимость -
// раньше "@" ВСЕГДА отталкивался от каталога главного файла, что путало
// декларацию корня с тем, где именно внутри него лежит точка входа -
// главный файл в принципе может лежать глубже настоящего корня дерева
// исходников).
func (pf *File) SourcesRoot() string {
	if pf.Исходники == "" {
		return filepath.Dir(pf.MainPath())
	}
	return filepath.Join(pf.Dir, sourcesRootFromGlob(pf.Исходники))
}

// Значения "сборка.тип"; buildTypes - единственный список допустимых
// (Load проверяет по нему).
const (
	buildTypeConsole  = "консольное"
	buildTypeWindowed = "оконное"
	buildTypeLibrary  = "библиотека"
)

var buildTypes = map[string]bool{
	buildTypeConsole:  true,
	buildTypeWindowed: true,
	buildTypeLibrary:  true,
}

// IsLibrary отражает "сборка.тип": "библиотека" - собирать разделяемую
// библиотеку (`rech build --lib`), не исполняемый файл.
func (pf *File) IsLibrary() bool {
	return pf.Тип == buildTypeLibrary
}

// IsWindowed отражает "сборка.тип": "оконное" - исполняемый файл для
// оконной подсистемы Windows: при запуске не открывается консоль. На
// Linux/macOS не отличается от "консольное".
func (pf *File) IsWindowed() bool {
	return pf.Тип == buildTypeWindowed
}

// KeepC отражает "сборка.режим": "отладка" оставляет сгенерированный .c
// рядом с бинарником (как --keep-c), "релиз" - убирает его.
func (pf *File) KeepC() bool {
	return pf.Режим == "отладка"
}

// platformOSNames - "сборка.платформа" -> список ast.PlatformNames,
// которым должен соответствовать хост компилятора (ast.CurrentPlatform()),
// чтобы сборка была разрешена. "текущая" -> nil (любой хост, без
// ограничения - значение по умолчанию, ведёт себя как раньше). "unix" -
// класс из двух конкретных платформ (Linux ИЛИ MacOS, не Windows), а не
// отдельный член перечисления "ОС" - "#если ОС.<Член>"/`текущая_ос()`
// по-прежнему знают только три конкретные платформы (internal/ast,
// PlatformNames). Ключи этой таблицы - одновременно единственный источник
// допустимых значений поля (Load проверяет через неё же).
var platformOSNames = map[string][]string{
	"текущая": nil,
	"linux":   {"Linux"},
	"windows": {"Windows"},
	"macos":   {"MacOS"},
	"unix":    {"Linux", "MacOS"},
}

// MatchesHostPlatform - true, если объявленная "сборка.платформа"
// допускает хост, на котором сейчас реально работает компилятор
// (ast.CurrentPlatform()) - всегда true для "текущая" (нет ограничения).
func (pf *File) MatchesHostPlatform() bool {
	names := platformOSNames[pf.Платформа]
	return names == nil || slices.Contains(names, ast.CurrentPlatform())
}

// CheckHostPlatform - понятная ошибка, если объявленная "сборка.платформа"
// не допускает текущий хост - "rech build"/"rech run" вызывают её сразу
// после Load, до компиляции: явная остановка вместо молчаливой сборки под
// чужую ОС (например, WinAPI-программы на Linux). Настоящей
// кросс-компиляции пока нет - C-компилятор ищется на PATH ЭТОГО ЖЕ хоста
// (internal/toolchain, findCCompiler), поэтому единственный работающий
// вариант сегодня - собирать на подходящей ОС самому. Формулировка "пока"
// - осознанный задел: когда появится настоящий кросс-toolchain (например,
// mingw-w64 для цели Windows с хоста Linux), эта же точка (единственное
// место проверки хоста) заменится на вызов кросс-сборки вместо ошибки, а
// не потребует менять формат/значения самого поля "сборка.платформа".
func (pf *File) CheckHostPlatform() error {
	if pf.MatchesHostPlatform() {
		return nil
	}
	return fmt.Errorf("сборка.платформа %q не допускает текущий хост (%s) - кросс-компиляция пока не реализована, соберите проект на подходящей ОС", pf.Платформа, ast.CurrentPlatform())
}

// splitCommaList разбирает raw через запятую, обрезая пробелы и
// пропуская пустые элементы - общая часть Библиотеки/ПутиБиблиотек ниже
// (и их платформенных вариантов) - nil, если raw == "".
func splitCommaList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// platformLibrariesRaw/platformLibraryPathsRaw - librariesWindowsRaw/
// librariesLinuxRaw/librariesMacosRaw (и путевой аналог) по ТЕКУЩЕЙ
// платформе (ast.CurrentPlatform()) - см. package doc полей выше.
func (pf *File) platformLibrariesRaw() string {
	return pf.platformField(pf.librariesWindowsRaw, pf.librariesLinuxRaw, pf.librariesMacosRaw)
}

func (pf *File) platformLibraryPathsRaw() string {
	return pf.platformField(pf.libraryPathsWindowsRaw, pf.libraryPathsLinuxRaw, pf.libraryPathsMacosRaw)
}

func (pf *File) platformField(windows, linux, macos string) string {
	switch ast.CurrentPlatform() {
	case "Windows":
		return windows
	case "MacOS":
		return macos
	default:
		return linux
	}
}

func (pf *File) platformHeadersRaw() string {
	return pf.platformField(pf.headersWindowsRaw, pf.headersLinuxRaw, pf.headersMacosRaw)
}

// Заголовки разбирает "сборка.заголовки" (через запятую, "windows.h,
// shellapi.h"), плюс "сборка.заголовки_windows"/"_linux"/"_macos" ТЕКУЩЕЙ
// платформы - тем же принципом, что и Библиотеки ниже, но для C-заголовков
// (`#include <...>` в сгенерированном файле), не библиотек для линковки.
// Каждый настоящий заголовок делает видимым НАСТОЯЩИЙ прототип для ЛЮБОГО
// "внешняя функция"/"внешняя структура" программы - cbackend больше не
// пишет для них свой собственный (docs/SPEC.md, раздел 12, "Внешние
// функции (FFI)") - при несовпадении сигнатуры настоящая ошибка всплывёт
// при компиляции сгенерированного C, как и у любого обычного FFI-биндинга.
// nil, если ни одно поле не указано (безусловно "как сейчас" -
// компилятор сам пишет собственный прототип по типам объявления).
func (pf *File) Заголовки() []string {
	out := splitCommaList(pf.headersRaw)
	return append(out, splitCommaList(pf.platformHeadersRaw())...)
}

// Библиотеки разбирает "сборка.библиотеки" (через запятую, "curl, ssl"),
// плюс "сборка.библиотеки_windows"/"_linux"/"_macos" ТЕКУЩЕЙ платформы
// (docs/SPEC.md, "#если ОС.<Член>") - внешние библиотеки для линковки
// (-lcurl -lssl) при сборке. "внешняя функция" в .rech-коде объявляет
// только сигнатуру, без привязки к конкретной библиотеке - компилятор
// безусловно линкует весь список, не пытаясь выяснить, какая внешняя
// функция из какой библиотеки (тот же принцип, что и у -lm сейчас:
// линковка неиспользуемой библиотеки безвредна). nil, если ни одно поле
// не указано.
func (pf *File) Библиотеки() []string {
	out := splitCommaList(pf.librariesRaw)
	return append(out, splitCommaList(pf.platformLibrariesRaw())...)
}

// ПутиБиблиотек разбирает "сборка.путь_библиотек" (через запятую, тем же
// принципом, что и Библиотеки выше) - локальные каталоги, где искать
// файлы библиотек из Библиотеки(), КРОМЕ стандартных системных путей
// линковщика (docs/SPEC.md, "Экспорт в разделяемую библиотеку" - без
// этого поля потребитель библиотеки, написанной на Rech и лежащей рядом,
// а не установленной в систему, не мог собраться вовсе без ручного
// LIBRARY_PATH/LD_LIBRARY_PATH). Путь БЕЗ абсолютного корня - относительно
// каталога САМОГО .rechproj (pf.Dir), не текущего рабочего каталога
// процесса, тем же принципом, что и "файлы.главный"/"файлы.выход" -
// работает одинаково, откуда бы ни был запущен "rech build/run". Путь С
// абсолютным корнем - как есть, БЕЗ присоединения к pf.Dir (в отличие от
// MainPath/OutputDir выше - там абсолютный путь бессмыслен, здесь же
// осмысленный сценарий - ОБЩИЙ каталог библиотек, один на несколько
// проектов сразу, не обязанный лежать где-то внутри/рядом с КАЖДЫМ из
// них). Результат всегда АБСОЛЮТНЫЙ (filepath.Abs, даже если сам pf.Dir
// относительный - "rech build ./проект.rechproj" из текущего каталога) -
// этот путь эмитится линковщиком не только как "-L" (нужен только во
// время ЭТОЙ сборки, относительный уже не был бы проблемой), но и как
// "-Wl,-rpath,<путь>" (internal/toolchain), зашитый В САМ СОБРАННЫЙ
// БИНАРНИК - относительный rpath резолвится ОС относительно РАБОЧЕГО
// КАТАЛОГА ПРОЦЕССА В МОМЕНТ ЗАПУСКА (не относительно каталога, где лежит
// сам бинарник, и НЕ относительно каталога, где он собирался) - будучи
// относительным, он был бы правильным только при случайном совпадении
// текущего каталога с тем, что был при сборке, и ломался бы при запуске
// бинарника из любого другого места. nil, если поле не указано.
func (pf *File) ПутиБиблиотек() []string {
	raw := splitCommaList(pf.libraryPathsRaw)
	raw = append(raw, splitCommaList(pf.platformLibraryPathsRaw())...)
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if !filepath.IsAbs(p) {
			p = filepath.Join(pf.Dir, p)
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		out = append(out, p)
	}
	return out
}

// DBPoolSize разбирает "бд.размер_пула" - число одновременных соединений с
// БД в пуле ("подключиться_к_бд" открывает СРАЗУ столько соединений, не
// одно - запросы из разных потоков/задач идут параллельно, а не в очередь
// друг за другом через общий мьютекс на одно соединение). 0, nil - поле не
// указано (или .rechproj нет вовсе) - вызывающая сторона (internal/
// compiler) в этом случае НЕ переопределяет ir.Program.DBPoolSize,
// оставляя её собственное значение по умолчанию (internal/semantic.
// DefaultDBPoolSize) - тем же принципом "0 - используй встроенное
// умолчание", что и nil у Библиотеки()/Зависимости. Ошибка - значение
// указано, но не положительное целое (отрицательный/нулевой пул или пул
// на 1000000 соединений одинаково бессмысленны для однопроцессной
// программы - гарантированно не то, что имел в виду программист, поэтому
// ошибка компиляции, не молчаливое усечение).
func (pf *File) DBPoolSize() (int, error) {
	if pf.dbPoolSizeRaw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(pf.dbPoolSizeRaw)
	if err != nil || n <= 0 || n > 1000 {
		return 0, fmt.Errorf("бд.размер_пула %q - ожидалось целое число от 1 до 1000", pf.dbPoolSizeRaw)
	}
	return n, nil
}

// DBPoolTimeoutMs разбирает "бд.таймаут_пула_мс" - сколько миллисекунд
// "запросить"/"выполнить" ждут свободное соединение из пула БД (Шаг 66),
// прежде чем сдаться (rtmsg.DBCheckoutTimeout, вместо бесконечного
// ожидания). В отличие от DBPoolSize выше, "0" здесь - НЕ сентинел
// "значение не указано, использовать умолчание": 0 сам по себе и есть
// содержательное умолчание ("ждать бесконечно", текущее поведение без
// этой настройки вовсе) - отсутствие поля в файле проекта и явное "0"
// неотличимы и не обязаны быть отличимы. Ошибка - только отрицательное
// значение (бессмысленно как таймаут).
func (pf *File) DBPoolTimeoutMs() (int, error) {
	if pf.dbPoolTimeoutMsRaw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(pf.dbPoolTimeoutMsRaw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("бд.таймаут_пула_мс %q - ожидалось неотрицательное целое число (0 - ждать бесконечно)", pf.dbPoolTimeoutMsRaw)
	}
	return n, nil
}

// DBSQLiteBusyTimeoutMs разбирает "бд.sqlite_таймаут_блокировки_мс" -
// сколько миллисекунд SQLite сама ждёт снятия блокировки файла ДРУГИМ
// соединением/процессом (sqlite3_busy_timeout/DSN "?_busy_timeout=") - не
// то же самое, что DBPoolTimeoutMs выше (то ожидание СОБСТВЕННОГО пула
// соединений этого рантайма, это - встроенное в саму SQLite ожидание
// файловой блокировки). 0/пусто - поле не указано, использовать встроенное
// умолчание (internal/semantic.DefaultDBSQLiteBusyTimeoutMs, 5000) - тем же
// принципом "0 - используй умолчание", что и у DBPoolSize (не у
// DBPoolTimeoutMs - явное "0" здесь НЕ поддержано как отдельное значение
// "не ждать вовсе", хотя sqlite3_busy_timeout(db, 0) формально это умеет -
// осознанное упрощение, тот же компромисс, что и у DBPoolSize с диапазоном
// 1..1000). Ошибка - значение указано, но не положительное целое.
func (pf *File) DBSQLiteBusyTimeoutMs() (int, error) {
	if pf.dbSQLiteBusyTimeoutMsRaw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(pf.dbSQLiteBusyTimeoutMsRaw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("бд.sqlite_таймаут_блокировки_мс %q - ожидалось положительное целое число", pf.dbSQLiteBusyTimeoutMsRaw)
	}
	return n, nil
}

// PoolShrinkEnabled разбирает "потоки.сжатие_пула" - управляет тем же
// пулом воркер-потоков, что обслуживает "запустить" (internal/backend/
// cbackend/threadpool.go): по умолчанию (поле не указано, "истина" или
// значение пустое) воркер, простоявший без работы дольше
// PoolIdleTimeoutMs, завершается сам (Шаг 81) - "ложь"
// ВЫКЛЮЧАЕТ сжатие: пул растёт до пиковой конкурентности и НИКОГДА не
// сжимается обратно (поведение Шага 80, до появления этой настройки).
// Явное значение, отличное от "истина"/"ложь" - ошибка (тот же принцип,
// что и у "сборка.режим"/"сборка.тип" - опечатка в булевом флаге не
// должна молча трактоваться как одно из двух значений).
func (pf *File) PoolShrinkEnabled() (bool, error) {
	switch pf.poolShrinkRaw {
	case "", "истина":
		return true, nil
	case "ложь":
		return false, nil
	default:
		return false, fmt.Errorf("потоки.сжатие_пула %q - ожидалось \"истина\" или \"ложь\"", pf.poolShrinkRaw)
	}
}

// PoolIdleTimeoutMs разбирает "потоки.таймаут_простоя_мс" - сколько
// миллисекунд воркер пула (см. PoolShrinkEnabled) простаивает без работы,
// прежде чем завершиться, когда сжатие включено. Имеет значение, ТОЛЬКО
// если "потоки.сжатие_пула" - "истина" (без сжатия воркер никогда не
// завершается, таймаут ожидания не нужен вовсе). 0, nil - поле не указано,
// вызывающая сторона (internal/compiler) в этом случае не переопределяет
// ir.Program.PoolIdleTimeoutMs, оставляя встроенное умолчание
// (internal/semantic.DefaultPoolIdleTimeoutMs) - тот же принцип "0 -
// используй встроенное умолчание", что и у DBPoolSize. Ошибка - значение
// указано, но не положительное целое (нулевой/отрицательный таймаут
// простоя бессмыслен - см. DBPoolSize).
func (pf *File) PoolIdleTimeoutMs() (int, error) {
	if pf.poolIdleTimeoutMsRaw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(pf.poolIdleTimeoutMsRaw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("потоки.таймаут_простоя_мс %q - ожидалось положительное целое число", pf.poolIdleTimeoutMsRaw)
	}
	return n, nil
}

// HTTPMaxRequestBytes разбирает "http.макс_размер_запроса_байт" - предел
// размера входящего HTTP-запроса ("принять_http"), заголовки и тело вместе
// (rtmsg.HTTPServerRequestTooLarge, вместо буферизации сколь угодно
// большого запроса в памяти без предела). 0, nil - поле не указано (или
// .rechproj нет вовсе) - вызывающая сторона (internal/compiler) в этом
// случае НЕ переопределяет ir.Program.HTTPMaxRequestBytes, оставляя
// встроенное умолчание (internal/semantic.DefaultHTTPMaxRequestBytes), тем
// же принципом "0 - используй встроенное умолчание", что и у DBPoolSize.
// Ошибка - значение указано, но не положительное целое (явный "0" здесь не
// поддержан как отдельное значение "без предела" - неотличим от "поле не
// указано", тем же осознанным упрощением, что и у DBPoolSize; если
// программе действительно нужен очень большой предел, можно указать
// большое число явно).
func (pf *File) HTTPMaxRequestBytes() (int, error) {
	if pf.httpMaxRequestBytesRaw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(pf.httpMaxRequestBytesRaw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("http.макс_размер_запроса_байт %q - ожидалось положительное целое число", pf.httpMaxRequestBytesRaw)
	}
	return n, nil
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
  # тип: консольное (по умолчанию; "оконное" - без консоли на Windows,
  # "библиотека" - разделяемая библиотека вместо исполняемого файла)
  # библиотеки: curl, ssl (внешние библиотеки для линковки, через запятую)
  # путь_библиотек: ../моя_библиотека/сборка (локальные каталоги для поиска
  # библиотек из "библиотеки:" выше, через запятую - для тех, что НЕ
  # установлены в систему, например собственная Rech-библиотека рядом -
  # см. docs/SPEC.md, "Экспорт в разделяемую библиотеку"; относительно
  # каталога ЭТОГО .rechproj, не текущего рабочего каталога)
  #
  # Компилятор C и его флаги. Пока поля ниже закомментированы, сборка
  # вызывает компилятор так:
  #   <компилятор_c> [флаги_c] <файл.c> -o <выход> <флаги_линковки>
  # компилятор_c - переменная окружения CC, иначе первый из cc/clang/gcc
  # в PATH; флагов C нет вовсе (без оптимизации); флаги линковки
  # подбираются сами по тому, что использует программа (список ниже), плюс
  # "-L<путь> -l<имя>" из путь_библиотек/библиотеки выше. Поле, которое
  # есть в файле, ПОЛНОСТЬЮ заменяет встроенное - перечислите в нём всё
  # нужное: компилятор сам добавит только "<файл.c> -o <выход>". Флаги -
  # через пробел, "..." - один флаг с пробелом внутри (путь). Варианты
  # _windows/_linux/_macos добавляются к общему полю на своей платформе
  # (компилятор_c_<платформа> - заменяет общий).
  # компилятор_c: gcc
  # флаги_c: -O2
  # флаги_линковки: -lm -lgc
  # флаги_линковки_linux: -pthread
  # флаги_линковки_macos: -pthread
  # флаги_линковки_windows: -lshell32 -lws2_32
  # Встроенные флаги линковки и когда компилятор ставит их сам:
  #   -lm                 математика (корень, степень, округлить...) - всегда
  #   -lgc                сборщик мусора Boehm GC - сущности, коллекции, строки
  #   -pthread            потоки - запустить, Блокировка, Семафор, пул БД (не Windows)
  #   -lsqlite3           БД SQLite
  #   -lpq                БД PostgreSQL
  #   -lmysqlclient       БД MySQL
  #   -lssl -lcrypto      TLS (слушать_tcp_защищённо, https, grpcs)
  #   -lcrypto            шифрование, хеши, подписи без TLS
  #   -lnghttp2           HTTP/2 и gRPC
  #   -lz                 gzip-сжатие gRPC-сообщений
  #   -lpng               PNG-изображения
  #   -ljpeg              JPEG-изображения
  #   -lshell32 -lws2_32  Windows: аргументы командной строки и сокеты - всегда
  # Для "тип: библиотека" встроенные флаги линковки начинаются с
  #   -shared -fPIC -fvisibility=hidden (Linux), -dynamiclib -fvisibility=hidden
  #   (macOS), -shared (Windows) - при своих флаги_линковки их тоже нужно
  #   написать самому.

# бд:
#   размер_пула: 8 (число одновременных соединений с БД - "подключиться_к_бд"
#   открывает СРАЗУ столько соединений, запросы из разных потоков идут
#   параллельно, а не в очередь через одно общее соединение; по умолчанию - 4)
#   таймаут_пула_мс: 500 (сколько ждать свободное соединение из пула, прежде
#   чем сдаться - по умолчанию 0, ждать бесконечно)
#   sqlite_таймаут_блокировки_мс: 5000 (сколько сама SQLite ждёт снятия
#   блокировки файла другим соединением/процессом - по умолчанию 5000)

# потоки:
#   сжатие_пула: ложь (пул воркер-потоков, обслуживающий "запустить", по
#   умолчанию сжимается - простаивающий воркер завершается сам; "ложь"
#   выключает сжатие: пул растёт до пиковой конкурентности и никогда не
#   сжимается обратно)
#   таймаут_простоя_мс: 30000 (сколько миллисекунд воркер простаивает без
#   работы, прежде чем завершиться - только пока сжатие не выключено,
#   по умолчанию 30000)

# http:
#   макс_размер_запроса_байт: 1048576 (предел размера входящего HTTP-
#   запроса - "принять_http", заголовки и тело вместе - по умолчанию 10
#   МиБ; запрос сверх предела аварийно завершает "принять_http", вместо
#   буферизации сколь угодно большого запроса в памяти без предела)

# окружение:
#   RECH_HTTP_PORT: 8080 (переменные окружения для "rech run" - вместо
#   "ПЕРЕМЕННАЯ=значение rech run" перед каждым запуском)

# опции:
#   единственная секция, где разрешён НАСТОЯЩИЙ YAML (произвольная
#   вложенность, списки, кавычки - в отличие от остального этого файла) -
#   компилятор её не разбирает и не проверяет вовсе, только отдаёт текстом
#   через встроенную "опции_проекта(): Строка", обычно сразу в "разобрать
#   YAML опции_проекта() как <Тип>" (docs/SPEC.md, раздел 18). Пример:
#   почта:
#     хост: smtp.example.com
#     порт: 587
#   "установить_опции_проекта(текст)" меняет то, что вернёт следующий
#   "опции_проекта()" ТОЛЬКО в памяти текущего запуска, не в этом файле
#   (как ini_set в PHP) - см. docs/SPEC.md, раздел 12.3.

# зависимости:
#   имя: https://github.com/пользователь/репозиторий v1.2.0 (URL и версия
#   через пробел - тег/ветка/коммит; "имя" - как зависимость видна в
#   "использовать ... из" внутри исходников проекта)
`
