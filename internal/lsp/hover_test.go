package lsp

import (
	"strings"
	"testing"
)

const hoverTestSrc = `
/** Точка на плоскости. */
сущность Точка
{
    имеет:
        /** Горизонтальная координата. */
        x: Целое
        y: Целое

    конструктор(x: Целое, y: Целое)
    {
        этот.x = x
        этот.y = y
    }

    /** Расстояние от начала координат. */
    длина(): Целое
    {
        вернуть этот.x
    }
}

/** Складывает два числа. */
функция сложить(кого: Целое, кому: Целое): Целое
{
    переменная итог = кого + кому
    вернуть итог
}

функция главная
{
    переменная т = новая Точка(1, 2)
    вывести т.x
    вывести т.длина()
    вывести сложить(1, 2)
}
`

func TestHoverOnFunctionShowsSignatureAndDoc(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, hoverTestSrc, "функция сложить")
	col := runeCol(t, "функция сложить(кого: Целое, кому: Целое): Целое", "сложить")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на имени функции")
	}
	if !strings.Contains(text, "сложить(кого: Целое, кому: Целое): Целое") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
	if !strings.Contains(text, "Складывает два числа.") {
		t.Fatalf("doc-комментарий не попал в hover: %q", text)
	}
}

func TestHoverOnFieldAccessShowsFieldTypeAndDoc(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	// строка "вывести т.x" - найдём её через поиск по тексту.
	line := lineOf(t, hoverTestSrc, "вывести т.x")
	col := runeCol(t, "    вывести т.x", "x")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на поле x")
	}
	if !strings.Contains(text, "Точка.x: Целое") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
	if !strings.Contains(text, "Горизонтальная координата.") {
		t.Fatalf("doc-комментарий поля не попал в hover: %q", text)
	}
}

func TestHoverOnMethodAccessShowsSignature(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "т.длина()")
	col := runeCol(t, "    вывести т.длина()", "длина")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на методе длина")
	}
	if !strings.Contains(text, "Точка.длина(): Целое") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
	if !strings.Contains(text, "Расстояние от начала координат.") {
		t.Fatalf("doc-комментарий метода не попал в hover: %q", text)
	}
}

func TestHoverOnLocalVariableShowsType(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "вернуть итог")
	col := runeCol(t, "    вернуть итог", "итог")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на локальной переменной")
	}
	if !strings.Contains(text, "итог: Целое") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
}

// runeCol - позиция (1-индексная, как lexer.Token.Col - лексер считает
// РУНЫ, не байты) needle внутри line - strings.Index сам по себе даёт
// байтовое смещение, для кириллицы (2 байта на руну в UTF-8) это не то же
// самое.
func runeCol(t *testing.T, line, needle string) int {
	t.Helper()
	byteIdx := strings.Index(line, needle)
	if byteIdx < 0 {
		t.Fatalf("%q не найдено в %q", needle, line)
	}
	return len([]rune(line[:byteIdx])) + 1
}

// lineOf - строка (1-индексная, как lexer.Token.Line), на которой src
// содержит needle - хелпер для тестов, которым проще искать нужную строку
// текстом, чем пересчитывать номера строк руками (и ошибаться).
func lineOf(t *testing.T, src, needle string) int {
	t.Helper()
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	t.Fatalf("строка с %q не найдена", needle)
	return 0
}
