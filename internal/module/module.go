// Package module разрешает "использовать <имена> из "<путь>"" (шаг 11,
// docs/ROADMAP.md) в связный многофайловый граф программ Rech: находит и
// разбирает файлы, на которые ссылается путь импорта (рекурсивно, включая
// импорты импортированных файлов), проверяет, что каждое импортируемое имя
// действительно объявлено в целевом файле, и обнаруживает циклы.
//
// Пакет не знает ни про C, ни про IR - как internal/ast, он лежит "ниже"
// backend'ов (internal/backend/cbackend, internal/semantic), которые
// используют Program.Link (link.go) для получения одной объединённой
// internal/ast.Program с уже переименованными (во избежание коллизий имён
// между независимыми файлами) именами - им не нужно знать про модули и
// импорты вообще, только про уже слинкованный ast.Program, как и раньше.
package module

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// Frontend превращает исходный текст на Rech в AST - то же самое, что
// internal/compiler.Frontend (структурно совместимо, отдельный тип здесь
// только для того, чтобы этот пакет не зависел от internal/compiler -
// зависимость должна идти в обратную сторону).
type Frontend interface {
	Parse(src string) (*ast.Program, error)
}

// segmentNameRe - разрешённый алфавит одного сегмента пути импорта:
// строчные буквы (кириллица или латиница), цифры, "_" - тот же алфавит, что
// и у имени .rech-файла (internal/compiler.sourceFileNameRe), потому что
// сегмент пути импорта либо и есть имя файла (последний сегмент), либо имя
// каталога, к которому применяется то же самое ограничение регистра ровно
// по той же причине (нечувствительные к регистру файловые системы).
var segmentNameRe = regexp.MustCompile(`^[a-zа-яё0-9_]+$`)

// ValidSegmentName проверяet один сегмент пути импорта (часть строки между
// "/" или сама строка целиком, если "/" нет) - тот же алфавит и та же форма
// NFC, что и у имени .rech-файла.
func ValidSegmentName(segment string) error {
	if segment == "" {
		return fmt.Errorf("пустой сегмент пути")
	}
	if !norm.NFC.IsNormalString(segment) {
		return fmt.Errorf("%q не в нормализованной форме Unicode NFC", segment)
	}
	if !segmentNameRe.MatchString(segment) {
		return fmt.Errorf("%q должен состоять только из строчных букв (кириллица или латиница), цифр и \"_\"", segment)
	}
	return nil
}

// Qualifier выводит идентификатор модуля из имени файла (без расширения) -
// используется как квалификатор в будущем "Модуль.Имя" (не реализовано в
// этом срезе - см. docs/ROADMAP.md, Шаг 11, "Пространства имён"): каждый
// "_"-сегмент имени файла делается заглавным и склеивается без разделителя
// ("коды_ответов" -> "КодыОтветов") - то же правило, которым пользователь
// описал желаемый вид пространства имён.
func Qualifier(baseName string) string {
	parts := strings.Split(baseName, "_")
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		r := []rune(part)
		b.WriteString(strings.ToUpper(string(r[0])))
		b.WriteString(string(r[1:]))
	}
	return b.String()
}

// Import - одно имя, попавшее в область видимости Module через
// "использовать" - Alias - имя, под которым Name доступно в файле, куда
// сделан импорт (Alias == Name без "как"), Name - как оно объявлено в
// Source. Source - модуль, где оно на самом деле объявлено.
type Import struct {
	Alias  string
	Name   string
	Source *Module
}

// Module - один загруженный .rech-файл вместе с его разрешёнными
// импортами.
type Module struct {
	// Path - очищенный (filepath.Clean) путь к файлу, использованный при
	// загрузке - идентичность модуля для обнаружения циклов/повторной
	// загрузки, не обязательно абсолютный.
	Path      string
	Qualifier string
	AST       *ast.Program
	Imports   []Import
}

// declaredNames - имена, которые Module объявляет на верхнем уровне и
// которые можно импортировать через "использовать" - функции и сущности.
// Производные типы состояний (StateDecl.TypeName) сюда не входят - они
// пока не участвуют в системе импортов, только localTypeNames ниже (для
// ссылок внутри того же файла, который их объявил).
func declaredNames(prog *ast.Program) map[string]bool {
	names := map[string]bool{}
	for _, fn := range prog.Functions {
		names[fn.Name] = true
	}
	for _, ent := range prog.Entities {
		names[ent.Name] = true
	}
	for _, ext := range prog.ExternFuncs {
		names[ext.Alias] = true
	}
	for _, ext := range prog.ExternEntities {
		names[ext.Alias] = true
	}
	return names
}

// moduleVisibility - то же множество имён, что и declaredNames, но с
// видимостью каждого (ast.FunctionDecl.Visibility/ast.EntityDecl.
// Visibility - необязательный "приватный"/"публичный" перед объявлением,
// docs/SPEC.md, "Импорты") - используется ТОЛЬКО для проверки
// "использовать" (в отличие от declaredNames, которая нужна и для проверки
// коллизии ЛОКАЛЬНЫХ имён, где видимость не имеет значения - см. её вызов
// в load ниже, declared). Внешние функции/сущности не поддерживают
// модификатор видимости - всегда Public.
func moduleVisibility(prog *ast.Program) map[string]ast.Visibility {
	vis := map[string]ast.Visibility{}
	for _, fn := range prog.Functions {
		vis[fn.Name] = fn.Visibility
	}
	for _, ent := range prog.Entities {
		vis[ent.Name] = ent.Visibility
	}
	for _, ext := range prog.ExternFuncs {
		vis[ext.Alias] = ast.Public
	}
	for _, ext := range prog.ExternEntities {
		vis[ext.Alias] = ast.Public
	}
	return vis
}

// localTypeNames - declaredNames плюс производные типы состояний
// (StateDecl.TypeName) и интерфейсы (InterfaceDecl.Name) - используется
// для разрешения ссылок на тип внутри самого файла, который их объявил
// (link.go), а не для проверки "использовать" (там - только
// declaredNames): ни производные типы состояний, ни интерфейсы пока не
// участвуют в системе импортов, но должны резолвиться как тип параметра/
// поля/результата в пределах одного файла, даже когда файл вообще не
// содержит "использовать" - Link() всё равно проходит по нему.
func localTypeNames(prog *ast.Program) map[string]bool {
	names := declaredNames(prog)
	for _, ent := range prog.Entities {
		for _, st := range ent.States {
			names[st.TypeName] = true
		}
	}
	for _, iface := range prog.Interfaces {
		names[iface.Name] = true
	}
	return names
}

// Program - весь загруженный многофайловый граф: Modules - в порядке
// топологической сортировки (зависимости раньше зависящих от них модулей),
// Entry - модуль, с которого начиналась загрузка.
type Program struct {
	Modules []*Module
	Entry   *Module
}

// Load загружает entryPath и рекурсивно все файлы, на которые он (и они
// сами, транзитивно) ссылаются через "использовать", разрешая пути
// относительно каталога того файла, где написано "использовать". Циклы
// (файл A использует B, B прямо или косвенно использует A) - ошибка
// компиляции: у Rech нет предварительных объявлений ни для функций/сущностей
// одного файла (сигнатуры собираются одним проходом, шаг 3.4/4), ни тем
// более между файлами, поэтому цикл в принципе не может быть корректно
// упорядочен для сбора сигнатур - в отличие от Python, где циклический
// импорт иногда работает (в зависимости от того, что именно используется до
// полной инициализации модуля), здесь это всегда явная ошибка, а не
// хрупкое "может сработать, а может и нет".
func Load(entryPath string, frontend Frontend) (*Program, error) {
	return LoadWithDeps(entryPath, frontend, nil)
}

// LoadWithDeps - Load с внешними зависимостями проекта (deps: имя
// зависимости, объявленное в .rechproj/"зависимости:" -> локальный каталог
// с уже скачанными исходниками, см. internal/pkgfetch) - "использовать ...
// из "<имя>/<путь>"" резолвится в файл ВНУТРИ этого каталога, если <имя> -
// объявленная зависимость, иначе как раньше (относительно каталога файла,
// где написано "использовать"). nil - как обычный Load (deps ни у одной
// программы, привычное поведение без .rechproj/без внешних зависимостей).
// БЕЗ транзитивных зависимостей (см. LoadWithGraph) - "использовать" внутри
// файла САМОЙ зависимости не может сослаться на ЕЁ собственную
// зависимость по имени, только относительными путями внутри неё самой.
func LoadWithDeps(entryPath string, frontend Frontend, deps map[string]string) (*Program, error) {
	return LoadWithGraph(entryPath, frontend, deps, nil)
}

// LoadWithGraph - LoadWithDeps с ПОЛНЫМ графом зависимостей, включая
// транзитивные (internal/pkgfetch.Graph.Scopes: каталог зависимости -> её
// СОБСТВЕННАЯ карта имён). Когда разбор "использовать" внутри файла ОДНОЙ
// зависимости встречает имя, объявленное КАК ЕЁ СОБСТВЕННАЯ зависимость,
// резолюция продолжается по ЕЁ карте (scopes[тот каталог]), а не по карте
// программы верхнего уровня - имя зависимости приватно для проекта,
// который его объявил (тот же принцип, что и у алиаса "использовать ...
// как ..."), не общий на всю программу неймспейс. scopes == nil - как
// LoadWithDeps (rootDeps без транзитивной части).
func LoadWithGraph(entryPath string, frontend Frontend, rootDeps map[string]string, scopes map[string]map[string]string) (*Program, error) {
	l := &loader{frontend: frontend, loaded: map[string]*Module{}, inProgress: map[string]bool{}, scopes: scopes}
	entry, err := l.load(filepath.Clean(entryPath), nil, rootDeps)
	if err != nil {
		return nil, err
	}
	return &Program{Modules: l.order, Entry: entry}, nil
}

type loader struct {
	frontend   Frontend
	loaded     map[string]*Module
	inProgress map[string]bool
	order      []*Module
	// scopes - весь граф "каталог зависимости -> её карта имён" (см.
	// package doc LoadWithGraph) - deps, параметр load, несёт карту ИМЕННО
	// для файла, который сейчас обрабатывается (у корневой программы - это
	// rootDeps; после перехода в файл зависимости - её запись в scopes).
	scopes map[string]map[string]string
}

func (l *loader) load(path string, stack []string, deps map[string]string) (*Module, error) {
	if m, ok := l.loaded[path]; ok {
		return m, nil
	}
	if l.inProgress[path] {
		return nil, fmt.Errorf("циклический импорт: %s -> %s", strings.Join(stack, " -> "), path)
	}
	l.inProgress[path] = true
	defer delete(l.inProgress, path)

	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл %q: %w", path, err)
	}
	prog, err := l.frontend.Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ast.StampFile(prog, path)

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	m := &Module{Path: path, Qualifier: Qualifier(base), AST: prog}
	declared := declaredNames(prog)

	for _, use := range prog.Uses {
		targetPath, crossedInto, err := resolveImportPath(filepath.Dir(path), use.Path, deps)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", use.Line, err)
		}
		nextDeps := deps
		if crossedInto != "" {
			nextDeps = l.scopes[crossedInto]
		}
		target, err := l.load(targetPath, append(append([]string{}, stack...), path), nextDeps)
		if err != nil {
			return nil, err
		}
		targetVisibility := moduleVisibility(target.AST)
		for _, un := range use.Names {
			vis, exists := targetVisibility[un.Name]
			if !exists {
				return nil, fmt.Errorf("строка %d: файл %q не объявляет %q", un.Line, use.Path, un.Name)
			}
			if vis == ast.Private {
				return nil, fmt.Errorf("строка %d: %q в файле %q приватное - недоступно для \"использовать\" (объявлено с модификатором \"приватный\"/\"приватная\")", un.Line, un.Name, use.Path)
			}
			if declared[un.Alias] {
				return nil, fmt.Errorf("строка %d: имя %q уже объявлено в этом файле - используйте \"как\" для переименования при импорте", un.Line, un.Alias)
			}
			for _, existing := range m.Imports {
				if existing.Alias == un.Alias {
					return nil, fmt.Errorf("строка %d: имя %q уже импортировано (из %q) - используйте \"как\" для переименования одного из импортов",
						un.Line, un.Alias, existing.Source.Path)
				}
			}
			m.Imports = append(m.Imports, Import{Alias: un.Alias, Name: un.Name, Source: target})
		}
	}

	l.loaded[path] = m
	l.order = append(l.order, m)
	return m, nil
}

// resolveImportPath разрешает путь импорта ("утилиты/http/коды_ответов",
// без расширения) в файловый путь. Каждый сегмент проверяется тем же
// правилом, что и имя файла (ValidSegmentName) - строчные буквы/цифры/"_",
// NFC - поэтому "." и ".." как сегменты уже отклонены самим алфавитом,
// отдельно исключать их не нужно.
//
// Если самый ПЕРВЫЙ сегмент совпадает с именем внешней зависимости из deps
// (см. package doc LoadWithDeps) - путь разрешается ВНУТРИ каталога этой
// зависимости, а не относительно baseDir ("использовать Клиент из
// "клиент/файл"" - "клиент" здесь имя зависимости из .rechproj, "файл" -
// путь внутри её скачанных исходников), и crossedInto возвращает этот
// каталог (см. package doc LoadWithGraph - вызывающая сторона должна
// продолжить резолюцию ВНУТРИ него уже по ЕГО собственной карте имён, не
// по deps). Иначе - как раньше, относительно baseDir (каталог файла, где
// написано "использовать"), crossedInto == "".
func resolveImportPath(baseDir, importPath string, deps map[string]string) (path, crossedInto string, err error) {
	segments := strings.Split(importPath, "/")
	for _, seg := range segments {
		if err := ValidSegmentName(seg); err != nil {
			return "", "", fmt.Errorf("путь импорта %q: %w", importPath, err)
		}
	}
	root := baseDir
	rest := segments
	if depDir, ok := deps[segments[0]]; ok {
		root = depDir
		crossedInto = depDir
		rest = segments[1:]
		if len(rest) == 0 {
			return "", "", fmt.Errorf("путь импорта %q: после имени зависимости %q ожидался путь к файлу внутри неё", importPath, segments[0])
		}
	}
	parts := append([]string{root}, rest...)
	return filepath.Join(parts...) + ".rech", crossedInto, nil
}
