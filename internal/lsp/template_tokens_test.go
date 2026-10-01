package lsp

import (
	"strings"
	"testing"
)

const templateHoverSrc = "/** Складывает. */\nфункция сложить(а: Целое, б: Целое): Целое\n{\n    вернуть а + б\n}\n\nфункция главная\n{\n    переменная х = 5\n    вывести `сумма ${сложить(х, 2)}, вложенно ${`${х}`}\\n`\n}\n"

// TestHoverInsideTemplateInsertion - hover и переход к определению видят
// имена внутри "${...}", в том числе во вложенном шаблоне.
func TestHoverInsideTemplateInsertion(t *testing.T) {
	d := newDoc("test://a.rech", templateHoverSrc)
	line := lineOf(t, templateHoverSrc, "вывести `")
	lineText := strings.Split(templateHoverSrc, "\n")[line-1]

	text, ok := d.hover(line, runeCol(t, lineText, "сложить"))
	if !ok || !strings.Contains(text, "Складывает.") {
		t.Fatalf("hover на функции во вставке: %q, %v", text, ok)
	}
	nested := strings.LastIndex(lineText, "${х}")
	text, ok = d.hover(line, len([]rune(lineText[:nested]))+3)
	if !ok || !strings.Contains(text, "Целое") {
		t.Fatalf("hover на переменной во вложенном шаблоне: %q, %v", text, ok)
	}
	if loc, ok := d.definition(line, runeCol(t, lineText, "сложить")); !ok || loc.Line != 2 {
		t.Fatalf("переход к определению из вставки: %+v, %v", loc, ok)
	}
}
