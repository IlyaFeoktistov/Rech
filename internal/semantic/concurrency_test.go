package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeLaunchAwaitRoundTripsReturnType(t *testing.T) {
	prog := parseProgram(t, `
функция посчитать(): Целое
{
    вернуть 42
}
функция главная
{
    переменная задача = запустить посчитать()
    переменная результат = дождаться задача
    вывести результат
}
`)
	fn := findFunc(prog, "главная")
	decl, ok := fn.Body[0].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[0])
	}
	launch, ok := decl.Value.(*ir.LaunchExpr)
	if !ok {
		t.Fatalf("ожидался LaunchExpr, получено %T", decl.Value)
	}
	if launch.Callee != "посчитать" {
		t.Fatalf("ожидался callee \"посчитать\", получено %q", launch.Callee)
	}
	if typ := ir.TypeOf(launch); typ != ir.TaskType("посчитать") {
		t.Fatalf("ожидался тип Задача<посчитать>, получено %s", typ)
	}
	result, ok := fn.Body[1].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[1])
	}
	await, ok := result.Value.(*ir.AwaitExpr)
	if !ok {
		t.Fatalf("ожидался AwaitExpr, получено %T", result.Value)
	}
	if await.Type != ir.TypeInt || !await.HasReturn {
		t.Fatalf("ожидался тип Целое с результатом, получено %s (HasReturn=%v)", await.Type, await.HasReturn)
	}
}

func TestAnalyzeAwaitWithoutReturnUsableOnlyAsStatement(t *testing.T) {
	prog := parseProgram(t, `
функция вывести_привет()
{
    вывести "привет"
}
функция главная
{
    переменная задача = запустить вывести_привет()
    дождаться задача
}
`)
	fn := findFunc(prog, "главная")
	stmt, ok := fn.Body[1].(*ir.ExprStmt)
	if !ok || stmt.Await == nil {
		t.Fatalf("ожидался ExprStmt{Await}, получено %T", fn.Body[1])
	}

	_, err := AnalyzeSource(t, `
функция вывести_привет()
{
    вывести "привет"
}
функция главная
{
    переменная задача = запустить вывести_привет()
    переменная х = дождаться задача
}
`)
	if err == nil || !strings.Contains(err.Error(), "нельзя использовать как выражение") {
		t.Fatalf("ожидалась ошибка - дождаться без результата нельзя как значение, получено: %v", err)
	}
}

func TestAnalyzeLaunchRejectsBuiltinFunction(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная задача = запустить корень(4.0)
}
`)
	if err == nil || !strings.Contains(err.Error(), "встроенные функции") {
		t.Fatalf("ожидалась ошибка о встроенных функциях, получено: %v", err)
	}
}

func TestAnalyzeAwaitRejectsNonTaskValue(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная х = 5
    переменная y = дождаться х
}
`)
	if err == nil || !strings.Contains(err.Error(), "ожидает значение задачи") {
		t.Fatalf("ожидалась ошибка о том, что дождаться ожидает задачу, получено: %v", err)
	}
}

func TestAnalyzeLaunchArgTypeMismatchFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    переменная задача = запустить сложить("не число", 2)
}
`)
	if err == nil || !strings.Contains(err.Error(), "должен быть типа") {
		t.Fatalf("ожидалась ошибка типа аргумента, получено: %v", err)
	}
}
