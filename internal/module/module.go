// Package module разрешает "использовать <имена> из "<путь>"" (шаг 11,
// ) в связный многофайловый граф программ Rech: находит и
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
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
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
// этом срезе, "Пространства имён"): каждый
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
	// StdImports - встроенные имена, импортированные из стандартных
	// модулей ("использовать знак из стандарт/математика"): Alias - как
	// написано в файле, Name - настоящее встроенное имя.
	StdImports []StdImport
}

// StdImport - одно имя из стандартного модуля (internal/stdlib).
type StdImport struct {
	Alias string
	Name  string
}

// declaredNames - имена, которые Module объявляет на верхнем уровне и
// которые можно импортировать через "использовать" - функции, сущности,
// интерфейсы, перечисления и константы уровня модуля. Производные типы
// состояний (StateDecl.TypeName) сюда не входят - они пока не участвуют в
// системе импортов, только localTypeNames ниже (для ссылок внутри того же
// файла, который их объявил).
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
	for _, iface := range prog.Interfaces {
		names[iface.Name] = true
	}
	for _, e := range prog.Enums {
		names[e.Name] = true
	}
	for _, c := range prog.Consts {
		names[c.Name] = true
	}
	for _, v := range prog.Vars {
		names[v.Name] = true
	}
	for _, s := range prog.Synonyms {
		names[s.Name] = true
	}
	return names
}

// moduleVisibility - то же множество имён, что и declaredNames, но с
// видимостью каждого (ast.FunctionDecl.Visibility/ast.EntityDecl.
// Visibility/ast.EnumDecl.Visibility/ast.ConstDecl.Visibility -
// необязательный "приватный"/"публичный" перед объявлением, согласованный
// по роду с governing-словом - internal/parser/modvisibility.go) -
// используется ТОЛЬКО для проверки "использовать" (в отличие от
// declaredNames, которая нужна и для проверки коллизии ЛОКАЛЬНЫХ имён, где
// видимость не имеет значения - см. её вызов в load ниже, declared).
// Внешние функции/сущности и интерфейсы не поддерживают модификатор
// видимости вовсе (ast.InterfaceDecl не несёт поля Visibility) - всегда
// Public.
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
	for _, iface := range prog.Interfaces {
		vis[iface.Name] = ast.Public
	}
	for _, e := range prog.Enums {
		vis[e.Name] = e.Visibility
	}
	for _, c := range prog.Consts {
		vis[c.Name] = c.Visibility
	}
	for _, v := range prog.Vars {
		vis[v.Name] = v.Visibility
	}
	for _, s := range prog.Synonyms {
		vis[s.Name] = s.Visibility
	}
	return vis
}

// localTypeNames - declaredNames (теперь уже включает интерфейсы) плюс
// производные типы состояний (StateDecl.TypeName) - используется для
// разрешения ссылок на тип внутри самого файла, который их объявил
// (link.go), а не для проверки "использовать" (там - только
// declaredNames напрямую): производные типы состояний по-прежнему не
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
	return LoadWithRoot(entryPath, frontend, rootDeps, scopes, "")
}

// LoadWithRoot - LoadWithGraph с явным projectRoot (точка отсчёта "@путь",
// см. package doc resolveImportPath) вместо каталога entryPath по
// умолчанию. "" ведёт себя как LoadWithGraph (каталог entryPath) - этим
// пользуются Load/LoadWithDeps/LoadWithGraph, которым для "rech build
// <главный файл>" каталог entryPath И ЕСТЬ корень программы (он же
// каталог .rechproj/"файлы.главный"). Явный projectRoot нужен вызывающей
// стороне, которая грузит файл, НЕ обязательно являющийся главным файлом
// проекта (LSP - открытый документ на любой глубине дерева, internal/lsp/
// multifile.go; "rech test" на отдельном файле, internal/testrunner) - без
// этого "@путь" внутри такого файла резолвился бы от его СОБСТВЕННОГО
// каталога, а не от каталога главного файла, и попадал бы не туда (module
// сам по-прежнему не знает про .rechproj - вычислить правильный
// projectRoot для такого файла обязана вызывающая сторона, обычно через
// internal/project.RootFor).
func LoadWithRoot(entryPath string, frontend Frontend, rootDeps map[string]string, scopes map[string]map[string]string, projectRoot string) (*Program, error) {
	return LoadWithOverlay(entryPath, frontend, rootDeps, scopes, projectRoot, nil)
}

// LoadWithOverlay - LoadWithRoot, где файлы из overlay (очищенный путь ->
// текст) читаются из памяти, а не с диска: так LSP разбирает граф импортов
// с несохранённым текстом открытого документа.
func LoadWithOverlay(entryPath string, frontend Frontend, rootDeps map[string]string, scopes map[string]map[string]string, projectRoot string, overlay map[string]string) (*Program, error) {
	return loadForPlatform(entryPath, frontend, rootDeps, scopes, projectRoot, overlay, ast.CurrentPlatform())
}

// loadForPlatform - LoadWithOverlay с блоками "#если", разрешёнными под
// platform (одно из ast.PlatformNames), а не под текущую платформу -
// автоимпорт (stdfix.go) проходит код всех платформ.
func loadForPlatform(entryPath string, frontend Frontend, rootDeps map[string]string, scopes map[string]map[string]string, projectRoot string, overlay map[string]string, platform string) (*Program, error) {
	l := &loader{frontend: frontend, loaded: map[string]*Module{}, inProgress: map[string]bool{}, scopes: scopes, overlay: overlay, platform: platform}
	root := projectRoot
	if root == "" {
		root = filepath.Dir(filepath.Clean(entryPath))
	}
	entry, err := l.load(filepath.Clean(entryPath), nil, rootDeps, root)
	if err != nil {
		return nil, err
	}
	return &Program{Modules: l.order, Entry: entry}, nil
}

type loader struct {
	// platform - под какую платформу разрешаются блоки "#если".
	platform   string
	frontend   Frontend
	loaded     map[string]*Module
	inProgress map[string]bool
	order      []*Module
	// scopes - весь граф "каталог зависимости -> её карта имён" (см.
	// package doc LoadWithGraph) - deps, параметр load, несёт карту ИМЕННО
	// для файла, который сейчас обрабатывается (у корневой программы - это
	// rootDeps; после перехода в файл зависимости - её запись в scopes).
	scopes map[string]map[string]string
	// overlay - путь -> текст, который читается вместо файла на диске
	// (LoadWithOverlay: несохранённый буфер редактора).
	overlay map[string]string
}

func (l *loader) load(path string, stack []string, deps map[string]string, root string) (*Module, error) {
	if m, ok := l.loaded[path]; ok {
		return m, nil
	}
	if l.inProgress[path] {
		return nil, fmt.Errorf("циклический импорт: %s -> %s", strings.Join(stack, " -> "), path)
	}
	l.inProgress[path] = true
	defer delete(l.inProgress, path)

	src, ok := l.overlay[path]
	if !ok {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("не удалось прочитать файл %q: %w", path, err)
		}
		src = string(data)
	}
	prog, err := l.frontend.Parse(src)
	if err != nil {
		return nil, diagnostics.WithFile(err, path)
	}
	ast.StampFile(prog, path)
	// "#если ОС.<Член> { ... }" (docs/SPEC.md) - разрешается ЗДЕСЬ, ДО
	// declaredNames ниже - руки, не подошедшие под текущую платформу,
	// никогда не попадают в declared/moduleVisibility/остальной конвейер
	// (см. package doc ast.PlatformBlockDecl/ResolvePlatformBlocks).
	ast.ResolvePlatformBlocks(prog, l.platform)

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	m := &Module{Path: path, Qualifier: Qualifier(base), AST: prog}
	declared := declaredNames(prog)

	for _, use := range prog.Uses {
		if use.Std {
			if err := m.addStdImports(use, declared); err != nil {
				return nil, err
			}
			continue
		}
		targetPath, crossedInto, err := resolveImportPath(filepath.Dir(path), root, use.Path, deps)
		if err != nil {
			return nil, fmt.Errorf("строка %d: %w", use.Line, err)
		}
		nextDeps := deps
		nextRoot := root
		if crossedInto != "" {
			nextDeps = l.scopes[crossedInto]
			// Файл ВНУТРИ зависимости резолвит СВОИ "@путь" от СВОЕГО же
			// корня (каталога, куда она скачана), не от корня программы,
			// которая её подключила - тем же принципом приватности, что и у
			// nextDeps/scopes выше (package doc LoadWithGraph).
			nextRoot = crossedInto
		}
		target, err := l.load(targetPath, append(append([]string{}, stack...), path), nextDeps, nextRoot)
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
			// "внутренний" - виден любому файлу ТОГО ЖЕ .rechproj-проекта, но
			// не файлу, дошедшему до этого "использовать" ЧЕРЕЗ ГРАНИЦУ
			// зависимости (crossedInto != "" - см. package doc resolveImportPath).
			// Проверяется здесь, не раньше (до вычисления crossedInto выше) -
			// именно ОН несёт "эта конкретная использовать-инструкция
			// пересекла границу", не сам путь importPath (тот же путь без
			// имени зависимости в начале не пересёк бы вовсе).
			if vis == ast.Internal && crossedInto != "" {
				return nil, fmt.Errorf("строка %d: %q в файле %q внутреннее - видно только файлам того же проекта, недоступно через границу зависимости (объявлено с модификатором \"внутренний\"/\"внутренняя\")", un.Line, un.Name, use.Path)
			}
			if declared[un.Alias] {
				return nil, fmt.Errorf("строка %d: имя %q уже объявлено в этом файле - используйте \"как\" для переименования при импорте", un.Line, un.Alias)
			}
			if err := m.checkAliasFree(un); err != nil {
				return nil, err
			}
			m.Imports = append(m.Imports, Import{Alias: un.Alias, Name: un.Name, Source: target})
		}
	}

	l.loaded[path] = m
	l.order = append(l.order, m)
	return m, nil
}

// resolveImportPath разрешает путь импорта ("утилиты/http/коды_ответов",
// без расширения) в файловый путь. Каждый сегмент (кроме ведущих ".." -
// см. ниже) проверяется тем же правилом, что и имя файла
// (ValidSegmentName) - строчные буквы/цифры/"_", NFC.
//
// **"@путь"** (абсолютный импорт от корня программы, аналог "baseUrl"/path
// alias в TypeScript) - самый короткий способ дотянуться до файла в
// СОСЕДНЕЙ подпапке (обычный относительный путь
// без "@"/".." не может выйти из своей поддиректории вообще: файл в
// "бд/файл.rech" в принципе не мог дотянуться до "токен/файл.rech"). "@"
// отбрасывается, ОСТАТОК резолвится от projectRoot (не от baseDir) -
// "@токен/токен" из ЛЮБОГО файла программы, на любой глубине, попадает в
// один и тот же файл, не завися от того, где лежит сам импортирующий файл
// (в отличие от ".." ниже - ЧИСЛО "../" пришлось бы менять при переносе
// файла глубже/мельче по дереву). projectRoot - см. package doc
// LoadWithGraph/load (каталог entry-файла ОСНОВНОЙ программы; у файла
// внутри зависимости - каталог САМОЙ этой зависимости, не общий на всю
// программу). "@" не смешивается с именем зависимости - зависимость
// резолвится ТОЛЬКО обычным (не "@") путём, тем же принципом
// "приватности" зависимости для объявившего её проекта, что и у
// deps/scopes.
//
// **"../"** (один и более раз подряд, В НАЧАЛЕ пути) - обычная навигация
// вверх по дереву каталогов ОТНОСИТЕЛЬНО baseDir, тем же смыслом, что и в
// файловых путях большинства языков - каждый "../" поднимается на один
// каталог выше, ПЕРЕД тем, как начнётся обычная (ValidSegmentName)
// часть пути. ".."/"." как отдельные сегменты ГДЕ УГОДНО, КРОМЕ этого
// ведущего префикса (например, "тест/../файл" - "запутывающий" путь,
// не поднимающийся, а петляющий) по-прежнему отклонены самим алфавитом
// ValidSegmentName - только ведущая последовательность "../.../" имеет
// особый смысл.
//
// Если самый ПЕРВЫЙ сегмент ОБЫЧНОГО (без "@", без ведущих "..") пути
// совпадает с именем внешней зависимости из deps (см. package doc
// LoadWithDeps) - путь разрешается ВНУТРИ каталога этой зависимости, а не
// относительно baseDir ("использовать Клиент из "клиент/файл"" - "клиент"
// здесь имя зависимости из .rechproj, "файл" - путь внутри её скачанных
// исходников), и crossedInto возвращает этот каталог (см. package doc
// LoadWithGraph - вызывающая сторона должна продолжить резолюцию ВНУТРИ
// него уже по ЕГО собственной карте имён, не по deps). Иначе - относительно
// baseDir (каталог файла, где написано "использовать", после применения
// ведущих ".." выше), crossedInto == "".
func resolveImportPath(baseDir, projectRoot, importPath string, deps map[string]string) (path, crossedInto string, err error) {
	if body, isAbsolute := strings.CutPrefix(importPath, "@"); isAbsolute {
		segments := strings.Split(body, "/")
		for _, seg := range segments {
			if err := ValidSegmentName(seg); err != nil {
				return "", "", fmt.Errorf("путь импорта %q: %w", importPath, err)
			}
		}
		parts := append([]string{projectRoot}, segments...)
		resolved, err := ResolveSourceExtension(filepath.Join(parts...))
		if err != nil {
			return "", "", err
		}
		return platformSpecificPath(resolved), "", nil
	}

	segments := strings.Split(importPath, "/")
	root := baseDir
	i := 0
	for i < len(segments) && segments[i] == ".." {
		root = filepath.Dir(root)
		i++
	}
	rest := segments[i:]
	if len(rest) == 0 {
		return "", "", fmt.Errorf("путь импорта %q: после \"..\" ожидался путь к файлу", importPath)
	}
	for _, seg := range rest {
		if err := ValidSegmentName(seg); err != nil {
			return "", "", fmt.Errorf("путь импорта %q: %w", importPath, err)
		}
	}
	if i == 0 {
		if depDir, ok := deps[rest[0]]; ok {
			depName := rest[0]
			root = depDir
			crossedInto = depDir
			rest = rest[1:]
			if len(rest) == 0 {
				return "", "", fmt.Errorf("путь импорта %q: после имени зависимости %q ожидался путь к файлу внутри неё", importPath, depName)
			}
		}
	}
	parts := append([]string{root}, rest...)
	resolved, err := ResolveSourceExtension(filepath.Join(parts...))
	if err != nil {
		return "", "", err
	}
	return platformSpecificPath(resolved), crossedInto, nil
}

// platformSpecificPath - если рядом с path (уже посчитанный обычный
// резолвинг "использовать X из "путь"", включая ".rech") лежит файл того
// же имени с суффиксом "_windows"/"_linux"/"_macos" (нижний регистр) под
// ТЕКУЩУЮ платформу (docs/SPEC.md) - используется ОН целиком вместо
// обычного path, взаимозаменяемо (не слияние - см. package doc
// ast.ResolvePlatformBlocks про тот же принцип у "#если") - обычный path
// в этом случае не обязан существовать вовсе. Файл для ДРУГОЙ платформы,
// лежащий рядом, просто никогда не читается этой сборкой - синтаксис,
// валидный только там, никогда не всплывает.
func platformSpecificPath(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	suffixed := base + "_" + strings.ToLower(ast.CurrentPlatform()) + ext
	if _, err := os.Stat(suffixed); err == nil {
		return suffixed
	}
	return path
}
