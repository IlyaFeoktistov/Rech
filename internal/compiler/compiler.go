// Package compiler описывает пайплайн компилятора Rech как набор
// интерфейсов (Frontend, Backend, Toolchain) и связывает их в Compiler.
//
// Конкретная сборка (какой Frontend, какой Backend, какой Toolchain)
// собирается через Wire - см. cmd/rech/wire.go. Когда появятся
// interpreter/VM backend или альтернативный toolchain (см. docs/ROADMAP.md),
// добавление новой конфигурации не потребует переписывать main.go.
package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/module"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
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
	Generate(prog *ast.Program) (string, error)
}

// Toolchain отвечает за финальную сборку сгенерированного исходника
// в нативный бинарник.
type Toolchain interface {
	CompileC(cPath, binPath string) error
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

func (defaultBackend) Generate(prog *ast.Program) (string, error) {
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		return "", err
	}
	return cbackend.GenerateFromIR(irProg)
}

// Compiler связывает Frontend, Backend и Toolchain в единый пайплайн
// "исходник на Rech -> нативный бинарник".
type Compiler struct {
	Frontend  Frontend
	Backend   Backend
	Toolchain Toolchain
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
func (c *Compiler) Build(srcPath string, keepC bool) (string, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return "", fmt.Errorf("не удалось найти %q: %w", srcPath, err)
	}

	var prog *ast.Program
	var base string
	if info.IsDir() {
		prog, err = c.parseDir(srcPath)
		if err != nil {
			return "", err
		}
		absDir, err := filepath.Abs(srcPath)
		if err != nil {
			return "", err
		}
		base = filepath.Join(srcPath, filepath.Base(absDir))
	} else {
		prog, err = c.parseFile(srcPath)
		if err != nil {
			return "", err
		}
		base = strings.TrimSuffix(srcPath, filepath.Ext(srcPath))
	}

	return c.compileProgram(prog, base, keepC)
}

// BuildSource собирает src как программу без файла-источника (`rech
// build/run -c "<код>"`) - в отличие от Build, не проверяет имя файла
// (validateSourceFileName - правило конкретно про будущий ключ импорта,
// у кода без файла его нет) и кладёт результат во временный каталог, а не
// "рядом с исходником", потому что рядом с исходником здесь - никуда.
func (c *Compiler) BuildSource(src string, keepC bool) (string, error) {
	prog, err := c.Frontend.Parse(src)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "rech-code-")
	if err != nil {
		return "", fmt.Errorf("не удалось создать временный каталог: %w", err)
	}
	return c.compileProgram(prog, filepath.Join(dir, "код"), keepC)
}

// compileProgram - общий хвост Build/BuildSource: генерация C, запись,
// сборка бинарником, необязательная уборка .c.
func (c *Compiler) compileProgram(prog *ast.Program, base string, keepC bool) (string, error) {
	cSource, err := c.Backend.Generate(prog)
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
	if err := c.Toolchain.CompileC(tmpCPath, tmpBase); err != nil {
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
	if err := moveFile(compiledBin, binPath); err != nil {
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

// moveFile переносит файл src в dst - сначала пробует os.Rename (дёшево,
// атомарно), а если это не сработало (типичная причина - src и dst на
// разных файловых системах/томах, например временный каталог сборки на
// tmpfs, а место назначения - на обычном диске: os.Rename тогда вернул бы
// "invalid cross-device link" на Linux), откатывается на явное копирование
// байт + удаление src. 0o755 (не 0o644, как у обычных файлов) - у src
// собранный исполняемый бинарник, право на выполнение должно сохраниться.
func moveFile(src, dst string) error {
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

// parseFileRaw разбирает ровно srcPath, без "использовать" (шаг 11) -
// используется в режиме каталога (parseDir), где у файлов друг с другом и
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
	return c.Frontend.Parse(string(srcBytes))
}

// parseFile разбирает srcPath как единицу компиляции верхнего уровня
// (`rech build/run <файл>`) - в отличие от parseFileRaw, разрешает
// "использовать" (шаг 11): рекурсивно загружает и линкует файлы, на которые
// srcPath ссылается, в одну программу с контролем видимости между файлами
// (module.Load/Program.Link, включая обнаружение циклических импортов) - у
// файла без единого "использовать" результат не отличается от обычного
// разбора одного файла.
func (c *Compiler) parseFile(srcPath string) (*ast.Program, error) {
	if err := validateSourceFileName(srcPath); err != nil {
		return nil, err
	}
	prog, err := module.Load(srcPath, c.Frontend)
	if err != nil {
		return nil, err
	}
	return prog.Link()
}

// parseDir собирает все ".rech"-файлы прямо в dirPath (без рекурсии в
// подкаталоги - как "." у "go build ."; рекурсивный обход отложен до
// файла-манифеста проекта, docs/ROADMAP.md, Шаг 11) и объединяет их в одну
// программу. Это НЕ система импортов (Шаг 11, "использовать X из "файла"" -
// явная, управляемая связь между конкретными файлами) - здесь всё
// содержимое каталога неявно становится одной единицей компиляции, файлы
// объединяются в алфавитном порядке имён (детерминированно, но без
// какого-либо контроля видимости между ними - временное решение до
// появления настоящих импортов). Дубли имён функций/сущностей между
// файлами - та же ошибка компиляции, что и дубль внутри одного файла
// (проверяется дальше по конвейеру, отдельной проверки здесь не нужно).
// "использовать" (шаг 11) внутри файла каталога - явная ошибка, а не тихо
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
	}
	return merged, nil
}
