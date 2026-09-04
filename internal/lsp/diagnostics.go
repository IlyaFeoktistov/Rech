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
// server.go (см. publishDiagnostics). Severity - LSP DiagnosticSeverity
// (errorSeverity=1 у ошибок разбора/анализа этого файла - см.
// errToDiagnostic, hintSeverity=4 у "@устарело" - см. deprecated.go, тот
// же файл несёт и deprecatedTag для Tags). Tags - nil у обычной ошибки.
type Diagnostic struct {
	Line, Col, Len int
	Message        string
	Severity       int
	Tags           []int
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
		return Diagnostic{Line: 1, Col: 1, Len: 1, Message: msg, Severity: errorSeverity}
	}
	line, _ := strconv.Atoi(m[1])
	col := 1
	if m[2] != "" {
		col, _ = strconv.Atoi(m[2])
	}
	return Diagnostic{Line: line, Col: col, Len: 1, Message: m[3], Severity: errorSeverity}
}

// setDiagnostic - одна ошибка (лексер - единственный оставшийся вызывающий,
// см. package doc document.go update - у него нет частичного результата,
// в отличие от lenient-парсера).
func (d *doc) setDiagnostic(err error) {
	d.diagnostics = []Diagnostic{errToDiagnostic(err)}
}

// setDiagnostics - см. package doc document.go, поле diagnostics.
func (d *doc) setDiagnostics(diags []Diagnostic) {
	d.diagnostics = diags
}

// parseDiagnostics превращает ошибки lenient-разбора (internal/parser,
// ParseProgramLenient - по одной на каждое сломанное верхнеуровневое
// объявление файла) в Diagnostic, той же errToDiagnostic, что и для любой
// другой ошибки компилятора. nil, если ошибок не было (разбор прошёл
// полностью успешно) - тогда append(...) в update() ниже просто ничего не
// добавляет к семантической ошибке, если она есть.
func parseDiagnostics(errs []error) []Diagnostic {
	if len(errs) == 0 {
		return nil
	}
	out := make([]Diagnostic, len(errs))
	for i, err := range errs {
		out[i] = errToDiagnostic(err)
	}
	return out
}
