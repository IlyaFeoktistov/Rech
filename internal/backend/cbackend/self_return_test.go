package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRSelfReturnCastsDispatchCallToDerivedType - метод с
// EntityType(Derived) как результатом (уже разрешённый семантическим
// анализом из "этот" - см. internal/semantic/inheritance.go,
// applyInheritance), унаследованный без переопределения и вызванный через
// производную сущность - генерирует приведение указателя функции к
// derived-типу результата (тот же приём, что и у обычной vtable-
// диспетчеризации, ничего специального не требуется - см. package doc
// generateDispatchCall).
func TestGenerateFromIRSelfReturnCastsDispatchCallToDerivedType(t *testing.T) {
	base := &ir.Entity{
		Name: "СтроительА",
		Methods: []*ir.Method{
			{Name: "задать", ReturnType: ir.EntityType("СтроительА"), HasReturn: true, Body: []ir.Stmt{
				&ir.ReturnStmt{Value: &ir.ThisExpr{Entity: "СтроительА"}},
			}},
		},
	}
	derived := &ir.Entity{Name: "СтроительБ", BaseName: "СтроительА"}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "б", Value: &ir.NewExpr{Entity: "СтроительБ"}},
		&ir.VarDecl{Name: "б2", Value: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "б", Type: ir.EntityType("СтроительБ")},
			Entity:   "СтроительБ", Method: "задать",
			Type: ir.EntityType("СтроительБ"), HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{base, derived}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_T_stroitelb * (*)(void *)") {
		t.Fatalf("ожидалось приведение вызова к rech_T_stroitelb *, получено:\n%s", out)
	}
}

// TestGenerateFromIRInterfaceReceiverMethodCallDoesNotTakeAddress -
// регрессия: получатель интерфейсного типа - всегда указатель (KindInterface,
// как и KindEntity-ссылка), receiverIsPointer раньше не знал про
// KindInterface и добавлял "&" перед уже-указателем - вызов через
// интерфейсное значение метода, реально трогающего "этот.поле", падал бы с
// segfault на несогласованном this_ (найдено вручную end-to-end проверкой,
// не через существующие тесты - ни один из них до сих пор не проверял
// мутацию поля через интерфейсный получатель).
func TestGenerateFromIRInterfaceReceiverMethodCallDoesNotTakeAddress(t *testing.T) {
	iface := &ir.Interface{Name: "Именуемый", Methods: []ir.InterfaceMethod{
		{Name: "задать_имя", Params: []ir.Param{{Name: "значение", Type: ir.TypeString}}, HasReturn: false},
	}}
	ent := &ir.Entity{
		Name: "Штука", Interfaces: []string{"Именуемый"},
		Fields: []ir.Field{{Name: "имя_поле", Type: ir.TypeString}},
		Methods: []*ir.Method{
			{Name: "задать_имя", Params: []ir.Param{{Name: "значение", Type: ir.TypeString}}, Body: []ir.Stmt{
				&ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: "Штука"}, Field: "имя_поле", Value: &ir.Identifier{Name: "значение", Type: ir.TypeString}},
			}},
		},
	}
	настроить := &ir.Function{
		Name:   "настроить",
		Params: []ir.Param{{Name: "х", Type: ir.InterfaceType("Именуемый")}},
		Body: []ir.Stmt{&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "х", Type: ir.InterfaceType("Именуемый")},
			Entity:   "Именуемый", Method: "задать_имя",
			Args: []ir.Expr{&ir.StringLiteral{Value: "х"}},
		}}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "ш", Value: &ir.NewExpr{Entity: "Штука", Args: []ir.Expr{&ir.StringLiteral{Value: ""}}}},
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "настроить", Args: []ir.Expr{&ir.Identifier{Name: "ш", Type: ir.EntityType("Штука")}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Interfaces: []*ir.Interface{iface}, Entities: []*ir.Entity{ent}, Functions: []*ir.Function{настроить, main},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "&(v_h)") {
		t.Fatalf("получатель интерфейсного типа - уже указатель, не должен получать \"&\", получено:\n%s", out)
	}
	if !strings.Contains(out, "(v_h)") {
		t.Fatalf("ожидался получатель без \"&\" в аргументах вызова, получено:\n%s", out)
	}
}
