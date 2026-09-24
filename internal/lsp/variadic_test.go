package lsp

import (
	"strings"
	"testing"
)

const variadicHoverSrc = `/** Сумма чисел. */
функция сумма(база: Целое, ...числа: Целое): Целое
{
    вернуть база
}

функция главная
{
    сумма(1, 2, 3)
}
`

func TestHoverShowsVariadicParamAsDeclared(t *testing.T) {
	d := newDoc("test://a.rech", variadicHoverSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, variadicHoverSrc, "функция сумма")
	col := runeCol(t, "функция сумма(база: Целое, ...числа: Целое): Целое", "сумма")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на имени функции")
	}
	if !strings.Contains(text, "сумма(база: Целое, ...числа: Целое): Целое") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
}
