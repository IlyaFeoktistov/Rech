package lsp

import (
	"errors"
	"strings"
	"testing"
)

func TestErrToDiagnosticExtractsLineAndColumn(t *testing.T) {
	err := errors.New(`парсер: строка 3, столбец 5: неожиданный токен "}"`)
	d := errToDiagnostic(err)
	if d.Line != 3 || d.Col != 5 {
		t.Fatalf("ожидалось line=3, col=5, получено %+v", d)
	}
	if !strings.Contains(d.Message, "неожиданный токен") {
		t.Fatalf("ожидалось сообщение про неожиданный токен, получено %q", d.Message)
	}
}

func TestErrToDiagnosticWithoutColumnDefaultsToOne(t *testing.T) {
	err := errors.New(`лексер: строка 7: недопустимый символ`)
	d := errToDiagnostic(err)
	if d.Line != 7 || d.Col != 1 {
		t.Fatalf("ожидалось line=7, col=1, получено %+v", d)
	}
}

func TestErrToDiagnosticWithoutLineFallsBackToLineOne(t *testing.T) {
	err := errors.New("исходный файл не в кодировке UTF-8")
	d := errToDiagnostic(err)
	if d.Line != 1 || d.Col != 1 {
		t.Fatalf("ожидалось line=1, col=1 (нет привязки к строке), получено %+v", d)
	}
	if d.Message != err.Error() {
		t.Fatalf("ожидалось сообщение целиком, получено %q", d.Message)
	}
}

// TestUpdateSetsDiagnosticOnParseError - синтаксическая ошибка (не закрыта
// фигурная скобка) должна попасть в d.diagnostic - см. package doc
// document.go, update().
func TestUpdateSetsDiagnosticOnParseError(t *testing.T) {
	d := newDoc("test://bad.rech", "функция главная {\n    вывести \"ок\"\n")
	if d.diagnostic == nil {
		t.Fatalf("ожидалась диагностика на незакрытую фигурную скобку")
	}
}

// TestUpdateClearsDiagnosticOnFix - документ с ошибкой, затем исправленный
// текст - diagnostic должен стать nil.
func TestUpdateClearsDiagnosticOnFix(t *testing.T) {
	d := newDoc("test://bad.rech", "функция главная {\n    вывести \"ок\"\n")
	if d.diagnostic == nil {
		t.Fatalf("ожидалась диагностика на незакрытую фигурную скобку")
	}
	d.update("функция главная {\n    вывести \"ок\"\n}\n")
	if d.diagnostic != nil {
		t.Fatalf("ожидалось, что диагностика снята после исправления, получено %+v", d.diagnostic)
	}
}
