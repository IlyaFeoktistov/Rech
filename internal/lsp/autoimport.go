package lsp

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Автоимпорт при сохранении ("textDocument/willSaveWaitUntil"): каждое имя,
// которое "не объявлено в этом файле и не импортировано" и объявлено ровно
// в одном файле проекта, получает строку "использовать <имя> из "<путь>""
// до записи файла на диск. Имя, найденное в нескольких файлах, не
// трогается - его чинит quick fix (codeaction.go), где выбор за
// пользователем.

// maxAutoImports - предел имён за одно сохранение: анализ останавливается
// на первой ошибке, поэтому каждое следующее имя - новый проход анализа.
const maxAutoImports = 16

// autoImportEdit - текст, который нужно вставить перед строкой line
// (1-индексная) документа, чтобы импортировать всё однозначно найденное;
// "" - вставлять нечего.
func (d *doc) autoImportEdit() (line int, text string) {
	if d.path == "" || d.astProg == nil {
		return 0, ""
	}
	line = d.importInsertLine()
	docDir := filepath.Dir(d.path)
	lines := strings.Split(d.text, "\n")
	var added []string
	cur := d
	for range maxAutoImports {
		imp, ok := cur.nextUnambiguousImport(docDir)
		if !ok || slices.Contains(added, imp) {
			// Тот же импорт второй раз - вставка не помогла, дальше не идём.
			break
		}
		added = append(added, imp)
		withImports := append(append(append([]string{}, lines[:line-1]...), added...), lines[line-1:]...)
		cur = &doc{uri: d.uri, path: d.path}
		cur.update(strings.Join(withImports, "\n"))
	}
	if len(added) == 0 {
		return 0, ""
	}
	return line, strings.Join(added, "\n") + "\n"
}

// nextUnambiguousImport - строка "использовать" для первого имени без
// импорта, объявленного ровно в одном файле проекта.
func (d *doc) nextUnambiguousImport(docDir string) (string, bool) {
	for _, diag := range d.diagnostics {
		m := missingImportRe.FindStringSubmatch(diag.Message)
		if m == nil {
			continue
		}
		if candidates := findWorkspaceDecl(docDir, d.path, m[1]); len(candidates) == 1 {
			return fmt.Sprintf("использовать %s из %q", m[1], candidates[0].ImportPath), true
		}
	}
	return "", false
}
