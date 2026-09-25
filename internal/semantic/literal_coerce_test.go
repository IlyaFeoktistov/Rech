package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeNumericConstCoercedToDeclaredType(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    Целое8 а = -128
    Байт б = 255
    Натуральное64 н = 9223372036854775807
    Целое16 ц = -300 * 2
    Дробное д = 1.5
}
`)
	body := irProg.Functions[0].Body
	want := []ir.Type{ir.TypeInt8, ir.TypeByte, ir.TypeUint64, ir.TypeInt16, ir.TypeFloat32}
	for i, typ := range want {
		cast, ok := body[i].(*ir.VarDecl).Value.(*ir.CastExpr)
		if !ok || cast.To != typ {
			t.Fatalf("инструкция %d: ожидалось приведение к %s, получено %#v", i, typ, body[i].(*ir.VarDecl).Value)
		}
	}
	if v := body[3].(*ir.VarDecl).Value.(*ir.CastExpr).Value.(*ir.IntLiteral).Value; v != -600 {
		t.Fatalf("-300 * 2: ожидалось -600, получено %d", v)
	}
}

func TestAnalyzeNumericConstCoercedInExpressions(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция возвести(х: Целое16): Целое16
{
    вернуть х * 2
}

функция главная
{
    Целое8 а = 0
    а = 5
    а += 1
    ?Целое8 м = Ничто
    Логическое л = а > 3
    Целое16 р = возвести(7)
    вывести `+"`"+`${а + 1} ${м ?? 0} ${л} ${р}\n`+"`"+`
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNumericConstOutOfRange(t *testing.T) {
	cases := map[string]string{
		"Целое8 а = 128":       "значение 128 не помещается в тип Целое8 (от -128 до 127)",
		"Байт а = -1":          "значение -1 не помещается в тип Байт (от 0 до 255)",
		"Натуральное8 а = 256": "значение 256 не помещается в тип Натуральное8 (от 0 до 255)",
		"Натуральное а = -1":   "значение -1 не помещается в тип Натуральное (от 0 до 18446744073709551615)",
	}
	for decl, want := range cases {
		_, err := AnalyzeSource(t, "функция главная\n{\n    "+decl+"\n}\n")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: ожидалась ошибка %q, получено: %v", decl, want, err)
		}
	}
}

func TestAnalyzeNumericVariableNotCoerced(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    Целое ц = 5
    Целое8 а = ц
}
`)
	if err == nil || !strings.Contains(err.Error(), "объявлена типом Целое8") {
		t.Fatalf("переменная Целое не должна неявно приводиться к Целое8, получено: %v", err)
	}
}
