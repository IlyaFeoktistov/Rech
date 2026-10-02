package lsp

import (
	"strings"
	"testing"
)

// TestHoverAndDefinitionOfModuleVar - hover на переменной модуля (в теле
// функции, во вставке шаблона, на объявлении) - тип и doc-комментарий;
// переход к определению - строка объявления.
func TestHoverAndDefinitionOfModuleVar(t *testing.T) {
	src := "/** Сколько раз вызвали. */\nЦелое счётчик = 0\n\nфункция главная\n{\n    счётчик++\n    вывести `${счётчик}\\n`\n}\n"
	d := newDoc("test://a.rech", src)
	if len(d.diagnostics) != 0 {
		t.Fatalf("неожиданные диагностики %v", d.diagnostics)
	}
	for _, pos := range [][2]int{{6, 5}, {7, 16}, {2, 7}} {
		text, ok := d.hover(pos[0], pos[1])
		if !ok || !strings.Contains(text, "Целое счётчик (переменная модуля)") || !strings.Contains(text, "Сколько раз вызвали.") {
			t.Fatalf("hover %v: %q, %v", pos, text, ok)
		}
		if loc, ok := d.definition(pos[0], pos[1]); !ok || loc.Line != 2 {
			t.Fatalf("definition %v: %+v, %v", pos, loc, ok)
		}
	}
}
