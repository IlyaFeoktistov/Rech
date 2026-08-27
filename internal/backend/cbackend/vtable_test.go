package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRPlainEntityGetsNoVtable - сущность вне какой-либо
// иерархии наследования и не реализующая интерфейсы не должна получать ни
// "vt" в C-структуре, ни отдельную таблицу, ни диспетчеризацию при вызове
// метода - остаётся обычный прямой вызов по мангл-имени.
func TestGenerateFromIRPlainEntityGetsNoVtable(t *testing.T) {
	entity := &ir.Entity{
		Name:   "Точка",
		Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}},
		Methods: []*ir.Method{
			{Name: "показать_x", ReturnType: ir.TypeInt, HasReturn: true, Body: []ir.Stmt{
				&ir.ReturnStmt{Value: &ir.FieldAccess{Receiver: &ir.ThisExpr{Entity: "Точка"}, Field: "x", Type: ir.TypeInt}},
			}},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "p", Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.IntLiteral{Value: 1}}}},
		&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "p", Type: ir.EntityType("Точка")}, Entity: "Точка", Method: "показать_x",
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "const rech_vtable *vt;") {
		t.Fatalf("не участвующая в диспетчеризации сущность не должна получать поле vt, получено:\n%s", out)
	}
	if strings.Contains(out, "rech_vtable") {
		t.Fatalf("программа без наследования/интерфейсов не должна содержать типов диспетчеризации, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_tochka_pokazat_x((v_p));") {
		t.Fatalf("ожидался прямой вызов метода по мангл-имени, получено:\n%s", out)
	}
}

// TestGenerateFromIRInheritingEntityGetsVtableFieldAndCtorAssignment -
// сущность с базовой ("наследует") получает встроенное первым полем "vt" в
// C-структуре, а её конструктор присваивает его на собственную таблицу.
func TestGenerateFromIRInheritingEntityGetsVtableFieldAndCtorAssignment(t *testing.T) {
	base := &ir.Entity{
		Name: "Животное",
		Methods: []*ir.Method{
			{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true, Body: []ir.Stmt{
				&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "..."}},
			}},
		},
	}
	derived := &ir.Entity{
		Name: "Кошка", BaseName: "Животное",
		Methods: []*ir.Method{
			{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true, Body: []ir.Stmt{
				&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "мяу"}},
			}},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "к", Value: &ir.NewExpr{Entity: "Кошка"}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{base, derived}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "const rech_vtable *vt;\n} rech_T_zhivotnoe;") {
		t.Fatalf("ожидалось поле vt первым в структуре базовой сущности, получено:\n%s", out)
	}
	if !strings.Contains(out, "const rech_vtable *vt;\n} rech_T_koshka;") {
		t.Fatalf("ожидалось поле vt первым в структуре производной сущности, получено:\n%s", out)
	}
	if !strings.Contains(out, "p->vt = &rech_VT_zhivotnoe;") {
		t.Fatalf("ожидалось присваивание vt в конструкторе базовой сущности, получено:\n%s", out)
	}
	if !strings.Contains(out, "p->vt = &rech_VT_koshka;") {
		t.Fatalf("ожидалось присваивание vt в конструкторе производной сущности, получено:\n%s", out)
	}
}

// TestGenerateFromIRVtableInstanceFillsOwnAndInheritedSlots - таблица
// производной сущности без переопределения ссылается на C-функцию базовой
// сущности в том же слоте, что и таблица базовой; переопределяющая
// сущность ссылается на собственную функцию.
func TestGenerateFromIRVtableInstanceFillsOwnAndInheritedSlots(t *testing.T) {
	a := &ir.Entity{
		Name: "А",
		Methods: []*ir.Method{
			{Name: "имя", ReturnType: ir.TypeString, HasReturn: true, Body: []ir.Stmt{
				&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "А"}},
			}},
		},
	}
	b := &ir.Entity{Name: "Б", BaseName: "А"} // не переопределяет "имя"
	v := &ir.Entity{
		Name: "В", BaseName: "Б",
		Methods: []*ir.Method{
			{Name: "имя", ReturnType: ir.TypeString, HasReturn: true, Body: []ir.Stmt{
				&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "В"}},
			}},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "б", Value: &ir.NewExpr{Entity: "Б"}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{a, b, v}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static const rech_vtable rech_VT_b = {") {
		t.Fatalf("ожидалась таблица сущности Б, получено:\n%s", out)
	}
	if !strings.Contains(out, "[0] = (rech_vtfunc)&rech_M_a_imya,") {
		t.Fatalf("ожидался слот, унаследованный от А без переопределения (одна функция на два слота Б и А), получено:\n%s", out)
	}
	if !strings.Contains(out, "[0] = (rech_vtfunc)&rech_M_v_imya,") {
		t.Fatalf("ожидался слот, ссылающийся на собственное переопределение В, получено:\n%s", out)
	}
}

// TestGenerateFromIRInterfaceMethodCallGeneratesDispatchThroughSlot -
// вызов метода на параметре интерфейсного типа генерирует приведение
// получателя к rech_vtable_header и косвенный вызов через slots[N], а не
// прямой вызов по мангл-имени какой-либо конкретной сущности.
func TestGenerateFromIRInterfaceMethodCallGeneratesDispatchThroughSlot(t *testing.T) {
	iface := &ir.Interface{Name: "Звучащий", Methods: []ir.InterfaceMethod{
		{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true},
	}}
	cat := &ir.Entity{
		Name: "Кошка", Interfaces: []string{"Звучащий"},
		Methods: []*ir.Method{
			{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true, Body: []ir.Stmt{
				&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "мяу"}},
			}},
		},
	}
	fn := &ir.Function{
		Name:       "озвучить",
		Params:     []ir.Param{{Name: "х", Type: ir.InterfaceType("Звучащий")}},
		ReturnType: ir.TypeString, HasReturn: true,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "х", Type: ir.InterfaceType("Звучащий")},
			Entity:   "Звучащий", Method: "издать_звук", Type: ir.TypeString, HasReturn: true,
		}}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "к", Value: &ir.NewExpr{Entity: "Кошка"}},
		&ir.PrintStmt{Value: &ir.CallExpr{
			Callee: "озвучить", Args: []ir.Expr{&ir.Identifier{Name: "к", Type: ir.EntityType("Кошка")}},
			Type: ir.TypeString, HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Interfaces: []*ir.Interface{iface},
		Entities:   []*ir.Entity{cat},
		Functions:  []*ir.Function{fn, main},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "((const rech_vtable_header *)(v_h))->vt->slots[0]") {
		t.Fatalf("ожидался косвенный вызов через vt->slots, получено:\n%s", out)
	}
	if strings.Contains(out, "rech_M_koshka_izdat_zvuk(v_h)") {
		t.Fatalf("вызов через интерфейсный параметр не должен идти напрямую по мангл-имени конкретной сущности, получено:\n%s", out)
	}
}
