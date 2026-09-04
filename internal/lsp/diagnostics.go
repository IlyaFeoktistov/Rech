// Диагностика ("textDocument/publishDiagnostics") - подсветка ошибки
// разбора/анализа прямо в открытом документе, без ручного запуска "Rech:
// собрать" - тот же текст ошибки, что и у настоящего компилятора (лексер/
// парсер/семантика/module.Load+Link), просто доставленный сразу, на каждое
// didOpen/didChange, а не по требованию через отдельную команду.
package lsp

import (
	"regexp"
	"strconv"
)

// Diagnostic - одна запись диагностики в форме, которую JSON-кодирует
// server.go (см. handlePublishDiagnostics) - LSP Diagnostic.severity: 1 -
// Error (лексер/парсер/семантика/линковка - все три вида ошибок этого
// пакета одинаково фатальны для остального анализа документа, отдельных
// уровней Warning/Information/Hint здесь нет).
type Diagnostic struct {
	Line, Col, Len int
	Message        string
}

// diagLineRe - тот же регекс, что и parseDiagnostics в rech-vscode/
// extension.js (используется для ВЫВОДА "rech build" в терминале) -
// продублирован здесь, а не вызван оттуда (JS недоступен из Go) -
// "строка N[, столбец M]: сообщение" - соглашение форматирования ошибок
// всего компилятора (fmt.Errorf с %w несёт исходную "строка N: ..."
// глубже в цепочке обёрток, регексп ищет её ГДЕ УГОДНО в итоговой строке,
// не только в начале).
var diagLineRe = regexp.MustCompile(`строка (\d+)(?:, столбец (\d+))?: (.+)`)

// errToDiagnostic превращает err.Error() в Diagnostic - строка/столбец, если
// сообщение содержит "строка N[, столбец M]: ..." (см. diagLineRe),
// иначе - вся ошибка целиком на строке 1 (лексер/module.Load иногда
// сообщают об ошибке, вообще не привязанной к конкретной строке - плохая
// кодировка файла, не найден файл импорта и т. п.) - показать диагностику
// ГДЕ-ТО лучше, чем не показать вовсе.
func errToDiagnostic(err error) Diagnostic {
	msg := err.Error()
	m := diagLineRe.FindStringSubmatch(msg)
	if m == nil {
		return Diagnostic{Line: 1, Col: 1, Len: 1, Message: msg}
	}
	line, _ := strconv.Atoi(m[1])
	col := 1
	if m[2] != "" {
		col, _ = strconv.Atoi(m[2])
	}
	return Diagnostic{Line: line, Col: col, Len: 1, Message: m[3]}
}

// setDiagnostic - см. package doc doc/document.go, поле diagnostic.
func (d *doc) setDiagnostic(err error) {
	diag := errToDiagnostic(err)
	d.diagnostic = &diag
}
