package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseGerundStmtAutoDetectedForm(t *testing.T) {
	prog, err := parseSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    сложив(2, 3)
        вывести результат
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	var main *ast.FunctionDecl
	for _, fn := range prog.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	if main == nil || len(main.Body) != 1 {
		t.Fatalf("ожидалась одна инструкция в \"главная\", получено %+v", main)
	}
	g, ok := main.Body[0].(*ast.GerundStmt)
	if !ok {
		t.Fatalf("ожидался GerundStmt, получено %T", main.Body[0])
	}
	if g.Callee != "сложить" || len(g.Args) != 2 || len(g.Body) != 1 {
		t.Fatalf("неожиданный GerundStmt: %+v", g)
	}
}

func TestParseGerundStmtExplicitTagOverridesAutoForm(t *testing.T) {
	prog, err := parseSource(t, `
функция странная_функция(а: Целое): Целое `+"`деепричастие:чудив`"+`
{
    вернуть а
}
функция главная
{
    чудив(1)
        вывести результат
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	var main *ast.FunctionDecl
	for _, fn := range prog.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	g, ok := main.Body[0].(*ast.GerundStmt)
	if !ok || g.Callee != "странная_функция" {
		t.Fatalf("ожидался GerundStmt с Callee \"странная_функция\", получено %+v", main.Body[0])
	}
}

func TestParseGerundStmtDuplicateRegistrationFails(t *testing.T) {
	_, err := parseSource(t, `
функция первая(а: Целое): Целое `+"`деепричастие:сделав`"+`
{
    вернуть а
}
функция вторая(а: Целое): Целое `+"`деепричастие:сделав`"+`
{
    вернуть а
}
функция главная
{
    сделав(1)
        вывести результат
}
`)
	if err == nil || !strings.Contains(err.Error(), "уже зарегистрировано") {
		t.Fatalf("ожидалась ошибка о повторной регистрации деепричастия, получено: %v", err)
	}
}

func TestParseGerundStmtRequiresIndentedBody(t *testing.T) {
	_, err := parseSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    сложив(2, 3)
    вывести "без отступа"
}
`)
	if err == nil || !strings.Contains(err.Error(), "ожидался блок инструкций") {
		t.Fatalf("ожидалась ошибка - деепричастие требует блок с отступом, получено: %v", err)
	}
}

func TestParseGerundStmtBodyStopsAtOuterIndent(t *testing.T) {
	prog, err := parseSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    сложив(2, 3)
        вывести результат
    вывести "снаружи"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	var main *ast.FunctionDecl
	for _, fn := range prog.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	if len(main.Body) != 2 {
		t.Fatalf("ожидались две инструкции в \"главная\" (GerundStmt + вывести \"снаружи\"), получено %d: %+v", len(main.Body), main.Body)
	}
	g, ok := main.Body[0].(*ast.GerundStmt)
	if !ok || len(g.Body) != 1 {
		t.Fatalf("ожидался GerundStmt с одной инструкцией в теле, получено %+v", main.Body[0])
	}
	if _, ok := main.Body[1].(*ast.PrintStmt); !ok {
		t.Fatalf("ожидалась \"вывести\" вне блока деепричастия, получено %T", main.Body[1])
	}
}

func TestParseFunctionWithoutGerundTagUnaffected(t *testing.T) {
	// "главная" сама не должна случайно зарегистрироваться как
	// деепричастие ни от чего - не глагол, morph.GerundOf для неё не
	// сработает, и тега у неё нет.
	_, prog, err := parseSourceWithParser(t, `
функция главная
{
    вывести "ок"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Functions) != 1 {
		t.Fatalf("ожидалась одна функция, получено %+v", prog.Functions)
	}
}
