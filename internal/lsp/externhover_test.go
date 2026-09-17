package lsp

import (
	"strings"
	"testing"
)

func TestHoverExternFuncShowsRechDeclaredTypesAndAlias(t *testing.T) {
	src := `
/** Возвращает длину строки в байтах. */
внешняя функция strlen как длина_байт(текст: Строка): Целое

функция главная
{
    переменная н = длина_байт("привет")
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "переменная н = длина_байт")
	col := runeCol(t, "    переменная н = длина_байт(\"привет\")", "длина_байт")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на псевдониме внешней функции")
	}
	if !strings.Contains(text, "длина_байт(текст: Строка): Целое") {
		t.Fatalf("сигнатура должна использовать РЕЧ-типы, получено %q", text)
	}
	if !strings.Contains(text, "C-имя: `strlen`") {
		t.Fatalf("ожидалось настоящее C-имя, получено %q", text)
	}
	if !strings.Contains(text, "Возвращает длину строки в байтах.") {
		t.Fatalf("ожидался doc-комментарий, получено %q", text)
	}
}

func TestHoverExternFuncWithoutAliasUsesCName(t *testing.T) {
	src := `
внешняя функция atoi(текст: Строка): Целое

функция главная
{
    переменная н = atoi("42")
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "переменная н = atoi")
	col := runeCol(t, `    переменная н = atoi("42")`, "atoi")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на внешней функции без псевдонима")
	}
	if !strings.Contains(text, "atoi(текст: Строка): Целое") {
		t.Fatalf("неожиданный текст: %q", text)
	}
	if strings.Contains(text, "C-имя:") {
		t.Fatalf("без псевдонима C-имя совпадает с именем - строка \"C-имя\" не нужна: %q", text)
	}
}

func TestHoverExternOpaqueEntity(t *testing.T) {
	src := `
внешняя сущность void как Блок
внешняя функция malloc как выделить(размер: Целое): Блок

функция главная
{
    переменная б = выделить(16)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "внешняя сущность void как Блок")
	col := runeCol(t, "внешняя сущность void как Блок", "Блок")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на псевдониме внешней сущности")
	}
	if !strings.Contains(text, "внешняя сущность Блок") {
		t.Fatalf("неожиданный текст: %q", text)
	}
	if !strings.Contains(text, "C-имя: `void`") {
		t.Fatalf("ожидалось настоящее C-имя, получено %q", text)
	}
}

func TestHoverExternFuncWithoutDocShowsOnlySignature(t *testing.T) {
	src := `
внешняя функция rand(): Целое

функция главная
{
    переменная н = rand()
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "переменная н = rand")
	col := runeCol(t, "    переменная н = rand()", "rand")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover")
	}
	if text != "```rech\nвнешняя функция rand(): Целое\n```" {
		t.Fatalf("неожиданный текст без doc-комментария: %q", text)
	}
}
