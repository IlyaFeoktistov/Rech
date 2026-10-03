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
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/module"
	"github.com/IlyaFeoktistov/Rech/internal/stdlib"
)

// stdModule - стандартный модуль встроенного name.
func stdModule(name string) string {
	m, _ := stdlib.ModuleOf(name)
	return m
}

// CodeAction - один quick fix - НЕ полный LSP CodeAction (severity/kind и
// т. п. добавляет server.go при сборке ответа), только то, что нужно этому
// пакету: заголовок и ОДНА вставка новой строки.
type CodeAction struct {
	Title   string
	NewLine int // строка (1-индексная), ПЕРЕД которой вставляется NewText
	NewText string
	// Replace - NewText заменяет саму строку NewLine (дописать имя в уже
	// существующий импорт стандартного модуля), а не вставляется перед ней.
	Replace bool
}

// missingImportRe - см. package doc: единственная форма сообщения об
// ошибке, которую стоит предлагать чинить автоимпортом (не "сущность %q не
// объявлена" - internal/semantic/expr.go - та же ошибка ДО попытки
// линковки, менее специфичная; document.go предпочитает ошибку линковки,
// когда она есть, ровно чтобы здесь всегда был этот, более точный текст).
var missingImportRe = regexp.MustCompile(`"([^"]+)" не объявлено в этом файле и не импортировано`)

// codeActions - quick fix'ы для ТЕКУЩЕГО состояния документа (см. package
// doc) - пусто, если нет ни одной диагностики, путь документа неизвестен
// (не "file://", см. multifile.go, uriToPath) или ни одна диагностика - не
// про отсутствующий импорт. d.diagnostics может нести несколько записей
// сразу (lenient-разбор, см. document.go update) - перебираются все, не
// только первая, хотя на практике сообщение этой формы даёт только
// семантика/линковка (см. package doc missingImportRe), она в d.diagnostics
// всегда одна, если вообще есть.
func (d *doc) codeActions() []CodeAction {
	if d.path == "" || d.astProg == nil {
		return nil
	}
	docDir := filepath.Dir(d.path)
	insertLine := d.importInsertLine()
	var out []CodeAction
	for _, diag := range d.diagnostics {
		m := missingImportRe.FindStringSubmatch(diag.Message)
		if m == nil {
			continue
		}
		name := m[1]
		if line, text, replace, ok := module.StdImportEdit(d.text, name); ok {
			out = append(out, CodeAction{
				Title:   fmt.Sprintf("Импортировать %q из %s", name, stdlib.Prefix+stdModule(name)),
				NewLine: line,
				NewText: text,
				Replace: replace,
			})
			continue
		}
		for _, c := range findWorkspaceDecl(docDir, d.path, name) {
			out = append(out, CodeAction{
				Title:   fmt.Sprintf("Импортировать %q из %q", name, c.ImportPath),
				NewLine: insertLine,
				NewText: fmt.Sprintf("использовать %s из %q\n", name, c.ImportPath),
			})
		}
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

// notImportedBuiltinInFile - err связывания сообщает о встроенном имени
// без импорта в самом файле path (а не в файле, который он импортирует).
func notImportedBuiltinInFile(err error, path string) bool {
	if _, ok := module.IsNotImportedBuiltin(err); !ok || path == "" {
		return false
	}
	return strings.HasPrefix(err.Error(), filepath.Clean(path)+": ")
}
