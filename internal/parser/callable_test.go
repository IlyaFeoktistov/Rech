package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseCallableTypeName(t *testing.T) {
	prog, err := parseSource(t, `
функция обработать(колбэк: Вызываемый<Целое, Строка>Целое): Целое
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
функция обработать(колбэк: Вызываемый<>): Целое
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
