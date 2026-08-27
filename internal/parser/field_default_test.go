package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseFieldDefaultValue(t *testing.T) {
	src := `
сущность Пользователь
{
    имеет:
        имя: Строка = "Илья"
        возраст: Целое
}
функция главная { вывести "ок" }
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	lit, ok := ent.Fields[0].Default.(*ast.StringLiteral)
	if !ok || lit.Value != "Илья" {
		t.Fatalf("неожиданное значение по умолчанию поля имя: %#v", ent.Fields[0].Default)
	}
	if ent.Fields[1].Default != nil {
		t.Fatalf("у поля возраст без значения по умолчанию Default должен быть nil, получено %#v", ent.Fields[1].Default)
	}
}
