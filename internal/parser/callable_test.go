package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseCallableTypeName(t *testing.T) {
	prog, err := parseSource(t, `
функция обработать(колбэк: Вызываемый(Целое, Строка): Целое): Целое
{
    вернуть 0
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	want := "Вызываемый<Целое, Строка>Целое"
	if fn.Params[0].Type != want {
		t.Fatalf("ожидался тип параметра %q, получено %q", want, fn.Params[0].Type)
	}
}

func TestParseCallableTypeNameNoParamsNoResult(t *testing.T) {
	prog, err := parseSource(t, `
функция обработать(колбэк: Вызываемый()): Целое
{
    вернуть 0
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got := prog.Functions[0].Params[0].Type; got != "Вызываемый<>" {
		t.Fatalf("ожидался тип параметра \"Вызываемый<>\", получено %q", got)
	}
}

// TestParseCallableTypeNameGenericHeader - "<T>" перед скобками (параметр
// типа самого значения "Вызываемый", package doc parseCallableTypeName) -
// имя T годится внутри скобок/типа результата, потому что здесь оно уже
// совпадает с собственным параметром типа объемлющей generic-функции.
func TestParseCallableTypeNameGenericHeader(t *testing.T) {
	prog, err := parseSource(t, `
функция применить<T>(х: T, ф: Вызываемый<T>(T): T): T
{
    вернуть ф(х)
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := "Вызываемый<T>T"
	if got := prog.Functions[0].Params[1].Type; got != want {
		t.Fatalf("ожидался тип параметра %q, получено %q", want, got)
	}
}

func TestParseFuncRefExpr(t *testing.T) {
	prog, err := parseSource(t, `
функция удвоить(x: Целое): Целое { вернуть x * 2 }
функция главная
{
    переменная ф = &удвоить
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	body := prog.Functions[1].Body
	decl, ok := body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", body[0])
	}
	ref, ok := decl.Value.(*ast.FuncRefExpr)
	if !ok {
		t.Fatalf("ожидался FuncRefExpr, получено %T", decl.Value)
	}
	if ref.Name != "удвоить" {
		t.Fatalf("ожидалось имя \"удвоить\", получено %q", ref.Name)
	}
}

func TestParseLambdaExpr(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная ф = функция(x: Целое): Целое
    {
        вернуть x + 1
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl, ok := prog.Functions[0].Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", prog.Functions[0].Body[0])
	}
	lambda, ok := decl.Value.(*ast.LambdaExpr)
	if !ok {
		t.Fatalf("ожидался LambdaExpr, получено %T", decl.Value)
	}
	if len(lambda.Params) != 1 || lambda.Params[0].Name != "x" {
		t.Fatalf("ожидался один параметр x, получено %+v", lambda.Params)
	}
	if lambda.ReturnType != "Целое" {
		t.Fatalf("ожидался тип результата Целое, получено %q", lambda.ReturnType)
	}
}

// TestParseLambdaExplicitCaptureList - "[x, &y]" сразу после типа
// результата, перед телом (см. package doc ast.LambdaExpr.Captures) -
// голое имя (ByRef false)/"&имя" (ByRef true).
func TestParseLambdaExplicitCaptureList(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная ф = функция(): Пусто [x, &y]
    {
        вывести x
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl, ok := prog.Functions[0].Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", prog.Functions[0].Body[0])
	}
	lambda, ok := decl.Value.(*ast.LambdaExpr)
	if !ok {
		t.Fatalf("ожидался LambdaExpr, получено %T", decl.Value)
	}
	if !lambda.HasCaptures {
		t.Fatal("ожидался HasCaptures == true")
	}
	if len(lambda.Captures) != 2 {
		t.Fatalf("ожидалось 2 элемента списка захвата, получено %d", len(lambda.Captures))
	}
	if lambda.Captures[0].Name != "x" || lambda.Captures[0].ByRef {
		t.Fatalf("ожидался первый элемент \"x\" по значению, получено %+v", lambda.Captures[0])
	}
	if lambda.Captures[1].Name != "y" || !lambda.Captures[1].ByRef {
		t.Fatalf("ожидался второй элемент \"&y\" по ссылке, получено %+v", lambda.Captures[1])
	}
}

// TestParseLambdaWithoutCaptureListUnaffected - лямбда БЕЗ "[...]" (обычная
// форма) - HasCaptures остаётся false, поведение (неявный захват) не
// меняется этой правкой вовсе.
func TestParseLambdaWithoutCaptureListUnaffected(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная ф = функция(): Пусто
    {
        вывести 1
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	lambda := decl.Value.(*ast.LambdaExpr)
	if lambda.HasCaptures {
		t.Fatal("ожидался HasCaptures == false")
	}
}

// Регрессия: "8 >> 2" (обычный побитовый сдвиг) не должен затрагиваться
// специальным разбором закрывающей "<...>" типа "Вызываемый"/generic - тот
// же регрессионный принцип, что и у shr_test.go (шаг 16).
func TestParseCallableTypeDoesNotBreakShiftOperator(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная х = 8 >> 2
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	bin, ok := decl.Value.(*ast.BinaryExpr)
	if !ok || bin.Op != ">>" {
		t.Fatalf("ожидался BinaryExpr \">>\", получено %+v", decl.Value)
	}
}
