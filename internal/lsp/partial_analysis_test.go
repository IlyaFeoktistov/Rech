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
