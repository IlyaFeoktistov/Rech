package protoimport

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Import читает path (".proto"-файл) и все его ЛОКАЛЬНЫЕ импорты
// (рекурсивно, относительно каталога импортирующего файла), разрешает
// ссылки типов и возвращает готовый Rech-исходник (сущности/структуры с
// тегами `protobuf:N`) - см. package doc generate.go для точного списка
// поддержанного/неподдержанного. "google/protobuf/..." (well-known types)
// - НЕ разрешаются, явная ошибка (см. collectFiles).
func Import(path string) (string, error) {
	files, err := collectFiles(path)
	if err != nil {
		return "", err
	}
	return buildAndGenerate(files)
}

// collectFiles обходит граф импортов от path, начиная с самого path -
// порядок результата: зависимости (импорты) РАНЬШЕ файла, который их
// импортирует (пост-порядок обхода) - не влияет на корректность (реестр
// типов строится по ВСЕМ файлам разом, до разрешения полей, см.
// buildAndGenerate), только на порядок печати сообщений в результате.
// visited - по абсолютному пути, защита от повторного разбора одного и
// того же файла (типичная ситуация - "diamond import", A и B оба
// импортируют C) и от бесконечной рекурсии при цикле импортов.
func collectFiles(path string) ([]*file, error) {
	var out []*file
	visited := map[string]bool{}
	var visit func(p string) error
	visit = func(p string) error {
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		if visited[abs] {
			return nil
		}
		visited[abs] = true

		src, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("не удалось прочитать %q: %w", p, err)
		}
		f, err := parseFile(string(src))
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		f.path = p

		dir := filepath.Dir(p)
		for _, imp := range f.imports {
			if isWellKnownImport(imp.path) {
				return fmt.Errorf("%s:%d: импорт %q - well-known type protobuf (google/protobuf/...) не поддержан этим импортёром (нет соответствующего встроенного типа Rech для него, см. package doc generate.go)", p, imp.line, imp.path)
			}
			importedPath := filepath.Join(dir, imp.path)
			if _, err := os.Stat(importedPath); err != nil {
				return fmt.Errorf("%s:%d: импорт %q не найден рядом с файлом (искал %q) - этот импортёр разрешает только локальные относительные импорты", p, imp.line, imp.path, importedPath)
			}
			if err := visit(importedPath); err != nil {
				return err
			}
		}
		out = append(out, f)
		return nil
	}
	if err := visit(path); err != nil {
		return nil, err
	}
	return out, nil
}

// isWellKnownImport - "google/protobuf/..." (Timestamp/Duration/Any/
// Empty/Struct/...) - см. package doc generate.go, почему не поддержаны.
func isWellKnownImport(path string) bool {
	return strings.HasPrefix(path, "google/protobuf/")
}
