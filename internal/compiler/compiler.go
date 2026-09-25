// Package compiler описывает пайплайн компилятора Rech как набор
// интерфейсов (Frontend, Backend, Toolchain) и связывает их в Compiler.
//
// Конкретная сборка (какой Frontend, какой Backend, какой Toolchain)
// собирается через Wire - см. cmd/rech/wire.go. Когда появятся
// interpreter/VM backend или альтернативный toolchain, добавление новой
// конфигурации не потребует переписывать main.go.
package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/frontend"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/module"
	"github.com/IlyaFeoktistov/Rech/internal/project"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// validateSourceFileName проверяет имя .rech-файла: расширение (".rech" или
// один из кириллических синонимов module.SourceExtensions - ".речь"/".р") и
// алфавит имени (module.ValidSegmentName - строчные буквы/цифры/"_", форма
// Unicode NFC; то же правило, которым разрешаются сегменты пути импорта,
// потому что имя файла - это уже сейчас единица импорта в системе модулей
// ("использовать X из "имя_файла""), а сегмент пути импорта - либо имя
// файла, либо имя каталога с тем же ограничением по той же причине; см.
// docs/GRAMMAR_DRAFT.md, раздел "Имена файлов и кодировка").
func validateSourceFileName(path string) error {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if !module.HasSourceExtension(base) {
		return fmt.Errorf("файл должен иметь расширение .rech (или синоним %s), получено %q", strings.Join(module.SourceExtensions[1:], "/"), base)
	}
	name := strings.TrimSuffix(base, ext)
	if err := module.ValidSegmentName(name); err != nil {
		return fmt.Errorf("имя файла %q: %w - например, имя_файла.rech", name, err)
	}
	return nil
}

// Frontend/NewFrontend - переэкспорт internal/frontend (см. package doc
// там - вынесены в отдельный пакет, чтобы потребителям, которым нужен
// только разбор, не приходилось тянуть internal/backend/cbackend
// транзитивно через этот пакет).
type Frontend = frontend.Frontend

// NewFrontend создаёт стандартный Frontend v0.1 (lexer.New + parser.New).
func NewFrontend() Frontend { return frontend.NewFrontend() }

// Backend превращает AST в исходник на целевом языке (сейчас - C).
type Backend interface {
	// dbPoolSize - "бд.размер_пула" файла проекта (0 - не указан, значит
	// используй встроенное умолчание internal/semantic.DefaultDBPoolSize,
	// см. package doc ir.Program.DBPoolSize) - единственная настройка
	// .rechproj, влияющая на саму КОДОГЕНЕРАЦИЮ (не на процесс сборки, как
	// extraLibs/keepC), поэтому передаётся сюда явным параметром, а не
	// полем Compiler, которое Backend не видит. dbPoolTimeoutMs - "бд.
	// таймаут_пула_мс" файла проекта, тем же принципом (0 - тоже
	// содержательное значение "ждать бесконечно", не сентинел - см. package
	// doc ir.Program.DBPoolTimeoutMs). dbSQLiteBusyTimeoutMs - "бд.
	// sqlite_таймаут_блокировки_мс", тем же принципом, что dbPoolSize (0 -
	// используй встроенное умолчание internal/semantic.
	// DefaultDBSQLiteBusyTimeoutMs).
	// poolShrinkEnabled - "потоки.сжатие_пула" файла проекта (false - пул
	// воркер-потоков "запустить" растёт и никогда не сжимается, см.
	// ir.Program.PoolShrinkEnabled). poolIdleTimeoutMs - "потоки.
	// таймаут_простоя_мс", тем же принципом, что dbPoolSize (0 - используй
	// встроенное умолчание internal/semantic.DefaultPoolIdleTimeoutMs) -
	// имеет значение, только если poolShrinkEnabled. httpMaxRequestBytes -
	// "http.макс_размер_запроса_байт" файла проекта, тем же принципом, что
	// dbPoolSize (0 - используй встроенное умолчание internal/semantic.
	// DefaultHTTPMaxRequestBytes). externHeaders - "сборка.заголовки" файла
	// проекта (internal/project.File.Заголовки) - nil, если не указаны (см.
	// ir.Program.ExternHeaders).
	// optionsYAML - "опции:" файла проекта (internal/project.File.Опции) -
	// см. ir.Program.OptionsYAML, тем же принципом, что и externHeaders.
	Generate(prog *ast.Program, dbPoolSize, dbPoolTimeoutMs, dbSQLiteBusyTimeoutMs int, poolShrinkEnabled bool, poolIdleTimeoutMs, httpMaxRequestBytes int, externHeaders []string, optionsYAML string) (string, error)
	// GenerateLib - как Generate, но для разделяемой библиотеки (`rech
	// build --lib`): второй результат - текст C-заголовка (.h) с
	// прототипами объявлений, помеченных "экспортировать"; третий - тот же
	// публичный интерфейс в синтаксисе Rech (.rechi, docs/SPEC.md,
	// "Закрытые (бинарные) пакеты") - потребитель подключает его через
	// "использовать" без единой ручной строки FFI, РАЗБИТЫЙ по исходным
	// файлам библиотеки (ключ - путь файла относительно libRoot без
	// расширения) - интерфейс сохраняет ту же структуру подкаталогов, что
	// исходники автора, не схлопывается в один плоский файл. libRoot == "" -
	// нет файла проекта (BuildLibSource) - один плоский результат под
	// ключом "".
	GenerateLib(prog *ast.Program, libRoot string, dbPoolSize, dbPoolTimeoutMs, dbSQLiteBusyTimeoutMs int, poolShrinkEnabled bool, poolIdleTimeoutMs, httpMaxRequestBytes int, externHeaders []string, optionsYAML string) (cSource, header string, interfaceStubs map[string]string, err error)
}

// Toolchain отвечает за финальную сборку сгенерированного исходника
// в нативный бинарник или разделяемую библиотеку (CompileLib, `rech build
// --lib`).
type Toolchain interface {
	// CompileC компилирует cPath в исполняемый файл binPath (см. CBuild).
	CompileC(cPath, binPath string, b CBuild) error
	// CompileLib компилирует cPath в разделяемую библиотеку libPath - имя
	// уже несёт правильное платформенное расширение (см. LibExtension).
	CompileLib(cPath, libPath string, b CBuild) error
	// LibExtension - расширение разделяемой библиотеки на текущей
	// платформе (".so"/".dylib"/".dll") - Compiler называет им итоговый
	// файл, сам не завися от runtime.GOOS.
	LibExtension() string
}

// CBuild - параметры вызова компилятора C.
//
// Libs - внешние библиотеки для линковки (-l<имя>, "сборка.библиотеки"
// файла проекта и его зависимостей), LibPaths - каталоги для их поиска
// (-L<путь>, "сборка.путь_библиотек" - уже АБСОЛЮТНЫЕ пути, разрешённые
// относительно каталога .rechproj). Оба nil - только системный поиск.
//
// CCompiler - "сборка.компилятор_c" ("" - $CC, затем cc/clang/gcc в PATH).
// CFlags/LinkFlags - "сборка.флаги_c"/"сборка.флаги_линковки": при
// CFlagsSet/LinkFlagsSet они ПОЛНОСТЬЮ заменяют флаги, которые компилятор
// ставит сам (оптимизация, -lm/-lgc/-pthread и прочие автоматически
// найденные библиотеки, Libs/LibPaths, -shared у библиотеки) - сам
// компилятор добавляет только "<файл.c> -o <выход>".
type CBuild struct {
	Libs, LibPaths []string
	CCompiler      string
	CFlags         []string
	CFlagsSet      bool
	LinkFlags      []string
	LinkFlagsSet   bool
	// Windowed - "сборка.тип: оконное": на Windows исполняемый файл
	// собирается для оконной подсистемы (без консоли).
	Windowed bool
}

type defaultBackend struct{}

// NewBackend создаёт стандартный Backend - анализ через internal/semantic
// (типы, встроенные проверки) с последующей транспиляцией IR в C. Раньше
// генерация шла прямо по AST (cbackend.Generate, без semantic.Analyze) - это
// означало, что проверки semantic (типы состояний и т.п.) защищали только
// `rech test` (через interp), а не собранные бинарники. cbackend.Generate
// сохранён только для тестов самого cbackend, из реального пайплайна сборки
// удалён.
func NewBackend() Backend { return defaultBackend{} }

func (defaultBackend) Generate(prog *ast.Program, dbPoolSize, dbPoolTimeoutMs, dbSQLiteBusyTimeoutMs int, poolShrinkEnabled bool, poolIdleTimeoutMs, httpMaxRequestBytes int, externHeaders []string, optionsYAML string) (string, error) {
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		return "", err
	}
	if dbPoolSize > 0 {
		irProg.DBPoolSize = dbPoolSize
	}
	irProg.DBPoolTimeoutMs = dbPoolTimeoutMs
	if dbSQLiteBusyTimeoutMs > 0 {
		irProg.DBSQLiteBusyTimeoutMs = dbSQLiteBusyTimeoutMs
	}
	irProg.PoolShrinkEnabled = poolShrinkEnabled
	if poolIdleTimeoutMs > 0 {
		irProg.PoolIdleTimeoutMs = poolIdleTimeoutMs
	}
	if httpMaxRequestBytes > 0 {
		irProg.HTTPMaxRequestBytes = httpMaxRequestBytes
	}
	irProg.ExternHeaders = externHeaders
	irProg.OptionsYAML = optionsYAML

	src, err := cbackend.GenerateFromIR(irProg)
	if err != nil {
		return "", diagnostics.Internal(err)
	}
	return src, nil
}

func (defaultBackend) GenerateLib(prog *ast.Program, libRoot string, dbPoolSize, dbPoolTimeoutMs, dbSQLiteBusyTimeoutMs int, poolShrinkEnabled bool, poolIdleTimeoutMs, httpMaxRequestBytes int, externHeaders []string, optionsYAML string) (string, string, map[string]string, error) {
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		return "", "", nil, err
	}
	if dbPoolSize > 0 {
		irProg.DBPoolSize = dbPoolSize
	}
	irProg.DBPoolTimeoutMs = dbPoolTimeoutMs
	if dbSQLiteBusyTimeoutMs > 0 {
		irProg.DBSQLiteBusyTimeoutMs = dbSQLiteBusyTimeoutMs
	}
	irProg.PoolShrinkEnabled = poolShrinkEnabled
	if poolIdleTimeoutMs > 0 {
		irProg.PoolIdleTimeoutMs = poolIdleTimeoutMs
	}
	if httpMaxRequestBytes > 0 {
		irProg.HTTPMaxRequestBytes = httpMaxRequestBytes
	}
	irProg.ExternHeaders = externHeaders
	irProg.OptionsYAML = optionsYAML

	cSource, header, stubs, err := cbackend.GenerateLibFromIR(irProg, libRoot)
	if err != nil {
		return "", "", nil, diagnostics.Internal(err)
	}
	return cSource, header, stubs, nil
}

// Compiler связывает Frontend, Backend и Toolchain в единый пайплайн
// "исходник на Rech -> нативный бинарник".
type Compiler struct {
	Frontend  Frontend
	Backend   Backend
	Toolchain Toolchain
	// Deps - внешние зависимости уже разрешённые (internal/pkgfetch) в
	// момент запуска - имя зависимости (из .rechproj/"зависимости:") ->
	// локальный каталог с её скачанными исходниками. nil, если проект не
	// объявляет ни одной зависимости (или собирается вообще без
	// .rechproj) - "использовать" тогда работает как раньше, только
	// локальными путями. Заполняется cmd/rech перед вызовом Build, см.
	// internal/module.LoadWithGraph.
	Deps map[string]string
	// DepScopes - весь граф ТРАНЗИТИВНЫХ зависимостей (internal/pkgfetch.
	// Graph.Scopes) - собственная карта имён каждой зависимости из Deps
	// (и их зависимостей, рекурсивно). nil - как без транзитивных
	// зависимостей вовсе (LoadWithDeps вместо LoadWithGraph по факту).
	DepScopes map[string]map[string]string
	// DBPoolSize - "бд.размер_пула" файла проекта (0 - нет .rechproj или
	// поле не указано, см. Backend.Generate) - заполняется cmd/rech перед
	// вызовом Build, тем же принципом, что и Deps/DepScopes.
	DBPoolSize int
	// DBPoolTimeoutMs - "бд.таймаут_пула_мс" файла проекта (0 - нет
	// .rechproj, поле не указано, ИЛИ явно "0" - все три неотличимы и не
	// обязаны быть отличимы, см. Backend.Generate) - тем же принципом, что
	// DBPoolSize.
	DBPoolTimeoutMs int
	// DBSQLiteBusyTimeoutMs - "бд.sqlite_таймаут_блокировки_мс" файла
	// проекта (0 - не указан, используй встроенное умолчание internal/
	// semantic.DefaultDBSQLiteBusyTimeoutMs), тем же принципом, что
	// DBPoolSize.
	DBSQLiteBusyTimeoutMs int
	// PoolShrinkEnabled - "потоки.сжатие_пула" файла проекта (true - нет
	// .rechproj, поле не указано, или "истина" - см. cmd/rech/main.go,
	// значение по умолчанию выставляется там явно, поскольку у bool, в
	// отличие от int-настроек вроде DBPoolSize, нет естественного
	// сентинела "не задано") - заполняется cmd/rech перед вызовом Build,
	// тем же принципом, что и Deps/DepScopes.
	PoolShrinkEnabled bool
	// PoolIdleTimeoutMs - "потоки.таймаут_простоя_мс" файла проекта (0 - не
	// указан, используй встроенное умолчание internal/semantic.
	// DefaultPoolIdleTimeoutMs), тем же принципом, что DBPoolSize. Имеет
	// значение, только если PoolShrinkEnabled.
	PoolIdleTimeoutMs int
	// HTTPMaxRequestBytes - "http.макс_размер_запроса_байт" файла проекта
	// (0 - не указан, используй встроенное умолчание internal/semantic.
	// DefaultHTTPMaxRequestBytes), тем же принципом, что DBPoolSize.
	HTTPMaxRequestBytes int
	// ExternHeaders - "сборка.заголовки" файла проекта (internal/project.
	// File.Заголовки) - заполняется cmd/rech перед вызовом Build, тем же
	// принципом, что и Deps/DepScopes. nil - нет .rechproj или поле не
	// указано (см. ir.Program.ExternHeaders).
	ExternHeaders []string
	// OptionsYAML - "опции:" файла проекта (internal/project.File.Опции) -
	// заполняется cmd/rech перед вызовом Build, тем же принципом, что и
	// ExternHeaders. "" - нет .rechproj или секции (см. ir.Program.OptionsYAML).
	OptionsYAML string
	// CBuild - компилятор/флаги C из файла проекта (Libs/LibPaths здесь не
	// заполняются - приходят параметрами Build, см. cBuild).
	CBuild CBuild
}

// cBuild - параметры вызова cc для этой сборки: флаги проекта плюс
// библиотеки конкретного вызова Build.
func (c *Compiler) cBuild(libs, libPaths []string) CBuild {
	b := c.CBuild
	b.Libs, b.LibPaths = libs, libPaths
	return b
}

// NewCompiler - провайдер для Wire, собирающий Compiler из готовых зависимостей.
func NewCompiler(frontend Frontend, backend Backend, toolchain Toolchain) *Compiler {
	return &Compiler{Frontend: frontend, Backend: backend, Toolchain: toolchain}
}

// Build прогоняет srcPath (файл .rech или каталог) через весь пайплайн и
// возвращает путь к собранному бинарнику. Сгенерированный .c удаляется
// после успешной сборки бинарника, если keepC == false - по умолчанию это
// техническая заготовка, а не то, что нужно смотреть; keepC == true (флаг
// --keep-c в cmd/rech) оставляет его - удобно для примеров и для того,
// чтобы посмотреть, во что компилируется программа.
func (c *Compiler) Build(srcPath string, keepC bool, extraLibs, extraLibPaths []string) (string, error) {
	prog, base, err := c.ParseSource(srcPath)
	if err != nil {
		return "", err
	}
	return c.compileProgram(prog, base, keepC, extraLibs, extraLibPaths)
}

// BuildLib - как Build, но собирает srcPath как разделяемую библиотеку
// (.so/.dylib/.dll, `rech build --lib`) вместо исполняемого файла: не
// требует функцию "главная" (библиотека не
// исполняется как программа) и дополнительно возвращает путь к
// сгенерированному C-заголовку (.h) и к путям публичного интерфейса в
// синтаксисе Rech (.rechi, docs/SPEC.md, "Закрытые (бинарные) пакеты") -
// оба с прототипами/объявлениями, помеченными "экспортировать". libRoot -
// каталог файла проекта (pf.Dir) для разбивки .rechi по исходным файлам
// библиотеки (см. Backend.GenerateLib) - "", если библиотека собрана без
// .rechproj (тогда один плоский .rechi рядом с .h/.so, как раньше). stubDir -
// куда класть РАЗЛОЖЕННЫЕ по подкаталогам .rechi (обычно "файлы.выход"
// проекта, "сборка") - не используется, если libRoot == "" (единственный
// плоский .rechi всегда идёт рядом с остальными артефактами, как и .h/.so
// ниже, независимо от stubDir).
func (c *Compiler) BuildLib(srcPath, libRoot, stubDir string, keepC bool, extraLibs, extraLibPaths []string) (libPath, hPath string, rechiPaths []string, err error) {
	prog, base, err := c.ParseSource(srcPath)
	if err != nil {
		return "", "", nil, err
	}
	return c.compileLibProgram(prog, base, libRoot, stubDir, keepC, extraLibs, extraLibPaths)
}

// BuildLibSource - BuildLib для кода без файла-источника (-c/--code
// --lib), симметрично BuildSource/Build - без файла проекта разбивать
// .rechi по исходным файлам нечему (libRoot/stubDir пустые).
func (c *Compiler) BuildLibSource(src string, keepC bool, extraLibs, extraLibPaths []string) (libPath, hPath string, rechiPaths []string, err error) {
	prog, err := c.Frontend.Parse(src)
	if err != nil {
		return "", "", nil, err
	}
	dir, err := os.MkdirTemp("", "rech-code-")
	if err != nil {
		return "", "", nil, fmt.Errorf("не удалось создать временный каталог: %w", err)
	}
	return c.compileLibProgram(prog, filepath.Join(dir, "код"), "", "", keepC, extraLibs, extraLibPaths)
}

// writeInterfaceStubs пишет результат Backend.GenerateLib (relPath без
// расширения -> содержимое) на диск: relPath == "" (нет libRoot - см.
// package doc BuildLib) - единственный плоский файл рядом с остальными
// артефактами библиотеки (base+".rechi", тем же путём, что и раньше);
// иначе - "<stubDir>/<relPath>.rechi", с созданием недостающих подкаталогов
// (docs/SPEC.md, "Закрытые (бинарные) пакеты" - интерфейс сохраняет
// структуру исходников автора). Возвращает пути в детерминированном
// (отсортированном по relPath) порядке.
func writeInterfaceStubs(stubs map[string]string, base, stubDir string) ([]string, error) {
	relPaths := make([]string, 0, len(stubs))
	for relPath := range stubs {
		relPaths = append(relPaths, relPath)
	}
	sort.Strings(relPaths)

	paths := make([]string, 0, len(relPaths))
	for _, relPath := range relPaths {
		target := base + ".rechi"
		if relPath != "" {
			target = filepath.Join(stubDir, filepath.FromSlash(relPath)) + ".rechi"
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, fmt.Errorf("не удалось создать каталог %q: %w", filepath.Dir(target), err)
			}
		}
		if err := os.WriteFile(target, []byte(stubs[relPath]), 0o644); err != nil {
			return nil, fmt.Errorf("не удалось записать %s: %w", target, err)
		}
		paths = append(paths, target)
	}
	return paths, nil
}

// compileLibProgram - общий хвост BuildLib/BuildLibSource, аналогичный
// compileProgram: генерация C+заголовка+интерфейса, сборка разделяемой
// библиотекой, необязательная уборка .c. В отличие от compileProgram,
// заголовок (.h) и интерфейс (.rechi) пишутся всегда, а не только при
// keepC - оба часть публичного API библиотеки, а не техническая заготовка
// вроде .c.
func (c *Compiler) compileLibProgram(prog *ast.Program, base, libRoot, stubDir string, keepC bool, extraLibs, extraLibPaths []string) (libPath, hPath string, rechiPaths []string, err error) {
	cSource, header, interfaceStubs, err := c.Backend.GenerateLib(prog, libRoot, c.DBPoolSize, c.DBPoolTimeoutMs, c.DBSQLiteBusyTimeoutMs, c.PoolShrinkEnabled, c.PoolIdleTimeoutMs, c.HTTPMaxRequestBytes, c.ExternHeaders, c.OptionsYAML)
	if err != nil {
		return "", "", nil, fmt.Errorf("генерация C: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "rech-build-")
	if err != nil {
		return "", "", nil, fmt.Errorf("не удалось создать временный каталог сборки: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpBase := filepath.Join(tmpDir, "out")
	tmpCPath := tmpBase + ".c"
	if err := os.WriteFile(tmpCPath, []byte(cSource), 0o644); err != nil {
		return "", "", nil, fmt.Errorf("не удалось записать %s: %w", tmpCPath, err)
	}
	libExt := c.Toolchain.LibExtension()
	tmpLibPath := tmpBase + libExt
	if err := c.Toolchain.CompileLib(tmpCPath, tmpLibPath, c.cBuild(extraLibs, extraLibPaths)); err != nil {
		return "", "", nil, diagnostics.Internal(err)
	}

	libPath = base + libExt
	if err := MoveFile(tmpLibPath, libPath); err != nil {
		return "", "", nil, fmt.Errorf("не удалось перенести собранную библиотеку в %s: %w", libPath, err)
	}

	hPath = base + ".h"
	if err := os.WriteFile(hPath, []byte(header), 0o644); err != nil {
		return "", "", nil, fmt.Errorf("не удалось записать %s: %w", hPath, err)
	}

	rechiPaths, err = writeInterfaceStubs(interfaceStubs, base, stubDir)
	if err != nil {
		return "", "", nil, err
	}

	if keepC {
		cPath := base + ".c"
		if err := os.WriteFile(cPath, []byte(cSource), 0o644); err != nil {
			return "", "", nil, fmt.Errorf("не удалось записать %s: %w", cPath, err)
		}
	}
	return libPath, hPath, rechiPaths, nil
}

// ParseSource разбирает srcPath (файл .rech или каталог) в *ast.Program -
// общая первая половина Build, вынесена отдельно ради internal/debugadapter
// ("rech debug" не идёт дальше AST/IR - до C-кодогенерации/системного
// компилятора дело не доходит, исполняет internal/interp). base - "имя без
// расширения" (файл) или "путь/имя каталога" (каталог) - нужно только Build
// (имя будущего бинарника), debugadapter его не использует.
func (c *Compiler) ParseSource(srcPath string) (*ast.Program, string, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, "", fmt.Errorf("не удалось найти %q: %w", srcPath, err)
	}
	if info.IsDir() {
		prog, err := c.parseDir(srcPath)
		if err != nil {
			return nil, "", err
		}
		absDir, err := filepath.Abs(srcPath)
		if err != nil {
			return nil, "", err
		}
		return prog, filepath.Join(srcPath, filepath.Base(absDir)), nil
	}
	prog, err := c.parseFile(srcPath)
	if err != nil {
		return nil, "", err
	}
	return prog, strings.TrimSuffix(srcPath, filepath.Ext(srcPath)), nil
}

// BuildIR разбирает и семантически анализирует srcPath, без кодогенерации/
// системного компилятора - нужен internal/debugadapter ("rech debug"
// исполняет IR напрямую через internal/interp.Debugger, C ему не нужен).
func (c *Compiler) BuildIR(srcPath string) (*ir.Program, error) {
	prog, _, err := c.ParseSource(srcPath)
	if err != nil {
		return nil, err
	}
	return semantic.Analyze(prog)
}

// BuildSource собирает src как программу без файла-источника (`rech
// build/run -c "<код>"`) - в отличие от Build, не проверяет имя файла
// (validateSourceFileName - правило конкретно про будущий ключ импорта,
// у кода без файла его нет) и кладёт результат во временный каталог, а не
// "рядом с исходником", потому что рядом с исходником здесь - никуда.
func (c *Compiler) BuildSource(src string, keepC bool, extraLibs, extraLibPaths []string) (string, error) {
	prog, err := c.Frontend.Parse(src)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "rech-code-")
	if err != nil {
		return "", fmt.Errorf("не удалось создать временный каталог: %w", err)
	}
	return c.compileProgram(prog, filepath.Join(dir, "код"), keepC, extraLibs, extraLibPaths)
}

// compileProgram - общий хвост Build/BuildSource: генерация C, запись,
// сборка бинарником, необязательная уборка .c.
func (c *Compiler) compileProgram(prog *ast.Program, base string, keepC bool, extraLibs, extraLibPaths []string) (string, error) {
	cSource, err := c.Backend.Generate(prog, c.DBPoolSize, c.DBPoolTimeoutMs, c.DBSQLiteBusyTimeoutMs, c.PoolShrinkEnabled, c.PoolIdleTimeoutMs, c.HTTPMaxRequestBytes, c.ExternHeaders, c.OptionsYAML)
	if err != nil {
		return "", fmt.Errorf("генерация C: %w", err)
	}

	if keepC {
		cPath := base + ".c"
		if err := os.WriteFile(cPath, []byte(cSource), 0o644); err != nil {
			return "", fmt.Errorf("не удалось записать %s: %w", cPath, err)
		}
	}

	// Сам системный компилятор C (Toolchain.CompileC) вызывается только с
	// ASCII-путями во временном каталоге, а не там, где на самом деле лежит
	// исходник/должен оказаться бинарник (base может содержать кириллицу -
	// обычное дело для Rech, вплоть до плейсхолдера "код" у BuildSource
	// ниже) - на Windows mingw gcc/ld открывают и входной .c, и выходной
	// бинарник через узкие (не wide-char) файловые API, перекодируя путь из
	// аргумента командной строки через активную ANSI-кодовую страницу
	// процесса - она кириллицу не покрывает, и оба открытия молча
	// проваливаются ("Permission denied" на искажённом имени). Готовый
	// бинарник переносится на настоящее место уже обычными файловыми
	// операциями Go (moveFile) - они на Windows идут через wide-char API и
	// этой проблеме не подвержены. Тот же путь используется на всех
	// платформах, не только Windows - на Linux/macOS он просто не нужен, но
	// и не вреден, а платформенно-specific ветка здесь не заводится.
	tmpDir, err := os.MkdirTemp("", "rech-build-")
	if err != nil {
		return "", fmt.Errorf("не удалось создать временный каталог сборки: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpBase := filepath.Join(tmpDir, "out")
	tmpCPath := tmpBase + ".c"
	if err := os.WriteFile(tmpCPath, []byte(cSource), 0o644); err != nil {
		return "", fmt.Errorf("не удалось записать %s: %w", tmpCPath, err)
	}
	if err := c.Toolchain.CompileC(tmpCPath, tmpBase, c.cBuild(extraLibs, extraLibPaths)); err != nil {
		return "", diagnostics.Internal(err)
	}

	// Некоторые тулчейны (mingw gcc на Windows) сами добавляют ".exe" к
	// выходному файлу, даже если -o дан без расширения - тот же суффикс
	// нужно сохранить и на настоящем месте (base), а не срезать его:
	// .exe - то, что реально умеет запускать сам Windows (explorer, cmd.exe,
	// сторонние скрипты) без знания о том, как именно Go разрешает
	// расширения при exec.Command - см. runBinary в cmd/rech.
	compiledBin, ext := tmpBase, ""
	if _, err := os.Stat(compiledBin); err != nil {
		if _, err2 := os.Stat(tmpBase + ".exe"); err2 == nil {
			compiledBin, ext = tmpBase+".exe", ".exe"
		}
	}
	binPath := base + ext
	if err := MoveFile(compiledBin, binPath); err != nil {
		return "", fmt.Errorf("не удалось перенести собранный бинарник в %s: %w", binPath, err)
	}

	return binPath, nil
}

// MoveFile переносит файл src в dst - сначала пробует os.Rename (дёшево,
// атомарно), а если это не сработало (типичная причина - src и dst на
// разных файловых системах/томах, например временный каталог сборки на
// tmpfs, а место назначения - на обычном диске: os.Rename тогда вернул бы
// "invalid cross-device link" на Linux), откатывается на явное копирование
// байт + удаление src. 0o755 (не 0o644, как у обычных файлов) - у src
// собранный исполняемый бинарник, право на выполнение должно сохраниться.
// Экспортирована - переиспользуется в cmd/rech для переноса бинарника в
// папку "выход" файла проекта (.rechproj).
func MoveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return err
	}
	return os.Remove(src)
}

// parseFileRaw разбирает ровно srcPath, без "использовать" - используется
// в режиме каталога (parseDir), где у файлов друг с другом и
// так нет разделения видимости, поэтому подключать через них ещё и файлы
// снаружи каталога было бы лишним смешением двух разных механизмов.
func (c *Compiler) parseFileRaw(srcPath string) (*ast.Program, error) {
	if err := validateSourceFileName(srcPath); err != nil {
		return nil, err
	}
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл: %w", err)
	}
	prog, err := c.Frontend.Parse(string(srcBytes))
	if err != nil {
		return nil, diagnostics.WithFile(err, srcPath)
	}
	ast.StampFile(prog, srcPath)
	return prog, nil
}

// parseFile разбирает srcPath как единицу компиляции верхнего уровня
// (`rech build/run <файл>`) - в отличие от parseFileRaw, разрешает
// "использовать": рекурсивно загружает и линкует файлы, на которые
// srcPath ссылается, в одну программу с контролем видимости между файлами
// (module.Load/Program.Link, включая обнаружение циклических импортов) - у
// файла без единого "использовать" результат не отличается от обычного
// разбора одного файла.
//
// НАЙДЕННЫЙ БАГ: srcPath не обязательно pf.MainPath -
// главный файл своего .rechproj (cmd/rech/main.go подставляет его только
// когда пользователь передал САМ .rechproj; обычный "rech build/run
// <файл.rech>" передаёт srcPath как есть, даже если файл лежит глубоко
// внутри дерева проекта - например, тестовый файл internal/testrunner
// рядом с исходниками, использующий "использовать ... из "@путь""). Без
// project.RootFor "@путь" резолвился бы от каталога САМОГО srcPath
// (module.LoadWithRoot с root == "" - её package doc), а не от корня
// проекта - расходилось с "rech test" на ТОМ ЖЕ файле (internal/
// testrunner.RunFile уже вызывает project.RootFor) и с LSP. project.
// RootFor возвращает "" без ошибки, если .rechproj не найден - тогда
// LoadWithRoot ведёт себя как прежде (каталог srcPath), никакой разницы
// для обычных отдельно стоящих файлов вне проекта.
func (c *Compiler) parseFile(srcPath string) (*ast.Program, error) {
	if err := validateSourceFileName(srcPath); err != nil {
		return nil, err
	}
	root, err := project.RootFor(srcPath)
	if err != nil {
		return nil, err
	}
	prog, err := module.LoadWithRoot(srcPath, c.Frontend, c.Deps, c.DepScopes, root)
	if err != nil {
		return nil, err
	}
	return prog.Link()
}

// parseDir собирает все ".rech"-файлы прямо в dirPath (без рекурсии в
// подкаталоги - как "." у "go build .") и объединяет их в одну программу.
// Это НЕ система импортов ("использовать X из "файла"" - явная,
// управляемая связь между конкретными файлами) - здесь всё
// содержимое каталога неявно становится одной единицей компиляции, файлы
// объединяются в алфавитном порядке имён (детерминированно, но без
// какого-либо контроля видимости между ними - временное решение до
// появления настоящих импортов). Дубли имён функций/сущностей между
// файлами - та же ошибка компиляции, что и дубль внутри одного файла
// (проверяется дальше по конвейеру, отдельной проверки здесь не нужно).
// "использовать" внутри файла каталога - явная ошибка, а не тихо
// игнорируемая конструкция: смешивать "весь каталог - одна единица
// компиляции" (без контроля видимости) с настоящими, управляемыми
// импортами (с контролем видимости) для одних и тех же файлов было бы
// путаницей, а не удобством.
func (c *Compiler) parseDir(dirPath string) (*ast.Program, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать каталог %q: %w", dirPath, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !module.HasSourceExtension(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("в каталоге %q не найдено ни одного файла .rech", dirPath)
	}
	if err := module.CheckNoAmbiguousExtensions(names); err != nil {
		return nil, fmt.Errorf("в каталоге %q: %w", dirPath, err)
	}
	sort.Strings(names)

	merged := &ast.Program{}
	for _, name := range names {
		prog, err := c.parseFileRaw(filepath.Join(dirPath, name))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if len(prog.Uses) > 0 {
			return nil, fmt.Errorf("%s: \"использовать\" не поддерживается в режиме сборки каталога (rech build <каталог>) - соберите этот файл напрямую (rech build <файл>) или уберите импорт", name)
		}
		merged.Functions = append(merged.Functions, prog.Functions...)
		merged.Entities = append(merged.Entities, prog.Entities...)
		merged.ExternFuncs = append(merged.ExternFuncs, prog.ExternFuncs...)
		merged.ExternEntities = append(merged.ExternEntities, prog.ExternEntities...)
		merged.Enums = append(merged.Enums, prog.Enums...)
		merged.Interfaces = append(merged.Interfaces, prog.Interfaces...)
		merged.Consts = append(merged.Consts, prog.Consts...)
	}
	return merged, nil
}
