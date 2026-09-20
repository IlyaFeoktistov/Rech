// Диагностика ("textDocument/publishDiagnostics") - подсветка ошибки
// разбора/анализа прямо в открытом документе, без ручного запуска "Rech:
// собрать" - тот же текст ошибки, что и у настоящего компилятора (лексер/
// парсер/семантика/module.Load+Link), просто доставленный сразу, на каждое
// didOpen/didChange, а не по требованию через отдельную команду.
package lsp

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
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
// ГДЕ-ТО лучше, чем не показать вовсе. lines - текст ДОКУМЕНТА (не
// обязательно того же файла, к которому относится ошибка - см. package doc
// sharpenDiagnosticRange) построчно, для уточнения столбца/длины подсветки:
// большинство сообщений компилятора (256 из ~290 на момент фикса) вообще не
// несут столбца - без lines Col/Len были бы всегда 1/1 (диагностика
// невидимо садится на самый первый символ строки, а не на ошибочное слово -
// репортнуто пользователем).
func errToDiagnostic(err error, lines []string) Diagnostic {
	msg := err.Error()
	m := diagLineRe.FindStringSubmatch(msg)
	if m == nil {
		return Diagnostic{Line: 1, Col: 1, Len: 1, Message: msg, Severity: errorSeverity}
	}
	line, _ := strconv.Atoi(m[1])
	hasCol := m[2] != ""
	col := 1
	if hasCol {
		col, _ = strconv.Atoi(m[2])
	}
	text := m[3]
	var lineText string
	if line >= 1 && line <= len(lines) {
		lineText = lines[line-1]
	}
	col, length := sharpenDiagnosticRange(text, lineText, col, hasCol)
	return Diagnostic{Line: line, Col: col, Len: length, Message: text, Severity: errorSeverity}
}

// quotedIdentRe - первая подстрока в двойных кавычках сообщения об ошибке -
// самый частый случай (fmt.Errorf с "%q") цитирует ИМЕННО то опознанное
// слово, из-за которого возникла ошибка ("переменная %q не объявлена",
// "функция %q не объявлена" и т. п.) - see sharpenDiagnosticRange.
var quotedIdentRe = regexp.MustCompile(`"([^"]*)"`)

// sharpenDiagnosticRange уточняет столбец/длину диагностики по тексту
// строки-источника (lineText, может быть "" - строка вне диапазона
// известных lines, см. errToDiagnostic):
//  1. Если сообщение цитирует конкретное слово (quotedIdentRe) - ищет его
//     как ЦЕЛОЕ СЛОВО (проверка границы - соседние руны не буква/цифра/"_",
//     unicode-осознанно, кириллица - тоже буквы) в lineText, сначала от уже
//     известного столбца компилятора (если он был указан), иначе с начала
//     строки - подсвечивает именно это слово, не всю строку.
//  2. Иначе, если компилятор вообще не указал столбец (hasCol=false) -
//     подсвечивает весь непустой текст строки целиком (лучше, чем один
//     невидимый символ в столбце 1, и не требует гадать конкретное слово).
//  3. Иначе (столбец был явно указан, но подходящей цитаты не нашлось) -
//     доверяет столбцу компилятора как есть, длина - 1 символ (как раньше).
func sharpenDiagnosticRange(message, lineText string, col int, hasCol bool) (int, int) {
	runes := []rune(lineText)
	if m := quotedIdentRe.FindStringSubmatch(message); m != nil && m[1] != "" {
		want := []rune(m[1])
		from := 0
		if hasCol && col-1 >= 0 && col-1 <= len(runes) {
			from = col - 1
		}
		if idx := indexWholeWord(runes, want, from); idx >= 0 {
			return idx + 1, len(want)
		}
		if from > 0 {
			if idx := indexWholeWord(runes, want, 0); idx >= 0 {
				return idx + 1, len(want)
			}
		}
	}
	if !hasCol {
		if start, end := trimmedRuneRange(runes); end > start {
			return start + 1, end - start
		}
	}
	return col, 1
}

// isIdentRune - руна, из которых состоят идентификаторы Rech (буква
// Unicode-алфавита - кириллица включена, цифра, "_") - используется только
// для проверки ГРАНИЦЫ найденной цитаты (соседняя руна не должна быть этим
// же классом, иначе совпадение - часть более длинного слова, не всё слово).
func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// indexWholeWord ищет want в runes, начиная с индекса from, требуя, чтобы
// найденное вхождение было ЦЕЛЫМ словом (соседние символы, если есть, не
// isIdentRune) - иначе, например, поиск "имя" совпал бы и внутри "имя_2".
// -1, если не нашлось.
func indexWholeWord(runes, want []rune, from int) int {
	if len(want) == 0 || from < 0 {
		return -1
	}
	for i := from; i+len(want) <= len(runes); i++ {
		if string(runes[i:i+len(want)]) != string(want) {
			continue
		}
		if i > 0 && isIdentRune(runes[i-1]) {
			continue
		}
		if end := i + len(want); end < len(runes) && isIdentRune(runes[end]) {
			continue
		}
		return i
	}
	return -1
}

// trimmedRuneRange - [start, end) непустого содержимого runes (без ведущих/
// хвостовых пробелов/табов) - start==end, если строка целиком пустая/из
// одних пробелов.
func trimmedRuneRange(runes []rune) (start, end int) {
	start = 0
	for start < len(runes) && (runes[start] == ' ' || runes[start] == '\t') {
		start++
	}
	end = len(runes)
	for end > start && (runes[end-1] == ' ' || runes[end-1] == '\t' || runes[end-1] == '\r') {
		end--
	}
	return start, end
}

// setDiagnostic - одна ошибка (лексер - единственный оставшийся вызывающий,
// см. package doc document.go update - у него нет частичного результата,
// в отличие от lenient-парсера).
func (d *doc) setDiagnostic(err error) {
	d.diagnostics = []Diagnostic{errToDiagnostic(err, strings.Split(d.text, "\n"))}
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
func parseDiagnostics(errs []error, lines []string) []Diagnostic {
	if len(errs) == 0 {
		return nil
	}
	out := make([]Diagnostic, len(errs))
	for i, err := range errs {
		out[i] = errToDiagnostic(err, lines)
	}
	return out
}

// semanticDiagnosticsFor - как errToDiagnostic, но разворачивает err в
// НЕСКОЛЬКО Diagnostic, если это internal/diagnostics.Bag (semantic.Analyze
// теперь может вернуть несколько независимых ошибок за один прогон - см.
// internal/semantic/analyzer.go, buildIR) - без этого LSP видел бы только
// текст-сводку Bag.Error() ("... (и ещё диагностик: N)") вместо реальной
// позиции каждой из них. Обычная одиночная ошибка (в т.ч. до-миграционная,
// без internal/diagnostics) - тот же путь, что и раньше, один Diagnostic.
func semanticDiagnosticsFor(err error, lines []string) []Diagnostic {
	var bag *diagnostics.Bag
	if errors.As(err, &bag) {
		diags := bag.Diagnostics()
		out := make([]Diagnostic, len(diags))
		for i, d := range diags {
			out[i] = errToDiagnostic(d, lines)
		}
		return out
	}
	return []Diagnostic{errToDiagnostic(err, lines)}
}
