package lsp

import (
	"errors"
	"strings"
	"testing"
)

func TestErrToDiagnosticExtractsLineAndColumn(t *testing.T) {
	err := errors.New(`парсер: строка 3, столбец 5: неожиданный токен "}"`)
	d := errToDiagnostic(err, nil)
	if d.Line != 3 || d.Col != 5 {
		t.Fatalf("ожидалось line=3, col=5, получено %+v", d)
	}
	if !strings.Contains(d.Message, "неожиданный токен") {
		t.Fatalf("ожидалось сообщение про неожиданный токен, получено %q", d.Message)
	}
}

func TestErrToDiagnosticWithoutColumnDefaultsToOne(t *testing.T) {
	err := errors.New(`лексер: строка 7: недопустимый символ`)
	d := errToDiagnostic(err, nil)
	if d.Line != 7 || d.Col != 1 {
		t.Fatalf("ожидалось line=7, col=1, получено %+v", d)
	}
}

func TestErrToDiagnosticWithoutLineFallsBackToLineOne(t *testing.T) {
	err := errors.New("исходный файл не в кодировке UTF-8")
	d := errToDiagnostic(err, nil)
	if d.Line != 1 || d.Col != 1 {
		t.Fatalf("ожидалось line=1, col=1 (нет привязки к строке), получено %+v", d)
	}
	if d.Message != err.Error() {
		t.Fatalf("ожидалось сообщение целиком, получено %q", d.Message)
	}
}

// TestErrToDiagnosticSharpensRangeToQuotedIdentifier - самый частый случай
// (fmt.Errorf с "%q", "переменная %q не объявлена" и т. п.) - без столбца
// в сообщении диагностика раньше садилась на column=1/len=1 (невидимый
// символ в начале строки, не на само ошибочное слово - репортнуто
// пользователем) - теперь ищет цитату как целое слово в тексте строки.
func TestErrToDiagnosticSharpensRangeToQuotedIdentifier(t *testing.T) {
	err := errors.New(`строка 2: переменная "счётчик" не объявлена`)
	lines := []string{"функция главная", "    вывести счётчик", "}"}
	d := errToDiagnostic(err, lines)
	wantCol := len([]rune(strings.SplitN(lines[1], "счётчик", 2)[0])) + 1
	if d.Line != 2 || d.Col != wantCol || d.Len != len([]rune("счётчик")) {
		t.Fatalf("ожидалось line=2, col=%d, len=%d, получено %+v", wantCol, len([]rune("счётчик")), d)
	}
}

// TestErrToDiagnosticWithoutColumnHighlightsWholeLine - когда компилятор не
// указал столбец И в сообщении нет подходящей цитаты (структурная ошибка
// разбора) - подсвечивается весь непустой текст строки, не один символ.
func TestErrToDiagnosticWithoutColumnHighlightsWholeLine(t *testing.T) {
	err := errors.New(`строка 2: не найдена закрывающая }`)
	lines := []string{"функция главная", "    вывести 1", "}"}
	d := errToDiagnostic(err, lines)
	wantLen := len([]rune(strings.TrimSpace(lines[1])))
	if d.Line != 2 || d.Col != 5 || d.Len != wantLen {
		t.Fatalf("ожидалось line=2, col=5, len=%d, получено %+v", wantLen, d)
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

// TestUpdateKeepsGoodDeclarationsAfterBrokenOne - "LSP не должен ГЛОХНУТЬ
// целиком из-за одной ошибки в файле": опечатка в
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
