package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestOverloadResolvesByParamCountAndType(t *testing.T) {
	irProg := parseProgram(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция сложить(а: Строка, б: Строка): Строка
{
    вернуть а + б
}
функция сложить(а: Целое, б: Целое, ц: Целое): Целое
{
    вернуть а + б + ц
}
функция главная
{
    вывести сложить(2, 3)
    вывести сложить("а", "б")
    вывести сложить(1, 2, 3)
}
`)
	var main *ir.Function
	for _, fn := range irProg.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	if main == nil || len(main.Body) != 3 {
		t.Fatalf("ожидались 3 инструкции в \"главная\", получено %+v", main)
	}
	wantCallees := []string{"сложить(Целое,Целое)", "сложить(Строка,Строка)", "сложить(Целое,Целое,Целое)"}
	for i, want := range wantCallees {
		print, ok := main.Body[i].(*ir.PrintStmt)
		if !ok {
			t.Fatalf("инструкция %d: ожидался PrintStmt, получено %T", i, main.Body[i])
		}
		call, ok := print.Value.(*ir.CallExpr)
		if !ok {
			t.Fatalf("инструкция %d: ожидался ir.CallExpr, получено %T", i, print.Value)
		}
		if call.Callee != want {
			t.Fatalf("инструкция %d: ожидался Callee %q, получено %q", i, want, call.Callee)
		}
	}
}

func TestOverloadDuplicateParamTypeListFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция сложить(а: Целое, б: Целое): Строка
{
    вернуть "х"
}
функция главная
{
    вывести сложить(1, 2)
}
`)
	if err == nil || !strings.Contains(err.Error(), "одинаковым списком типов параметров") {
		t.Fatalf("ожидалась ошибка о дублирующемся списке типов параметров, получено: %v", err)
	}
}

func TestOverloadNoMatchingCandidateFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция сложить(а: Строка, б: Строка): Строка
{
    вернуть а + б
}
функция главная
{
    вывести сложить(истина, ложь)
}
`)
	if err == nil || !strings.Contains(err.Error(), "нет подходящей перегрузки") {
		t.Fatalf("ожидалась ошибка \"нет подходящей перегрузки\", получено: %v", err)
	}
}

// TestOverloadAmbiguousCallFails - "Утка" реализует ОБА интерфейса, каждый
// из двух кандидатов принимает свой интерфейс - оба совместимы через
// isAssignable с одинаковым счётом (1 очко, точного совпадения ни у
// одного) - неоднозначный вызов, ни один вариант не побеждает.
func TestOverloadAmbiguousCallFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
интерфейс Летающий
{
    летать(): Строка
}
интерфейс Плавающий
{
    плавать(): Строка
}
сущность Утка реализует Летающий, Плавающий
{
    летать(): Строка
    {
        вернуть "летит"
    }
    плавать(): Строка
    {
        вернуть "плывёт"
    }
}
функция действие(х: Летающий): Строка
{
    вернуть "летающее"
}
функция действие(х: Плавающий): Строка
{
    вернуть "плавающее"
}
функция главная
{
    вывести действие(новая Утка())
}
`)
	if err == nil || !strings.Contains(err.Error(), "неоднозначный вызов") {
		t.Fatalf("ожидалась ошибка о неоднозначном вызове, получено: %v", err)
	}
}

func TestOverloadExactMatchBeatsAssignableSubtype(t *testing.T) {
	irProg := parseProgram(t, `
сущность Животное
{
}
сущность Собака наследует Животное
{
}
функция покормить(а: Животное): Строка
{
    вернуть "животное"
}
функция покормить(а: Собака): Строка
{
    вернуть "собака"
}
функция главная
{
    вывести покормить(новая Собака())
}
`)
	var main *ir.Function
	for _, fn := range irProg.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	print := main.Body[0].(*ir.PrintStmt)
	call := print.Value.(*ir.CallExpr)
	if call.Callee != "покормить(Собака)" {
		t.Fatalf("ожидался выбор точного совпадения покормить(Собака), получено Callee %q", call.Callee)
	}
}

// TestOverloadFuncRefRejectsUntypedOverloadedName - без ожидаемого типа
// "Вызываемый(...)" перегрузку выбрать не из чего.
func TestOverloadFuncRefRejectsUntypedOverloadedName(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция сложить(а: Строка, б: Строка): Строка
{
    вернуть а + б
}
функция главная
{
    переменная ф = &сложить
}
`)
	if err == nil || !strings.Contains(err.Error(), "перегружена") {
		t.Fatalf("ожидалась ошибка - \"&\" перегруженной функции без ожидаемого типа, получено: %v", err)
	}
}

// TestOverloadFuncRefResolvesByExpectedType - "&имя" перегруженной функции
// выбирает перегрузку по объявленному типу "Вызываемый(...)".
func TestOverloadFuncRefResolvesByExpectedType(t *testing.T) {
	irProg := parseProgram(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция сложить(а: Строка, б: Строка): Строка
{
    вернуть а + б
}
функция главная
{
    переменная ф: Вызываемый(Строка, Строка): Строка = &сложить
}
`)
	var main *ir.Function
	for _, fn := range irProg.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	decl := main.Body[0].(*ir.VarDecl)
	ref := decl.Value.(*ir.FuncRefExpr)
	if ref.FuncName != "сложить(Строка,Строка)" {
		t.Fatalf("ожидалась перегрузка сложить(Строка,Строка), получено %q", ref.FuncName)
	}
}

func TestOverloadFuncRefRejectsMismatchedExpectedType(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция сложить(а: Строка, б: Строка): Строка
{
    вернуть а + б
}
функция главная
{
    переменная ф: Вызываемый(Логическое, Логическое): Логическое = &сложить
}
`)
	if err == nil || !strings.Contains(err.Error(), "ни одна перегрузка") {
		t.Fatalf("ожидалась ошибка - нет подходящей перегрузки, получено: %v", err)
	}
}

func TestOverloadLaunchAndAwaitResolveMangledName(t *testing.T) {
	irProg := parseProgram(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция сложить(а: Строка, б: Строка): Строка
{
    вернуть а + б
}
функция главная
{
    переменная задача = запустить сложить(2, 3)
    вывести дождаться задача
}
`)
	var main *ir.Function
	for _, fn := range irProg.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	decl, ok := main.Body[0].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", main.Body[0])
	}
	launch, ok := decl.Value.(*ir.LaunchExpr)
	if !ok {
		t.Fatalf("ожидался ir.LaunchExpr, получено %T", decl.Value)
	}
	if ref, ok := launch.Value.(*ir.FuncRefExpr); !ok || ref.FuncName != "сложить(Целое,Целое)" {
		t.Fatalf("ожидался запуск &сложить(Целое,Целое), получено %#v", launch.Value)
	}
}

func TestOverloadGerundResolvesMangledName(t *testing.T) {
	irProg := parseProgram(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция сложить(а: Строка, б: Строка): Строка
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
	g := main.Body[0].(*ir.GerundStmt)
	decl := g.Body[0].(*ir.VarDecl)
	call := decl.Value.(*ir.CallExpr)
	if call.Callee != "сложить(Целое,Целое)" {
		t.Fatalf("ожидался мангл-Callee сложить(Целое,Целое), получено %q", call.Callee)
	}
}
