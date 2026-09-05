package lsp

import (
	"strings"
	"testing"
)

const moduleConstHoverTestSrc = `
/** Максимально допустимый возраст. */
константа МАКС_ВОЗРАСТ = 120

функция главная
{
    вывести МАКС_ВОЗРАСТ
}
`

func TestHoverOnModuleConstShowsValueAndDoc(t *testing.T) {
	d := newDoc("test://a.rech", moduleConstHoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, moduleConstHoverTestSrc, "константа МАКС_ВОЗРАСТ")
	col := runeCol(t, "константа МАКС_ВОЗРАСТ = 120", "МАКС_ВОЗРАСТ")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на имени модульной константы")
	}
	if !strings.Contains(text, "МАКС_ВОЗРАСТ = 120") {
		t.Fatalf("значение константы не попало в hover: %q", text)
	}
	if !strings.Contains(text, "Максимально допустимый возраст.") {
		t.Fatalf("doc-комментарий не попал в hover: %q", text)
	}
}

func TestHoverOnModuleConstUseSiteAlsoShowsDoc(t *testing.T) {
	d := newDoc("test://a.rech", moduleConstHoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, moduleConstHoverTestSrc, "вывести МАКС_ВОЗРАСТ")
	col := runeCol(t, "    вывести МАКС_ВОЗРАСТ", "МАКС_ВОЗРАСТ")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover в месте использования константы")
	}
	if !strings.Contains(text, "Максимально допустимый возраст.") {
		t.Fatalf("doc-комментарий не попал в hover в месте использования: %q", text)
	}
}
