package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestModuleVarsAnalyzed - переменные модуля в порядке объявления, тип -
// из аннотации или из значения; значение видит объявленные выше; в лямбде
// переменная модуля не захватывается.
func TestModuleVarsAnalyzed(t *testing.T) {
	irProg := parseProgram(t, `Целое счётчик = 0
переменная префикс = "№"
Целое удвоенное = счётчик * 2

функция главная
{
    Вызываемый() ф = функция() {
        счётчик++
    }
    ф()
    вывести префикс
}
`)
	want := []struct {
		name string
		typ  ir.Type
	}{{"счётчик", ir.TypeInt}, {"префикс", ir.TypeString}, {"удвоенное", ir.TypeInt}}
	if len(irProg.Globals) != len(want) {
		t.Fatalf("ожидалось %d переменных модуля, получено %d", len(want), len(irProg.Globals))
	}
	for i, w := range want {
		if g := irProg.Globals[i]; g.Name != w.name || g.Type != w.typ {
			t.Fatalf("переменная %d: %s %v, ожидалось %s %v", i, g.Name, g.Type, w.name, w.typ)
		}
	}
	lambda := irProg.Functions[0].Body[0].(*ir.VarDecl).Value.(*ir.LambdaExpr)
	if len(lambda.Captures) != 0 {
		t.Fatalf("переменная модуля не должна захватываться лямбдой: %v", lambda.Captures)
	}
}

func TestModuleVarsErrors(t *testing.T) {
	cases := map[string]string{
		"Целое а = б + 1\nЦелое б = 1\nфункция главная\n{\n}\n":   "объявлена ниже",
		"Целое а = 1\nЦелое а = 2\nфункция главная\n{\n}\n":       "более одного раза",
		"функция а()\n{\n}\nЦелое а = 1\nфункция главная\n{\n}\n": "совпадает с уже объявленной функцией",
		"Целое а = 1\nфункция главная\n{\n    а = \"x\"\n}\n":     "нельзя присвоить",
	}
	for src, want := range cases {
		_, err := AnalyzeSource(t, src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: ожидалась ошибка с %q, получено %v", src, want, err)
		}
	}
}

// TestModuleVarShadowedByLocalAndParam - локальная переменная и параметр с
// тем же именем перекрывают переменную модуля.
func TestModuleVarShadowedByLocalAndParam(t *testing.T) {
	_, err := AnalyzeSource(t, `Целое а = 1

функция ф(а: Строка)
{
    вывести а + "!"
}

функция главная
{
    Строка а = "x"
    вывести а + "!"
    ф(а)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
