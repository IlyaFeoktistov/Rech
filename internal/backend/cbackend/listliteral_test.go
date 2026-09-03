package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRListLiteralEmitsFromLiteralHelper - "_from_literal"
// эмитится только для инстанциаций, реально построенных литералом
// (NeedsListLiteral), после typedef'а самой коллекции (использует его как
// тип возврата).
func TestGenerateFromIRListLiteralEmitsFromLiteralHelper(t *testing.T) {
	listType := ir.EntityType("Список<Целое>")
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.ListLiteralExpr{
			Elems:          []ir.Expr{&ir.IntLiteral{Value: 1}, &ir.IntLiteral{Value: 2}},
			ElemType:       ir.TypeInt,
			CollectionName: "Список<Целое>",
			Type:           listType,
		}},
	}}
	prog := &ir.Program{
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt, NeedsListLiteral: true}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	structName := mangleType("Список<Целое>")
	fn := structName + "_from_literal"
	if !strings.Contains(out, fn+"(") {
		t.Fatalf("ожидалась функция %q, получено:\n%s", fn, out)
	}
	if !strings.Contains(out, fn+"(2LL, (long long[]){1LL, 2LL})") {
		t.Fatalf("ожидался вызов с готовым массивом элементов, получено:\n%s", out)
	}
	typedefIdx := strings.Index(out, "} "+structName+";")
	fnIdx := strings.Index(out, "static "+structName+" *"+fn)
	if typedefIdx == -1 || fnIdx == -1 || fnIdx < typedefIdx {
		t.Fatalf("_from_literal должна идти после typedef'а коллекции, получено:\n%s", out)
	}
}

func TestGenerateFromIRListWithoutLiteralUseSkipsFromLiteral(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.NewExpr{Entity: "Список<Целое>"}},
	}}
	prog := &ir.Program{
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "_from_literal") {
		t.Fatalf("не ожидалась \"_from_literal\" без литерала, получено:\n%s", out)
	}
}
