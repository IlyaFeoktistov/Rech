package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/format"
	"github.com/IlyaFeoktistov/Rech/internal/frontend"
	"github.com/IlyaFeoktistov/Rech/internal/module"
	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// runFmt - "rech fmt [--write|-w | --check] <файл.rech>|<каталог>" - тем же
// принципом, что "gofmt": без флагов печатает отформатированный текст в
// stdout (для файла), с --write переписывает на месте, с --check ничего не
// пишет и не переписывает - только проверяет (тем же принципом, что
// "gofmt -l"): печатает путь КАЖДОГО файла, который "rech fmt --write"
// изменил бы, и возвращает false, если такой файл нашёлся хоть один - для
// CI ("проект отформатирован?"), без побочного эффекта записи. Каталог
// обходится рекурсивно (все ".rech", пропускает скрытые каталоги, тем же
// принципом, что testrunner.Discover) - без --write/--check печатать
// содержимое сразу нескольких файлов подряд в stdout бессмысленно, поэтому
// для каталога один из двух обязателен.
func runFmt(args []string) bool {
	write := false
	check := false
	var target, as string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--write", "-w":
			write = true
		case "--check":
			check = true
		case "--as":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "ошибка: после --as ожидался путь")
				return false
			}
			i++
			as = args[i]
		default:
			if target != "" {
				fmt.Fprintf(os.Stderr, "ошибка: лишний аргумент %q\n", a)
				return false
			}
			target = a
		}
	}
	if write && check {
		fmt.Fprintln(os.Stderr, "ошибка: --write и --check несовместимы")
		return false
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "использование: rech fmt [--write|-w | --check] <файл.rech>|<каталог> | rech fmt --as <путь> <файл.rech>")
		return false
	}

	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}

	if info.IsDir() {
		if !write && !check {
			fmt.Fprintln(os.Stderr, "ошибка: форматирование целого каталога требует --write или --check (иначе некуда печатать сразу несколько файлов)")
			return false
		}
		files, err := discoverRechFiles(target)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			return false
		}
		ok := true
		for _, f := range files {
			if check {
				if !checkOneFile(f) {
					ok = false
				}
				continue
			}
			if !formatOneFile(f, true) {
				ok = false
			}
		}
		return ok
	}

	if as != "" {
		return formatAs(target, as)
	}
	if check {
		return checkOneFile(target)
	}
	return formatOneFile(target, write)
}

// formatAs - "--as <путь>": текст файла target (копия несохранённого
// документа редактора) форматируется так, будто лежит по пути as -
// импорты разрешаются от настоящего места файла. Результат - в stdout.
func formatAs(target, as string) bool {
	src, err := os.ReadFile(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}
	out, err := fixImportsAndFormat(as, string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %s: %v\n", as, err)
		return false
	}
	fmt.Print(out)
	return true
}

// formatOneFile форматирует один файл - при write==true переписывает его на
// месте (молча при успехе, тем же принципом, что "gofmt -w"), иначе печатает
// результат в stdout.
func formatOneFile(path string, write bool) bool {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}
	out, err := fixImportsAndFormat(path, string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %s: %v\n", path, err)
		return false
	}
	if !write {
		fmt.Print(out)
		return true
	}
	if out == string(src) {
		return true
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}
	return true
}

// checkOneFile - "--check" для одного файла (см. package doc runFmt выше) -
// НЕ пишет ничего на диск ни при каком исходе, в отличие от formatOneFile.
// Печатает путь в stdout и возвращает false, только если файл ещё не
// отформатирован ("gofmt -l") - ошибка разбора/чтения, как и у
// formatOneFile, идёт в stderr.
func checkOneFile(path string) bool {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}
	out, err := fixImportsAndFormat(path, string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %s: %v\n", path, err)
		return false
	}
	if out == string(src) {
		return true
	}
	fmt.Println(path)
	return false
}

// discoverRechFiles - тот же принцип, что internal/testrunner.Discover, но
// без фильтра по суффиксу имени - ЛЮБОЙ файл каталога с одним из
// module.SourceExtensions, рекурсивно, пропускает скрытые каталоги
// (".git" и т. п.).
func discoverRechFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if module.HasSourceExtension(d.Name()) {
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

// fixImportsAndFormat - недостающие импорты встроенных имён из стандартных
// модулей (module.FixStdImports, как goimports), затем форматирование.
// Файл, граф импортов которого не разбирается, только форматируется -
// ошибку разбора покажет сам format.Format.
func fixImportsAndFormat(path, src string) (string, error) {
	if root, err := project.RootFor(path); err == nil {
		if fixed, err := module.FixStdImports(path, src, frontend.NewFrontend(), root); err == nil {
			src = fixed
		}
	}
	return format.Format(src)
}
