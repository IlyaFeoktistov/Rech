package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseParamDefaultValue(t *testing.T) {
	src := `
функция приветствовать(имя: Строка, текст: Строка = "привет") { вывести текст }
функция главная { вывести "ок" }
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if fn.Params[0].Default != nil {
		t.Fatalf("у параметра имя без значения по умолчанию Default должен быть nil, получено %#v", fn.Params[0].Default)
	}
	lit, ok := fn.Params[1].Default.(*ast.StringLiteral)
	if !ok || lit.Value != "привет" {
		t.Fatalf("неожиданное значение по умолчанию параметра текст: %#v", fn.Params[1].Default)
	}
}

func TestParseParamDefaultTrailingOrderRejected(t *testing.T) {
	src := `
функция х(а: Целое = 1, б: Целое) { вернуть а + б }
функция главная { вывести "ок" }
`
	_, err := parseSource(t, src)
	if err == nil || !strings.Contains(err.Error(), "значения по умолчанию допустимы только хвостом") {
		t.Fatalf("ожидалась ошибка про порядок значений по умолчанию, получено: %v", err)
	}
}
