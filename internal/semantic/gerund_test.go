package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeGerundStmtBuildsResultVarDeclAndBody(t *testing.T) {
	irProg := parseProgram(t, `
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
	var main *ir.Function
	for _, fn := range irProg.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	if main == nil || len(main.Body) != 1 {
		t.Fatalf("ожидалась одна инструкция в \"главная\", получено %+v", main)
	}
	g, ok := main.Body[0].(*ir.GerundStmt)
	if !ok {
		t.Fatalf("ожидался ir.GerundStmt, получено %T", main.Body[0])
	}
	if len(g.Body) != 2 {
		t.Fatalf("ожидались 2 инструкции внутри GerundStmt.Body (результат + вывести), получено %+v", g.Body)
	}
	decl, ok := g.Body[0].(*ir.VarDecl)
	if !ok || decl.Name != "результат" {
		t.Fatalf("ожидался VarDecl \"результат\" первым элементом Body, получено %+v", g.Body[0])
	}
	call, ok := decl.Value.(*ir.CallExpr)
	if !ok || call.Callee != "сложить" || !call.HasReturn {
		t.Fatalf("ожидался вызов \"сложить\" в качестве значения \"результат\", получено %+v", decl.Value)
	}
}

func TestAnalyzeGerundStmtRequiresFunctionWithReturnType(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сделать(а: Целое)
{
    вывести а
}
функция главная
{
    сделав(1)
        вывести "ок"
}
`)
	if err == nil || !strings.Contains(err.Error(), "объявленным типом результата") {
		t.Fatalf("ожидалась ошибка - деепричастие требует функцию с результатом, получено: %v", err)
	}
}

func TestAnalyzeGerundStmtArgTypeMismatchFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    сложив("не число", 3)
        вывести результат
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка типа аргумента")
	}
}

func TestAnalyzeGerundStmtResultNotVisibleOutsideBlock(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    сложив(2, 3)
        вывести результат
    вывести результат
}
`)
	if err == nil || !strings.Contains(err.Error(), "не объявлена") {
		t.Fatalf("ожидалась ошибка - \"результат\" не видна снаружи блока деепричастия, получено: %v", err)
	}
}

func TestAnalyzeNestedGerundStmtShadowsOuterResult(t *testing.T) {
	irProg := parseProgram(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция умножить(а: Целое, б: Целое): Целое
{
    вернуть а * б
}
функция главная
{
    сложив(2, 3)
        умножив(результат, 10)
            вывести результат
        вывести результат
}
`)
	var main *ir.Function
	for _, fn := range irProg.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	outer := main.Body[0].(*ir.GerundStmt)
	// outer.Body[0] - VarDecl "результат" (сложить), outer.Body[1] -
	// вложенный GerundStmt (умножив), outer.Body[2] - "вывести результат"
	// снаружи вложенного, но всё ещё внутри внешнего блока.
	if len(outer.Body) != 3 {
		t.Fatalf("ожидались 3 инструкции во внешнем блоке, получено %+v", outer.Body)
	}
	inner, ok := outer.Body[1].(*ir.GerundStmt)
	if !ok {
		t.Fatalf("ожидался вложенный GerundStmt, получено %T", outer.Body[1])
	}
	innerDecl := inner.Body[0].(*ir.VarDecl)
	innerCall := innerDecl.Value.(*ir.CallExpr)
	if innerCall.Callee != "умножить" {
		t.Fatalf("ожидался вызов \"умножить\" внутри вложенного блока, получено %+v", innerCall)
	}
	arg, ok := innerCall.Args[0].(*ir.Identifier)
	if !ok || arg.Name != "результат" {
		t.Fatalf("ожидалось, что первый аргумент вложенного вызова читает внешний \"результат\", получено %+v", innerCall.Args[0])
	}
}
