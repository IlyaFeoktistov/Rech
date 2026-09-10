package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRUserGenericEntityGetsOwnStruct - пользовательская
// generic-сущность уже раскрыта semantic.Analyze в обычный ir.Entity
// ("Коробка<Целое>") до того, как cbackend вообще её увидел - генерация
// не отличает её от любой другой сущности программы (обычный typedef
// struct + конструктор), никакого отдельного кода под generics в самом
// cbackend нет.
func TestGenerateFromIRUserGenericEntityGetsOwnStruct(t *testing.T) {
	inst := &ir.Entity{Name: "Коробка<Целое>", Fields: []ir.Field{{Name: "содержимое", Type: ir.TypeInt}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "к", Value: &ir.NewExpr{Entity: "Коробка<Целое>", Args: []ir.Expr{&ir.IntLiteral{Value: 5}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{inst}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "typedef struct {\n    long long f_soderzhimoe;\n} rech_T_korobka_of_celoe;") {
		t.Fatalf("ожидался typedef структуры инстанциации, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_new_korobka_of_celoe(") {
		t.Fatalf("ожидался конструктор инстанциации, получено:\n%s", out)
	}
	if strings.Contains(out, "неизвестный generic-шаблон") {
		t.Fatalf("пользовательская generic-сущность не должна путаться со встроенными generic-коллекциями, получено:\n%s", out)
	}
}

// TestGenerateFromIRMultiParamGenericMangling - несколько параметров типа
// ("Пара<Целое, Строка>") мангируются в единый валидный C-идентификатор -
// разделитель ", " между аргументами (пробел в том числе) не должен
// просочиться в имя как есть.
func TestGenerateFromIRMultiParamGenericMangling(t *testing.T) {
	inst := &ir.Entity{Name: "Пара<Целое, Строка>", Fields: []ir.Field{
		{Name: "первый", Type: ir.TypeInt}, {Name: "второй", Type: ir.TypeString},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "п", Value: &ir.NewExpr{Entity: "Пара<Целое, Строка>", Args: []ir.Expr{&ir.IntLiteral{Value: 1}, &ir.StringLiteral{Value: "x"}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{inst}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_T_para_of_celoe_stroka") {
		t.Fatalf("ожидалось мангл-имя без пробела rech_T_para_of_celoe_stroka, получено:\n%s", out)
	}
	if strings.Contains(out, "celoe_ stroka") || strings.Contains(out, "celoe_,stroka") {
		t.Fatalf("в мангл-имени просочился разделитель аргументов как есть, получено:\n%s", out)
	}
}

// TestGenerateFromIRListEqualPrimitiveElement - Equatable:true у
// "Список<Целое>" (internal/semantic уже решил, что "равен" существует, IR
// доверяет полю - см. package doc ir.GenericInstantiation.Equatable)
// эмитит функцию поэлементного сравнения, вызываемую как обычный метод.
func TestGenerateFromIRListEqualPrimitiveElement(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.NewExpr{Entity: "Список<Целое>"}},
		&ir.VarDecl{Name: "б", Value: &ir.NewExpr{Entity: "Список<Целое>"}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "а", Type: ir.EntityType("Список<Целое>")},
			Entity:   "Список<Целое>", Method: "равен", Type: ir.TypeBool,
			Args: []ir.Expr{&ir.Identifier{Name: "б", Type: ir.EntityType("Список<Целое>")}},
		}},
	}}
	prog := &ir.Program{
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt, Equatable: true}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_M_spisok_of_celoe_raven(") {
		t.Fatalf("ожидался вызов rech_M_spisok_of_celoe_raven, получено:\n%s", out)
	}
	if !strings.Contains(out, "this_->len != other->len") {
		t.Fatalf("ожидалось тело функции сравнения по длине, получено:\n%s", out)
	}
}

// TestGenerateFromIRListEqualStructElementOrdersStructEqBeforeUse -
// РЕГРЕССИЯ найденной при живом прогоне ошибки: элемент "структура" -
// "Список<Точка>.равен" (её dest - genericsCodeAfterEntities, package doc
// entitySig.Equatable) зовёт rech_struct_eq_tochka - её прототип/тело
// (structEqProtos/structEqFuncs) обязаны идти РАНЬШЕ в тексте C, иначе
// "implicit declaration"/"conflicting types" у cc (см. правку порядка в
// generateSource).
func TestGenerateFromIRListEqualStructElementOrdersStructEqBeforeUse(t *testing.T) {
	tochka := &ir.Entity{Name: "Точка", IsStruct: true, Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.NewExpr{Entity: "Список<Точка>"}},
		&ir.VarDecl{Name: "б", Value: &ir.NewExpr{Entity: "Список<Точка>"}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "а", Type: ir.EntityType("Список<Точка>")},
			Entity:   "Список<Точка>", Method: "равен", Type: ir.TypeBool,
			Args: []ir.Expr{&ir.Identifier{Name: "б", Type: ir.EntityType("Список<Точка>")}},
		}},
	}}
	prog := &ir.Program{
		Entities:  []*ir.Entity{tochka},
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Точка>", Template: "Список", ElemType: ir.EntityType("Точка"), Equatable: true}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	protoIdx := strings.Index(out, "static bool rech_struct_eq_tochka(")
	useIdx := strings.Index(out, "rech_M_spisok_of_tochka_raven(")
	if protoIdx < 0 || useIdx < 0 || protoIdx > useIdx {
		t.Fatalf("rech_struct_eq_tochka должна быть объявлена РАНЬШЕ Список<Точка>.равен, получено:\n%s", out)
	}
}
