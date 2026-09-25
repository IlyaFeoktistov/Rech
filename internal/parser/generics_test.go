package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseGenericEntityDecl(t *testing.T) {
	prog, err := parseSource(t, `
сущность Коробка<T>
{
    имеет:
        содержимое: T
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if len(ent.TypeParams) != 1 || ent.TypeParams[0].Name != "T" {
		t.Fatalf("ожидался один параметр типа T, получено %+v", ent.TypeParams)
	}
	if ent.Fields[0].Type != "T" {
		t.Fatalf("ожидался тип поля T, получено %q", ent.Fields[0].Type)
	}
}

func TestParseGenericEntityMultipleParamsWithConstraints(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пара<A как Целое|Строка, Б>
{
    имеет:
        первый: A
        второй: Б
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	tp := prog.Entities[0].TypeParams
	if len(tp) != 2 || tp[0].Name != "A" || tp[1].Name != "Б" {
		t.Fatalf("ожидались параметры A, Б, получено %+v", tp)
	}
	if len(tp[0].Constraints) != 2 || tp[0].Constraints[0] != "Целое" || tp[0].Constraints[1] != "Строка" {
		t.Fatalf("ожидалось ограничение [Целое Строка] у A, получено %+v", tp[0].Constraints)
	}
	if tp[1].Constraints != nil {
		t.Fatalf("Б без ограничений, получено %+v", tp[1].Constraints)
	}
}

func TestParseGenericEntityInheritsAndImplements(t *testing.T) {
	prog, err := parseSource(t, `
интерфейс Описываемый
{
    описание(): Строка
}
сущность Коробка<T> реализует Описываемый
{
}
сущность Подписанная<T> наследует Коробку<T>
{
}
сущность ЦелаяКоробка наследует Коробку<Целое>
{
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := map[string]string{"Подписанная": "Коробка<T>", "ЦелаяКоробка": "Коробка<Целое>"}
	for _, ent := range prog.Entities {
		if base, ok := want[ent.Name]; ok && ent.BaseName != base {
			t.Errorf("%s: база %q, ожидалась %q", ent.Name, ent.BaseName, base)
		}
		if ent.Name == "Коробка" && (len(ent.Interfaces) != 1 || ent.Interfaces[0] != "Описываемый") {
			t.Errorf("Коробка: интерфейсы %v", ent.Interfaces)
		}
	}
}

func TestParseGenericFunctionDeclAndTypeParamDedup(t *testing.T) {
	_, err := parseSource(t, `
функция первый<T, T>(x: T): T
{
    вернуть x
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "уже объявлен") {
		t.Fatalf("ожидалась ошибка про повторный параметр типа, получено: %v", err)
	}
}

func TestParseGenericCallExprDistinguishedFromComparison(t *testing.T) {
	prog, err := parseSource(t, `
функция первый<T>(x: T): T
{
    вернуть x
}
функция главная
{
    переменная а = 1
    переменная б = 2
    вывести а < б
    вывести первый<Целое>(а)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[1]
	printCmp, ok := fn.Body[2].(*ast.PrintStmt)
	if !ok {
		t.Fatalf("ожидался PrintStmt, получено %T", fn.Body[2])
	}
	if _, ok := printCmp.Value.(*ast.BinaryExpr); !ok {
		t.Fatalf("ожидалось \"а < б\" как обычное сравнение (BinaryExpr), получено %T", printCmp.Value)
	}
	printCall, ok := fn.Body[3].(*ast.PrintStmt)
	if !ok {
		t.Fatalf("ожидался PrintStmt, получено %T", fn.Body[3])
	}
	call, ok := printCall.Value.(*ast.CallExpr)
	if !ok || call.Callee != "первый" || len(call.TypeArgs) != 1 || call.TypeArgs[0] != "Целое" {
		t.Fatalf("ожидался generic-вызов первый<Целое>, получено %+v", printCall.Value)
	}
}

// TestParseNestedGenericTypeSplitsShrToken - "Коробка<Коробка<T>>" - лексер
// токенизирует два подряд ">" как один SHR (побитовый сдвиг) - парсер
// обязан расщепить его на два закрывающих "<" при разборе вложенного
// generic-типа (см. Parser.expectCloseAngle).
func TestParseNestedGenericTypeSplitsShrToken(t *testing.T) {
	prog, err := parseSource(t, `
сущность Коробка<T>
{
    имеет:
        содержимое: T
}
функция главная
{
    переменная а = новая Коробка<Целое>(1)
    переменная б = новая Коробка<Коробка<Целое>>(а)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	decl, ok := fn.Body[1].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", fn.Body[1])
	}
	newExpr, ok := decl.Value.(*ast.NewExpr)
	if !ok || newExpr.Type != "Коробка<Коробка<Целое>>" {
		t.Fatalf("ожидался тип \"Коробка<Коробка<Целое>>\", получено %+v", decl.Value)
	}
}

// TestParseBitwiseShiftStillWorksAfterAngleSplitting - regression: >>
// как обычный побитовый сдвиг в выражениях не должен пострадать от
// расщепления SHR для generic-скобок (та же лексема, разный контекст).
func TestParseBitwiseShiftStillWorksAfterAngleSplitting(t *testing.T) {
	prog, err := parseSource(t, `
сущность Коробка<T>
{
    имеет:
        содержимое: T
}
функция главная
{
    переменная а = 8 >> 2
    вывести а
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	decl := fn.Body[0].(*ast.VarDecl)
	bin, ok := decl.Value.(*ast.BinaryExpr)
	if !ok || bin.Op != ">>" {
		t.Fatalf("ожидался BinaryExpr \">>\", получено %+v", decl.Value)
	}
}
