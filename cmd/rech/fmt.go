package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/format"
	"github.com/IlyaFeoktistov/Rech/internal/module"
)

// runFmt - "rech fmt [--write|-w] <файл.rech>|<каталог>" - тем же принципом,
// что "gofmt": без --write печатает
// отформатированный текст в stdout (для файла), с --write переписывает на
// месте. Каталог обходится рекурсивно (все ".rech", пропускает скрытые
// каталоги, тем же принципом, что testrunner.Discover) - без --write
// печатать содержимое сразу нескольких файлов подряд в stdout бессмысленно,
// поэтому для каталога --write обязателен.
func runFmt(args []string) bool {
	write := false
	var target string
	for _, a := range args {
		switch a {
		case "--write", "-w":
			write = true
		default:
			if target != "" {
				fmt.Fprintf(os.Stderr, "ошибка: лишний аргумент %q\n", a)
				return false
			}
			target = a
		}
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "использование: rech fmt [--write|-w] <файл.rech>|<каталог>")
		return false
	}

	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return false
	}

	if info.IsDir() {
		if !write {
			fmt.Fprintln(os.Stderr, "ошибка: форматирование целого каталога требует --write (иначе некуда печатать сразу несколько файлов)")
			return false
		}
		files, err := discoverRechFiles(target)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка:", err)
			return false
		}
		ok := true
		for _, f := range files {
			if !formatOneFile(f, true) {
				ok = false
			}
		}
		return ok
	}

	return formatOneFile(target, write)
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
	out, err := format.Format(string(src))
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
