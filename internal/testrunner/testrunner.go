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

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
	"github.com/IlyaFeoktistov/Rech/internal/interp"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// testFileSuffix - суффикс имени тестового файла (см. package doc).
const testFileSuffix = "_тест.rech"

// Result - исход одного тестового файла.
type Result struct {
	Path   string
	Passed bool
	Err    error
	Stdout string
}

// Discover находит тестовые файлы в root: если root - файл, это ровно один
// "тест" (независимо от имени - если пользователь явно указал файл, ему не
// нужно повторно проверять соглашение об имени); если root - каталог,
// рекурсивно находит все файлы с суффиксом testFileSuffix, пропуская
// скрытые каталоги (например, ".git"). Результат отсортирован по пути -
// детерминированный порядок вывода "rech test".
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
		if strings.HasSuffix(d.Name(), testFileSuffix) {
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

// RunFile прогоняет один тестовый файл: разбор (тот же Frontend, что и у
// компилятора - internal/compiler.NewFrontend - не дублировать проверку
// UTF-8/NFC отдельно), семантический анализ, исполнение через interp.Run.
// Ошибка компиляции (лексер/парсер/семантика) - тоже "тест не прошёл", не
// отдельная категория - пользователю всё равно нужно увидеть, что не так.
func RunFile(path string) Result {
	src, err := os.ReadFile(path)
	if err != nil {
		return Result{Path: path, Err: err}
	}
	prog, err := compiler.NewFrontend().Parse(string(src))
	if err != nil {
		return Result{Path: path, Err: err}
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		return Result{Path: path, Err: err}
	}
	var buf bytes.Buffer
	if err := interp.Run(irProg, &buf); err != nil {
		return Result{Path: path, Err: err, Stdout: buf.String()}
	}
	return Result{Path: path, Passed: true, Stdout: buf.String()}
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
