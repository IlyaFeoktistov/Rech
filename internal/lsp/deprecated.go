// Подсветка "@устарело" в doc-комментарии (свой, русский тег - не
// "@deprecated" JSDoc, специфичный для ЭТОГО LSP - сам компилятор о нём не
// знает вовсе, ни парсер, ни семантика ничего не проверяют) - функция/
// сущность/метод, чей Doc содержит "@устарело", помечается диагностикой с
// тегом DiagnosticTag.Deprecated (2, см. LSP spec) на КАЖДОМ вхождении её
// имени в текущем документе - редактор рисует его зачёркнутым везде, где
// оно использовано, не только там, где стоит курсор (в отличие от hover,
// который и так уже показывает doc-комментарий целиком).
package lsp

import (
	"regexp"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// deprecatedTag/hintSeverity - см. LSP spec, DiagnosticTag/
// DiagnosticSeverity - "устарело" не ошибка и не предупреждение, поэтому
// Hint (4), не Error (1, см. errToDiagnostic).
const (
	deprecatedTag   = 2
	hintSeverity    = 4
	warningSeverity = 2
	errorSeverity   = 1
)

// deprecatedRe - "@устарело" и необязательный текст после него до конца
// строки (тот же комментарий переносится в сообщение диагностики - "уберите
// вызов, замените на Y" и т. п., если автор его написал). Без "\b" после
// тега - Go regexp (RE2) понимает "\b" только как границу МЕЖДУ ASCII-
// "словом" ([0-9A-Za-z_]) и не-словом; кириллица для него не "слово"
// вообще, значит "\b" сразу после кириллической буквы никогда не совпадёт
// (найдено живой проверкой - "@устарело\b" не матчился НИ РАЗУ) - "[ \t]*"
// и так уже требует пробел/таб/конец строки сразу после тега, отдельная
// граница слова не нужна.
var deprecatedRe = regexp.MustCompile(`(?i)@устарело[ \t]*(.*)`)

// deprecatedInfo - ok=true, если doc содержит "@устарело" - message
// тогда текст диагностики (свой, если есть, иначе просто "устарело").
func deprecatedInfo(doc string) (message string, ok bool) {
	m := deprecatedRe.FindStringSubmatch(doc)
	if m == nil {
		return "", false
	}
	msg := strings.TrimSpace(m[1])
	if msg == "" {
		return "устарело", true
	}
	return "устарело: " + msg, true
}

// deprecatedDiagnostics - Diagnostic (Tags: [deprecatedTag]) на каждом
// вхождении имени устаревшей функции/сущности/метода в d.tokens - и
// свободные функции/сущности (d.funcs/d.entities - уже несут алиасы
// импортов, см. multifile.go, поэтому работает и для устаревшего имени,
// объявленного в ДРУГОМ файле), и методы каждой сущности этого документа.
// Приближённо (совпадение по токену/значению, теми же functionRefTokens/
// methodRefTokens/entityRefTokens, что и references.go/codelens.go) - не
// разрешает область видимости по-настоящему.
func (d *doc) deprecatedDiagnostics() []Diagnostic {
	if d.irProg == nil {
		return nil
	}
	var out []Diagnostic
	seenFunc := map[string]bool{}
	for name, fn := range d.funcs {
		if seenFunc[name] {
			continue
		}
		seenFunc[name] = true
		if msg, ok := deprecatedInfo(fn.Doc); ok {
			out = append(out, d.tokenDiagnostics(d.functionRefTokens(name), msg)...)
		}
	}
	seenEnt := map[string]bool{}
	for name, ent := range d.entities {
		if seenEnt[name] {
			continue
		}
		seenEnt[name] = true
		if msg, ok := deprecatedInfo(ent.Doc); ok {
			out = append(out, d.tokenDiagnostics(d.entityRefTokens(name), msg)...)
		}
		for _, m := range ent.Methods {
			if msg, ok := deprecatedInfo(m.Doc); ok {
				out = append(out, d.tokenDiagnostics(d.methodRefTokens(m.Name), msg)...)
			}
		}
	}
	return out
}

// tokenDiagnostics строит один Diagnostic на каждый токен из toks - общий
// message, тег "устарело".
func (d *doc) tokenDiagnostics(toks []lexer.Token, message string) []Diagnostic {
	out := make([]Diagnostic, len(toks))
	for i, tok := range toks {
		out[i] = Diagnostic{
			Line: tok.Line, Col: tok.Col, Len: len([]rune(tok.Value)),
			Message: message, Severity: hintSeverity, Tags: []int{deprecatedTag},
		}
	}
	return out
}
