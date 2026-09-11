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
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/module"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/project"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// validateSourceFileName проверяет имя .rech-файла: расширение и алфавит
// имени (module.ValidSegmentName - строчные буквы/цифры/"_", форма Unicode
// NFC; то же правило, которым разрешаются сегменты пути импорта, потому что
// имя файла - это уже сейчас единица импорта в системе модулей
// ("использовать X из "имя_файла""), а сегмент пути импорта - либо имя
// файла, либо имя каталога с тем же ограничением по той же причине; см.
// docs/GRAMMAR.md, раздел "Имена файлов и кодировка").
func validateSourceFileName(path string) error {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != ".rech" {
		return fmt.Errorf("файл должен иметь расширение .rech, получено %q", base)
	}
	name := strings.TrimSuffix(base, ext)
	if err := module.ValidSegmentName(name); err != nil {
		return fmt.Errorf("имя файла %q: %w - например, имя_файла.rech", name, err)
	}
	return nil
}

// Frontend превращает исходный текст на Rech в AST (лексер + парсер).
type Frontend interface {
	Parse(src string) (*ast.Program, error)
}

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
	// DefaultHTTPMaxRequestBytes).
	Generate(prog *ast.Program, dbPoolSize, dbPoolTimeoutMs, dbSQLiteBusyTimeoutMs int, poolShrinkEnabled bool, poolIdleTimeoutMs, httpMaxRequestBytes int) (string, error)
	// GenerateLib - как Generate, но для разделяемой библиотеки (`rech
	// build --lib`): второй результат - текст C-заголовка (.h) с
	// прототипами объявлений, помеченных "экспортировать"; третий - тот же
	// публичный интерфейс в синтаксисе Rech (.rechi, docs/SPEC.md,
	// "Закрытые (бинарные) пакеты") - потребитель подключает его через
	// "использовать" без единой ручной строки FFI.
	GenerateLib(prog *ast.Program, dbPoolSize, dbPoolTimeoutMs, dbSQLiteBusyTimeoutMs int, poolShrinkEnabled bool, poolIdleTimeoutMs, httpMaxRequestBytes int) (cSource, header, interfaceStub string, err error)
}

// Toolchain отвечает за финальную сборку сгенерированного исходника
// в нативный бинарник или разделяемую библиотеку (CompileLib, `rech build
// --lib`).
type Toolchain interface {
	// CompileC компилирует cPath в исполняемый файл binPath. extraLibs -
	// внешние библиотеки для линковки (-l<имя>, "сборка.библиотеки" файла
	// проекта) - nil, если проект их не объявил. extraLibPaths -
	// дополнительные каталоги для их поиска (-L<путь>, "сборка.
	// путь_библиотек" - уже АБСОЛЮТНЫЕ пути, разрешённые относительно
	// каталога .rechproj в internal/project.File.ПутиБиблиотек, эта
	// функция сама их не резолвит) - nil, если не указаны (обычный
	// системный поиск линковщика, как и раньше).
	CompileC(cPath, binPath string, extraLibs, extraLibPaths []string) error
	// CompileLib компилирует cPath в разделяемую библиотеку libPath - имя
	// уже несёт правильное платформенное расширение (см. LibExtension).
	// extraLibs/extraLibPaths - см. CompileC.
	CompileLib(cPath, libPath string, extraLibs, extraLibPaths []string) error
	// LibExtension - расширение разделяемой библиотеки на текущей
	// платформе (".so"/".dylib"/".dll") - Compiler называет им итоговый
	// файл, сам не завися от runtime.GOOS.
	LibExtension() string
}

type defaultFrontend struct{}

// NewFrontend создаёт стандартный Frontend v0.1 (lexer.New + parser.New).
func NewFrontend() Frontend { return defaultFrontend{} }

func (defaultFrontend) Parse(src string) (*ast.Program, error) {
	if err := lexer.ValidateUTF8(src); err != nil {
		return nil, err
	}
	if err := lexer.ValidateNFC(src); err != nil {
		return nil, err
	}
	tokens, err := lexer.New(src).Tokenize()
	if err != nil {
		return nil, fmt.Errorf("лексер: %w", err)
	}
	prog, err := parser.New(tokens).ParseProgram()
	if err != nil {
		return nil, fmt.Errorf("парсер: %w", err)
	}
	return prog, nil
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

func (defaultBackend) Generate(prog *ast.Program, dbPoolSize, dbPoolTimeoutMs, dbSQLiteBusyTimeoutMs int, poolShrinkEnabled bool, poolIdleTimeoutMs, httpMaxRequestBytes int) (string, error) {
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

	return cbackend.GenerateFromIR(irProg)
}

func (defaultBackend) GenerateLib(prog *ast.Program, dbPoolSize, dbPoolTimeoutMs, dbSQLiteBusyTimeoutMs int, poolShrinkEnabled bool, poolIdleTimeoutMs, httpMaxRequestBytes int) (string, string, string, error) {
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		return "", "", "", err
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

	return cbackend.GenerateLibFromIR(irProg)
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
// сгенерированному C-заголовку (.h) и к публичному интерфейсу в синтаксисе
// Rech (.rechi, docs/SPEC.md, "Закрытые (бинарные) пакеты") - оба с
// прототипами/объявлениями, помеченными "экспортировать".
func (c *Compiler) BuildLib(srcPath string, keepC bool, extraLibs, extraLibPaths []string) (libPath, hPath, rechiPath string, err error) {
	prog, base, err := c.ParseSource(srcPath)
	if err != nil {
		return "", "", "", err
	}
	return c.compileLibProgram(prog, base, keepC, extraLibs, extraLibPaths)
}

// BuildLibSource - BuildLib для кода без файла-источника (-c/--code
// --lib), симметрично BuildSource/Build.
func (c *Compiler) BuildLibSource(src string, keepC bool, extraLibs, extraLibPaths []string) (libPath, hPath, rechiPath string, err error) {
	prog, err := c.Frontend.Parse(src)
	if err != nil {
		return "", "", "", err
	}
	dir, err := os.MkdirTemp("", "rech-code-")
	if err != nil {
		return "", "", "", fmt.Errorf("не удалось создать временный каталог: %w", err)
	}
	return c.compileLibProgram(prog, filepath.Join(dir, "код"), keepC, extraLibs, extraLibPaths)
}

// compileLibProgram - общий хвост BuildLib/BuildLibSource, аналогичный
// compileProgram: генерация C+заголовка+интерфейса, сборка разделяемой
// библиотекой, необязательная уборка .c. В отличие от compileProgram,
// заголовок (.h) и интерфейс (.rechi) пишутся всегда, а не только при
// keepC - оба часть публичного API библиотеки, а не техническая заготовка
// вроде .c.
func (c *Compiler) compileLibProgram(prog *ast.Program, base string, keepC bool, extraLibs, extraLibPaths []string) (libPath, hPath, rechiPath string, err error) {
	cSource, header, interfaceStub, err := c.Backend.GenerateLib(prog, c.DBPoolSize, c.DBPoolTimeoutMs, c.DBSQLiteBusyTimeoutMs, c.PoolShrinkEnabled, c.PoolIdleTimeoutMs, c.HTTPMaxRequestBytes)
	if err != nil {
		return "", "", "", fmt.Errorf("генерация C: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "rech-build-")
	if err != nil {
		return "", "", "", fmt.Errorf("не удалось создать временный каталог сборки: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpBase := filepath.Join(tmpDir, "out")
	tmpCPath := tmpBase + ".c"
	if err := os.WriteFile(tmpCPath, []byte(cSource), 0o644); err != nil {
		return "", "", "", fmt.Errorf("не удалось записать %s: %w", tmpCPath, err)
	}
	libExt := c.Toolchain.LibExtension()
	tmpLibPath := tmpBase + libExt
	if err := c.Toolchain.CompileLib(tmpCPath, tmpLibPath, extraLibs, extraLibPaths); err != nil {
		return "", "", "", err
	}

	libPath = base + libExt
	if err := MoveFile(tmpLibPath, libPath); err != nil {
		return "", "", "", fmt.Errorf("не удалось перенести собранную библиотеку в %s: %w", libPath, err)
	}

	hPath = base + ".h"
	if err := os.WriteFile(hPath, []byte(header), 0o644); err != nil {
		return "", "", "", fmt.Errorf("не удалось записать %s: %w", hPath, err)
	}

	rechiPath = base + ".rechi"
	if err := os.WriteFile(rechiPath, []byte(interfaceStub), 0o644); err != nil {
		return "", "", "", fmt.Errorf("не удалось записать %s: %w", rechiPath, err)
	}

	if keepC {
		cPath := base + ".c"
		if err := os.WriteFile(cPath, []byte(cSource), 0o644); err != nil {
			return "", "", "", fmt.Errorf("не удалось записать %s: %w", cPath, err)
		}
	}
	return libPath, hPath, rechiPath, nil
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
	cSource, err := c.Backend.Generate(prog, c.DBPoolSize, c.DBPoolTimeoutMs, c.DBSQLiteBusyTimeoutMs, c.PoolShrinkEnabled, c.PoolIdleTimeoutMs, c.HTTPMaxRequestBytes)
	if err != nil {
		return "", fmt.Errorf("генерация C: %w", err)
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
	if err := c.Toolchain.CompileC(tmpCPath, tmpBase, extraLibs, extraLibPaths); err != nil {
		return "", err
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

	if keepC {
		cPath := base + ".c"
		if err := os.WriteFile(cPath, []byte(cSource), 0o644); err != nil {
			return "", fmt.Errorf("не удалось записать %s: %w", cPath, err)
		}
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
		return nil, err
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
// НАЙДЕННЫЙ БАГ (docs/ROADMAP.md): srcPath не обязательно pf.MainPath() -
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
		if e.IsDir() || filepath.Ext(e.Name()) != ".rech" {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("в каталоге %q не найдено ни одного файла .rech", dirPath)
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
