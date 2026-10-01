// Package testrunner реализует "rech test" - поиск и запуск тестовых
// файлов Rech.
//
// Тестовый файл - обычная программа Rech со своей "главная", отличается от
// прочих .rech-файлов только суффиксом имени ("_тест.rech", как "_test.go"
// в Go). Проверки внутри - обычная инструкция "утверждать <условие>"
// (docs/SPEC.md), доступная в любой функции, не только в тестовых файлах.
//
// Тесты запускаются через internal/interp, а не через cbackend+cc - без
// вызова системного компилятора. Оба backend'а читают один и тот же
// internal/ir, разрешённый одним internal/semantic.Analyze, поэтому
// поведение теста и поведение скомпилированной программы совпадают по
// построению, а не по соглашению.
package testrunner

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/frontend"
	"github.com/IlyaFeoktistov/Rech/internal/interp"
	"github.com/IlyaFeoktistov/Rech/internal/module"
	"github.com/IlyaFeoktistov/Rech/internal/project"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// TestFileSuffix - каноническое (латиница) значение суффикса имени
// тестового файла, для сообщений/документации (см. package doc). Сама
// проверка имени - IsTestFile - принимает тот же суффикс с ЛЮБЫМ из
// module.SourceExtensions (".rech"/".речь"/".р"), не только ".rech".
const TestFileSuffix = "_тест.rech"

// testFileBase - суффикс имени тестового файла БЕЗ расширения - основа
// для перебора всех module.SourceExtensions в IsTestFile.
const testFileBase = "_тест"

// IsTestFile - true, если name оканчивается на testFileBase с одним из
// module.SourceExtensions - тот же тестовый файл, что и раньше
// ("_тест.rech"), но узнаётся и через кириллические синонимы расширения.
func IsTestFile(name string) bool {
	for _, ext := range module.SourceExtensions {
		if strings.HasSuffix(name, testFileBase+ext) {
			return true
		}
	}
	return false
}

// Result - исход одного тестового файла.
type Result struct {
	Path   string
	Passed bool
	Err    error
	Stdout string
	// Warnings - предупреждения анализа файла (ir.Program.Warnings): тест
	// от них не проваливается.
	Warnings []error
}

// Discover находит тестовые файлы в root: если root - файл, это ровно один
// "тест" (независимо от имени - если пользователь явно указал файл, ему не
// нужно повторно проверять соглашение об имени); если root - каталог,
// рекурсивно находит все файлы, для которых IsTestFile(имя) верно,
// пропуская скрытые каталоги (например, ".git"). Результат отсортирован по
// пути - детерминированный порядок вывода "rech test".
func Discover(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("не удалось найти %q: %w", root, err)
	}
	if !info.IsDir() {
		return []string{root}, nil
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if IsTestFile(d.Name()) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// RunFile прогоняет один тестовый файл: разбор с разрешением "использовать"
// (тот же module.Load/Link, что и у компилятора - internal/compiler.Build -
// не дублировать проверку UTF-8/NFC/видимости между файлами отдельно),
// семантический анализ, исполнение через interp.Run. Ошибка компиляции
// (лексер/парсер/семантика) - тоже "тест не прошёл", не отдельная
// категория - пользователю всё равно нужно увидеть, что не так.
func RunFile(path string) Result {
	// root - каталог главного файла проекта, если path (тестовый файл,
	// обычно НЕ главный файл) лежит под каким-то .rechproj - без этого
	// "использовать ... из "@путь"" внутри теста резолвился бы от
	// каталога САМОГО теста, а не от корня программы (см.
	// internal/project.RootFor, internal/module.LoadWithRoot). "" (path
	// вне какого-либо .rechproj) - module.LoadWithRoot ведёт себя как
	// module.Load.
	root, err := project.RootFor(path)
	if err != nil {
		return Result{Path: path, Err: err}
	}
	loaded, err := module.LoadWithRoot(path, frontend.NewFrontend(), nil, nil, root)
	if err != nil {
		return Result{Path: path, Err: err}
	}
	prog, err := loaded.Link()
	if err != nil {
		return Result{Path: path, Err: err}
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		return Result{Path: path, Err: err}
	}
	// "сборка.библиотеки"/"путь_библиотек" из .rechproj (если есть) -
	// нужны только FFI интерпретатора (ffi.go, resolveFFISymbol), чтобы
	// знать, где искать C-символ через dlopen/dlsym - cbackend их не через
	// ir.Program получает вовсе (сам вызывает линкер).
	pf, err := project.LoadFor(path)
	if err != nil {
		return Result{Path: path, Err: err}
	}
	if pf != nil {
		irProg.FFILibraries = pf.Библиотеки()
		irProg.FFILibraryPaths = pf.ПутиБиблиотек()
		irProg.OptionsYAML = pf.Опции
	}
	var buf bytes.Buffer
	if err := interp.Run(irProg, &buf, os.Stdin); err != nil {
		return Result{Path: path, Err: err, Stdout: buf.String(), Warnings: irProg.Warnings}
	}
	return Result{Path: path, Passed: true, Stdout: buf.String(), Warnings: irProg.Warnings}
}

// Run находит и запускает все тестовые файлы под root (см. Discover).
func Run(root string) ([]Result, error) {
	files, err := Discover(root)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(files))
	for _, f := range files {
		results = append(results, RunFile(f))
	}
	return results, nil
}
