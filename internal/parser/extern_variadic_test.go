package parser

import (
	"strings"
	"testing"
)

func TestParseExternVariadic(t *testing.T) {
	prog, err := parseSource(t, "внешняя функция printf(формат: Строка, ...): Целое32\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ext := prog.ExternFuncs[0]
	if !ext.Variadic || len(ext.Params) != 1 {
		t.Fatalf("ожидался вариативный printf с одним параметром, получено Variadic=%v, %d параметров", ext.Variadic, len(ext.Params))
	}
	_, err = parseSource(t, "функция ф(x: Целое, ...)\n{\n}\n")
	if err == nil || !strings.Contains(err.Error(), "только у внешней функции") {
		t.Fatalf("ожидалась ошибка про \"...\" без имени, получено %v", err)
	}
}
