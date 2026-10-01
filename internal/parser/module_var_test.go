package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

func parseProgramSrc(t *testing.T, src string) (*ast.Program, error) {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	return New(toks).ParseProgram()
}

// TestModuleVarsBothFormsAndVisibility - "переменная имя = ..." и "<Тип>
// имя = ..." на верхнем уровне файла с явной "главная" - переменные
// модуля; модификатор перед "переменная" согласуется со словом
// "переменная", перед "<Тип> <имя>" - с именем переменной.
func TestModuleVarsBothFormsAndVisibility(t *testing.T) {
	prog, err := parseProgramSrc(t, `переменная счётчик = 0
приватная переменная имя = "а"
публичное Целое число = 5
публичный Целое предел = 1
приватная Строка строка = ""

функция главная
{
    счётчик++
}
`)
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	want := map[string]ast.Visibility{"счётчик": ast.Public, "имя": ast.Private, "число": ast.Public, "предел": ast.Public, "строка": ast.Private}
	if len(prog.Vars) != len(want) {
		t.Fatalf("ожидалось %d переменных модуля, получено %d", len(want), len(prog.Vars))
	}
	for _, v := range prog.Vars {
		if vis, ok := want[v.Name]; !ok || vis != v.Visibility {
			t.Fatalf("переменная %q: видимость %v", v.Name, v.Visibility)
		}
	}
	if len(prog.Functions) != 1 || len(prog.Functions[0].Body) != 1 {
		t.Fatalf("переменные модуля не должны попадать в тело главная")
	}
}

// TestModuleVarsInScriptStayStatements - в файле-скрипте переменные
// верхнего уровня остаются инструкциями неявной "главная" по порядку.
func TestModuleVarsInScriptStayStatements(t *testing.T) {
	prog, err := parseProgramSrc(t, "переменная х = 5\nх = х + 1\nвывести х\n")
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if len(prog.Vars) != 0 || len(prog.Functions[0].Body) != 3 {
		t.Fatalf("в скрипте переменная должна остаться инструкцией: vars=%d, тело=%d", len(prog.Vars), len(prog.Functions[0].Body))
	}
}

func TestModuleVarsErrors(t *testing.T) {
	cases := map[string]string{
		"публичный Целое число = 5\nфункция главная\n{\n}\n":  "не согласован",
		"публичный переменная х = 1\nфункция главная\n{\n}\n": "согласуется",
		"Целое число\nфункция главная\n{\n}\n":                "нужно начальное значение",
		"публичная переменная х = 1\nвывести х\n":             "файле-скрипте",
	}
	for src, want := range cases {
		_, err := parseProgramSrc(t, src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: ожидалась ошибка с %q, получено %v", src, want, err)
		}
	}
}

// TestBareVarDeclParsed - "Тип имя" и "переменная имя: Тип" без значения
// внутри функции - объявление без значения; строчное слово в начале
// строки ("напечатать отчёт") - не объявление.
func TestBareVarDeclParsed(t *testing.T) {
	prog, err := parseProgramSrc(t, `функция напечатать(отчёт: Строка)
{
}

функция главная
{
    Целое число
    ?Строка имя
    Список<Целое> числа
    переменная другое: Целое
    Строка отчёт = ""
    напечатать отчёт
}
`)
	if err != nil {
		t.Fatalf("разбор: %v", err)
	}
	body := prog.Functions[1].Body
	for i, name := range []string{"число", "имя", "числа", "другое"} {
		d, ok := body[i].(*ast.VarDecl)
		if !ok || d.Name != name || d.Value != nil || d.Type == "" {
			t.Fatalf("инструкция %d: ожидалось объявление %q без значения, получено %#v", i, name, body[i])
		}
	}
	if _, ok := body[5].(*ast.VarDecl); ok {
		t.Fatalf("\"напечатать отчёт\" - естественный вызов, не объявление")
	}
}
