package lsp

import (
	"strings"
	"testing"
)

// TestHoverShowsReferenceCountForFunction - "сложить" вызывается ровно один
// раз в hoverTestSrc ("вывести сложить(1, 2)"), не считая собственного
// объявления.
func TestHoverShowsReferenceCountForFunction(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "функция сложить")
	col := runeCol(t, "функция сложить(кого: Целое, кому: Целое): Целое", "сложить")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на имени функции")
	}
	if !strings.Contains(text, "1 ссылка") {
		t.Fatalf("ожидалась одна ссылка на \"сложить\", получено: %q", text)
	}
}

// TestHoverShowsReferenceCountForMethod - "длина" вызывается один раз
// ("т.длина()").
func TestHoverShowsReferenceCountForMethod(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "т.длина()")
	col := runeCol(t, "    вывести т.длина()", "длина")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на методе длина")
	}
	if !strings.Contains(text, "1 ссылка") {
		t.Fatalf("ожидалась одна ссылка на \"длина\", получено: %q", text)
	}
}

// TestHoverShowsReferenceCountForField - поле "x" встречается дважды:
// "т.x" в главная и "этот.x" внутри метода длина.
func TestHoverShowsReferenceCountForField(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "вывести т.x")
	col := runeCol(t, "    вывести т.x", "x")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на поле x")
	}
	if !strings.Contains(text, "2 ссылки") {
		t.Fatalf("ожидались две ссылки на \"x\", получено: %q", text)
	}
}

// TestHoverShowsReferenceCountForEntity - "Точка" создаётся один раз
// ("новая Точка(1, 2)").
func TestHoverShowsReferenceCountForEntity(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "новая Точка(1, 2)")
	col := runeCol(t, "    переменная т = новая Точка(1, 2)", "Точка")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на имени сущности")
	}
	if !strings.Contains(text, "1 ссылка") {
		t.Fatalf("ожидалась одна ссылка на \"Точка\", получено: %q", text)
	}
}

const constHoverSrc = `
сущность Точка
{
    константа ПРЕДЕЛ = 100

    имеет:
        x: Целое
}
функция главная
{
    переменная т = новая Точка(1)
    вывести т.ПРЕДЕЛ
    вывести т.ПРЕДЕЛ
}
`

// TestHoverOnConstantShowsResolvedValue - "т.ПРЕДЕЛ" - hover должен
// показать разрешённое значение константы (100), не только её тип.
func TestHoverOnConstantShowsResolvedValue(t *testing.T) {
	d := newDoc("test://b.rech", constHoverSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, constHoverSrc, "вывести т.ПРЕДЕЛ")
	col := runeCol(t, "    вывести т.ПРЕДЕЛ", "ПРЕДЕЛ")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на константе ПРЕДЕЛ")
	}
	if !strings.Contains(text, "Значение: `100`") {
		t.Fatalf("ожидалось значение константы в hover, получено: %q", text)
	}
	if !strings.Contains(text, "2 ссылки") {
		t.Fatalf("ожидались две ссылки на \"ПРЕДЕЛ\", получено: %q", text)
	}
}
