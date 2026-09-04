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
// фигурная скобка) должна попасть в d.diagnostics - см. package doc
// document.go, update().
func TestUpdateSetsDiagnosticOnParseError(t *testing.T) {
	d := newDoc("test://bad.rech", "функция главная {\n    вывести \"ок\"\n")
	if len(d.diagnostics) == 0 {
		t.Fatalf("ожидалась диагностика на незакрытую фигурную скобку")
	}
}

// TestUpdateClearsDiagnosticOnFix - документ с ошибкой, затем исправленный
// текст - diagnostics должен стать пустым.
func TestUpdateClearsDiagnosticOnFix(t *testing.T) {
	d := newDoc("test://bad.rech", "функция главная {\n    вывести \"ок\"\n")
	if len(d.diagnostics) == 0 {
		t.Fatalf("ожидалась диагностика на незакрытую фигурную скобку")
	}
	d.update("функция главная {\n    вывести \"ок\"\n}\n")
	if len(d.diagnostics) != 0 {
		t.Fatalf("ожидалось, что диагностика снята после исправления, получено %+v", d.diagnostics)
	}
}

// TestUpdateKeepsGoodDeclarationsAfterBrokenOne - главная находка ROADMAP
// ("LSP не должен ГЛОХНУТЬ целиком из-за одной ошибки в файле"): опечатка в
// одной функции файла не должна лишать hover/definition/completion доступа
// к ОСТАЛЬНЫМ, синтаксически исправным объявлениям того же файла.
func TestUpdateKeepsGoodDeclarationsAfterBrokenOne(t *testing.T) {
	d := newDoc("test://mixed.rech", `
функция хорошая
{
    вывести "1"
}

функция сломанная(
{
    вывести "не разберётся"
}
`)
	if len(d.diagnostics) == 0 {
		t.Fatalf("ожидалась диагностика на сломанную функцию")
	}
	if d.astProg == nil {
		t.Fatalf("ожидался частичный ast.Program, получено nil")
	}
	if _, ok := d.astFuncs["хорошая"]; !ok {
		t.Fatalf("ожидалось, что \"хорошая\" останется разобранной несмотря на соседнюю ошибку, astFuncs=%+v", d.astFuncs)
	}
}
