package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseTypeOfExpr(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная т = тип Целое
    вывести т
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl, ok := prog.Functions[0].Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %+v", prog.Functions[0].Body[0])
	}
	typeOf, ok := decl.Value.(*ast.TypeOfExpr)
	if !ok || typeOf.TypeName != "Целое" {
		t.Fatalf("ожидался TypeOfExpr{TypeName: \"Целое\"}, получено %+v", decl.Value)
	}
}

func TestParseAsExpr(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная коробка = (Значение) 5
    переменная обратно = коробка как Целое
    вывести обратно
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl, ok := prog.Functions[0].Body[1].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %+v", prog.Functions[0].Body[1])
	}
	as, ok := decl.Value.(*ast.AsExpr)
	if !ok || as.TypeName != "Целое" {
		t.Fatalf("ожидался AsExpr{TypeName: \"Целое\"}, получено %+v", decl.Value)
	}
	if _, ok := as.Value.(*ast.Identifier); !ok {
		t.Fatalf("ожидался Identifier как разбоксируемое значение, получено %+v", as.Value)
	}
}

// TestParseAsExprDoesNotBreakRelatesTo - "как" и "относится к" - две
// независимые конструкции на одном уровне приоритета (parseComparison) -
// ни одна не должна мешать разбору другой.
func TestParseAsExprDoesNotBreakRelatesTo(t *testing.T) {
	_, err := parseSource(t, `
сущность Ошибка наследует Исключение {}
функция главная
{
    попытаться {
        выбросить новая Ошибка("текст")
    } поймать(иск: Исключение) {
        если иск относится к Ошибка {
            вывести "поймано"
        }
    }
    переменная коробка = (Значение) 5
    переменная обратно = коробка как Целое
    вывести обратно
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestParseBoxCastRecognizesZnachenieAsCastType - "Значение" уже входит в
// primitiveCastTypes (в отличие от произвольного имени сущности - тех
// парсер кастом не считает).
func TestParseBoxCastRecognizesZnachenieAsCastType(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная коробка = (Значение) 5
    вывести коробка
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl, ok := prog.Functions[0].Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %+v", prog.Functions[0].Body[0])
	}
	cast, ok := decl.Value.(*ast.CastExpr)
	if !ok || cast.Type != "Значение" {
		t.Fatalf("ожидался CastExpr{Type: \"Значение\"}, получено %+v", decl.Value)
	}
}
