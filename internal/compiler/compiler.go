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
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

// sourceFileNameRe - разрешённый алфавит имени .rech-файла (без расширения):
// строчные буквы (кириллица или латиница), цифры, "_". Никаких заглавных
// букв - на файловых системах без учёта регистра (Windows, macOS по
// умолчанию) "Файл.rech" и "файл.rech" - один и тот же файл, а на Linux -
// два разных; единственный надёжный способ не подорваться на этом -
// заглавные буквы в именах файлов не использовать вовсе. Правило и
// мотивация - docs/GRAMMAR.md, раздел "Имена файлов и кодировка".
var sourceFileNameRe = regexp.MustCompile(`^[a-zа-яё0-9_]+$`)

// validateSourceFileName проверяет имя .rech-файла: расширение, алфавит и
// форму Unicode NFC. Это уже сейчас единица импорта в будущей системе
// модулей ("использовать X из "имя_файла""), поэтому имя файла должно быть
// однозначным байт-в-байт, а не только "похожим на правильное".
func validateSourceFileName(path string) error {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != ".rech" {
		return fmt.Errorf("файл должен иметь расширение .rech, получено %q", base)
	}
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		return fmt.Errorf("имя файла не может быть пустым (получено %q)", base)
	}
	if !norm.NFC.IsNormalString(name) {
		return fmt.Errorf("имя файла %q не в нормализованной форме Unicode NFC - переименуйте файл в редакторе/файловом менеджере с нормализацией NFC", name)
	}
	if !sourceFileNameRe.MatchString(name) {
		return fmt.Errorf("имя файла %q должно состоять только из строчных букв (кириллица или латиница), цифр и \"_\" - например, имя_файла.rech", name)
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

// NewBackend создаёт стандартный Backend v0.1 - транспиляцию в C.
func NewBackend() Backend { return defaultBackend{} }

func (defaultBackend) Generate(prog *ast.Program) (string, error) {
	return cbackend.Generate(prog)
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

	cSource, err := c.Backend.Generate(prog)
	if err != nil {
		return "", fmt.Errorf("генерация C: %w", err)
	}

	cPath := base + ".c"
	if err := os.WriteFile(cPath, []byte(cSource), 0o644); err != nil {
		return "", fmt.Errorf("не удалось записать %s: %w", cPath, err)
	}

	binPath := base
	if err := c.Toolchain.CompileC(cPath, binPath); err != nil {
		return "", err
	}
	if !keepC {
		// Необязательная уборка - ошибка удаления не портит уже собранный
		// бинарник, поэтому не возвращается вызывающему коду как ошибка сборки.
		_ = os.Remove(cPath)
	}
	return binPath, nil
}

func (c *Compiler) parseFile(srcPath string) (*ast.Program, error) {
	if err := validateSourceFileName(srcPath); err != nil {
		return nil, err
	}
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл: %w", err)
	}
	return c.Frontend.Parse(string(srcBytes))
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
		prog, err := c.parseFile(filepath.Join(dirPath, name))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		merged.Functions = append(merged.Functions, prog.Functions...)
		merged.Entities = append(merged.Entities, prog.Entities...)
	}
	return merged, nil
}
