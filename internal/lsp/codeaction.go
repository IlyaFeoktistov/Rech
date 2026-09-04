// Автоимпорт ("textDocument/codeAction", quick fix) - когда диагностика
// (diagnostics.go) сообщает "имя не объявлено в этом файле и не
// импортировано" (module.resolveBareName, internal/module/walk.go -
// ЕДИНСТВЕННОЕ сообщение об ошибке именно этой формы во всём компиляторе,
// см. её же package doc), ищет объявление этого имени среди файлов
// проекта (workspace_scan.go) и предлагает вставить готовую строку
// "использовать <имя> из "<путь>"" - без неё пользователю пришлось бы
// искать нужный файл и высчитывать относительный путь руками.
package lsp

import (
	"fmt"
	"path/filepath"
	"regexp"
)

// CodeAction - один quick fix - НЕ полный LSP CodeAction (severity/kind и
// т. п. добавляет server.go при сборке ответа), только то, что нужно этому
// пакету: заголовок и ОДНА вставка новой строки.
type CodeAction struct {
	Title   string
	NewLine int // строка (1-индексная), ПЕРЕД которой вставляется NewText
	NewText string
}

// missingImportRe - см. package doc: единственная форма сообщения об
// ошибке, которую стоит предлагать чинить автоимпортом (не "сущность %q не
// объявлена" - internal/semantic/expr.go - та же ошибка ДО попытки
// линковки, менее специфичная; document.go предпочитает ошибку линковки,
// когда она есть, ровно чтобы здесь всегда был этот, более точный текст).
var missingImportRe = regexp.MustCompile(`"([^"]+)" не объявлено в этом файле и не импортировано`)

// codeActions - quick fix'ы для ТЕКУЩЕГО состояния документа (см. package
// doc) - пусто, если нет активной диагностики, путь документа неизвестен
// (не "file://", см. multifile.go, uriToPath) или диагностика - не про
// отсутствующий импорт.
func (d *doc) codeActions() []CodeAction {
	if d.diagnostic == nil || d.path == "" || d.astProg == nil {
		return nil
	}
	m := missingImportRe.FindStringSubmatch(d.diagnostic.Message)
	if m == nil {
		return nil
	}
	name := m[1]
	docDir := filepath.Dir(d.path)
	candidates := findWorkspaceDecl(docDir, d.path, name)
	insertLine := d.importInsertLine()
	out := make([]CodeAction, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, CodeAction{
			Title:   fmt.Sprintf("Импортировать %q из %q", name, c.ImportPath),
			NewLine: insertLine,
			NewText: fmt.Sprintf("использовать %s из %q\n", name, c.ImportPath),
		})
	}
	return out
}

// importInsertLine - строка (1-индексная), ПЕРЕД которой вставляется новый
// "использовать" - сразу после ПОСЛЕДНЕГО уже написанного (сохраняет их
// все вместе одним блоком в начале файла, обычный стиль всех examples/), 1
// (самое начало файла) - если импортов ещё не было вовсе.
func (d *doc) importInsertLine() int {
	last := 0
	for _, u := range d.astProg.Uses {
		if u.Line > last {
			last = u.Line
		}
	}
	return last + 1
}
