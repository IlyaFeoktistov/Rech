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
