package lsp

import (
	"strings"
	"testing"
)

const brokenSemanticSrc = `
/** Точка на плоскости. */
сущность Точка
{
    имеет:
        x: Целое

    /** Сдвигает точку. */
    сдвинуть(на: Целое)
    {
        этот.x = "не число"
    }

    /** Координата. */
    координата(): Целое
    {
        вернуть этот.x
    }
}

/** Складывает. */
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}

функция главная
{
    переменная т: Целое = "строка"
    вывести сложить(1, 2)
}
`

// hoverAt - hover на первом вхождении word в строке, содержащей lineText.
func hoverAt(t *testing.T, d *doc, src, lineText, word string) (string, bool) {
	t.Helper()
	line := lineOf(t, src, lineText)
	return d.hover(line, runeCol(t, strings.Split(src, "\n")[line-1], word))
}

func TestHoverSurvivesSemanticErrorOnFirstOpen(t *testing.T) {
	d := newDoc("test://a.rech", brokenSemanticSrc)
	if len(d.diagnostics) == 0 {
		t.Fatalf("ожидалась диагностика семантической ошибки")
	}
	text, ok := hoverAt(t, d, brokenSemanticSrc, "функция сложить", "сложить")
	if !ok || !strings.Contains(text, "Складывает.") {
		t.Fatalf("hover на исправной функции при ошибке в другой: %q, %v", text, ok)
	}
	text, ok = hoverAt(t, d, brokenSemanticSrc, "сущность Точка", "Точка")
	if !ok || !strings.Contains(text, "Точка на плоскости.") {
		t.Fatalf("hover на сущности с ошибкой в методе: %q, %v", text, ok)
	}
	text, ok = hoverAt(t, d, brokenSemanticSrc, "координата(): Целое", "координата")
	if !ok || !strings.Contains(text, "Координата.") {
		t.Fatalf("hover на исправном методе: %q, %v", text, ok)
	}
	// Метод с ошибкой в теле - сигнатура и параметры остаются видны.
	text, ok = hoverAt(t, d, brokenSemanticSrc, "сдвинуть(на: Целое)", "сдвинуть")
	if !ok || !strings.Contains(text, "Сдвигает точку.") {
		t.Fatalf("hover на методе с ошибкой в теле: %q, %v", text, ok)
	}
	text, ok = hoverAt(t, d, brokenSemanticSrc, "вывести сложить(1, 2)", "сложить")
	if !ok || !strings.Contains(text, "сложить(а: Целое, б: Целое): Целое") {
		t.Fatalf("hover на вызове внутри функции с ошибкой: %q, %v", text, ok)
	}
}

func TestHoverSurvivesSemanticErrorAfterEdit(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	// Лишняя строка сверху сдвигает все объявления - hover должен
	// работать по текущим строкам, а не по последнему удачному тексту.
	broken := "\n" + strings.Replace(hoverTestSrc, "вывести т.x", "вывести т.нет_такого", 1)
	d.update(broken)
	if len(d.diagnostics) == 0 {
		t.Fatalf("ожидалась диагностика")
	}
	text, ok := hoverAt(t, d, broken, "функция сложить", "сложить")
	if !ok || !strings.Contains(text, "Складывает два числа.") {
		t.Fatalf("hover после правки с ошибкой: %q, %v", text, ok)
	}
}

func TestHoverSurvivesLexerError(t *testing.T) {
	src := "\n/** Складывает. */\nфункция сложить(а: Целое, б: Целое): Целое\n{\n    вернуть а + б\n}\n\nфункция главная\n{\n    вывести `незакрытая\n    вывести сложить(1, 2)\n}\n"
	d := newDoc("test://a.rech", src)
	if len(d.diagnostics) != 1 || !strings.Contains(d.diagnostics[0].Message, "не закрыта") {
		t.Fatalf("ожидалась ровно одна диагностика лексера, получено %v", d.diagnostics)
	}
	text, ok := hoverAt(t, d, src, "функция сложить", "сложить")
	if !ok || !strings.Contains(text, "Складывает.") {
		t.Fatalf("hover при ошибке лексера: %q, %v", text, ok)
	}
}

func TestAnalyzeIsRepeatableOnSameAST(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	for _, iface := range d.astProg.Interfaces {
		if iface.Name == "Строчный" {
			t.Fatalf("semantic.Analyze не должен дописывать builtin-интерфейс в AST вызывающего")
		}
	}
}

const brokenSignatureSrc = `сущность Точка
{
    имеет:
        x: НетТакогоТипа
        y: Целое
}

/** Складывает. */
функция сложить(а: Целое, б: НетТипа): Целое
{
    вернуть а
}

/** Другая. */
функция другая(а: Целое): Целое
{
    вернуть а
}

функция главная
{
    Точка т = новая Точка()
    вывести другая(1)
}
`

// TestSalvageDeclarationErrorsKeepRestOfFile - ошибки в типе поля и в
// сигнатуре показываются на своих строках, обе сразу (семантика сама
// останавливается после первой фазы), без каскадной ошибки от убранной
// сущности; hover в остальном файле работает.
func TestSalvageDeclarationErrorsKeepRestOfFile(t *testing.T) {
	d := newDoc("test://a.rech", brokenSignatureSrc)
	lines := map[int]string{}
	for _, dg := range d.diagnostics {
		lines[dg.Line] = dg.Message
	}
	if !strings.Contains(lines[4], `поле "x"`) || !strings.Contains(lines[9], `параметр "б"`) || len(lines) != 2 {
		t.Fatalf("ожидались ошибки поля (строка 4) и параметра (строка 9), получено %v", d.diagnostics)
	}
	text, ok := hoverAt(t, d, brokenSignatureSrc, "вывести другая(1)", "другая")
	if !ok || !strings.Contains(text, "Другая.") {
		t.Fatalf("hover на исправной функции: %q, %v", text, ok)
	}
}

const brokenBodyWithResultSrc = `/** Точка. */
сущность Точка
{
    имеет:
        x: НетТипа
}

/** Считает. */
функция посчитать(а: Целое): Целое
{
    переменная двойное = а * 2
    переменная плохо: Целое = "строка"
    Точка т = новая Точка()
    вернуть двойное
}

функция главная
{
    вывести посчитать(1)
}
`

// TestSalvageKeepsFunctionWithResultAndItsLocals - у функции с типом
// результата ошибка в теле убирает только сломанные инструкции: сама
// функция и её исправные локальные переменные остаются видны hover'у
// (раньше выкинутое тело само становилось ошибкой "нет вернуть" и функция
// пропадала целиком).
func TestSalvageKeepsFunctionWithResultAndItsLocals(t *testing.T) {
	d := newDoc("test://a.rech", brokenBodyWithResultSrc)
	text, ok := hoverAt(t, d, brokenBodyWithResultSrc, "вывести посчитать(1)", "посчитать")
	if !ok || !strings.Contains(text, "Считает.") {
		t.Fatalf("hover на функции со сломанным телом: %q, %v", text, ok)
	}
	text, ok = hoverAt(t, d, brokenBodyWithResultSrc, "вернуть двойное", "двойное")
	if !ok || !strings.Contains(text, "Целое") {
		t.Fatalf("hover на исправной локальной переменной: %q, %v", text, ok)
	}
}

// TestHoverSurvivesSyntaxErrorInBody - синтаксическая ошибка в инструкции
// не выкидывает функцию целиком: остальные инструкции тела разобраны.
func TestHoverSurvivesSyntaxErrorInBody(t *testing.T) {
	src := `/** Складывает. */
функция сложить(а: Целое, б: Целое): Целое
{
    переменная ц = а +
    вернуть а + б
}

функция главная
{
    переменная х = 5
    вывести сложить(х, 2)
    если х > {
    }
    вывести х
}
`
	d := newDoc("test://a.rech", src)
	if len(d.diagnostics) != 2 || d.diagnostics[0].Line != 5 || d.diagnostics[1].Line != 12 {
		t.Fatalf("ожидались две синтаксические ошибки (строки 5 и 12), получено %v", d.diagnostics)
	}
	text, ok := hoverAt(t, d, src, "вывести сложить(х, 2)", "сложить")
	if !ok || !strings.Contains(text, "Складывает.") {
		t.Fatalf("hover на вызове в функции с синтаксической ошибкой: %q, %v", text, ok)
	}
	text, ok = hoverAt(t, d, src, "вывести х", "х")
	if !ok || !strings.Contains(text, "Целое") {
		t.Fatalf("hover на переменной после сломанной инструкции: %q, %v", text, ok)
	}
}
