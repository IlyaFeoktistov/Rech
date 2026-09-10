package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRListIndexEmitsSafeGetOnlyWhenUsed - "?T"-обёртка и
// "_get_safe" эмитятся, только если "[]" реально встретилось
// (NeedsSafeIndex) - обычное использование Список<T> без "[]" не должно
// тянуть за собой этот код.
func TestGenerateFromIRListIndexEmitsSafeGetOnlyWhenUsed(t *testing.T) {
	elemType := ir.TypeInt
	nullableType := ir.NullableType(elemType)
	listType := ir.EntityType("Список<Целое>")
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.NewExpr{Entity: "Список<Целое>"}},
		&ir.PrintStmt{Value: &ir.NullCheckExpr{
			Value: &ir.ListIndexExpr{
				Receiver:       &ir.Identifier{Name: "а", Type: listType},
				Index:          &ir.IntLiteral{Value: 0},
				CollectionName: "Список<Целое>",
				Type:           nullableType,
			},
			IsNot: true,
		}},
	}}
	prog := &ir.Program{
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: elemType, NeedsSafeIndex: true}},
		Nullables: []ir.NullableInstantiation{{Name: nullableType.Entity, Inner: elemType}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	structName := mangleType("Список<Целое>")
	getSafe := structName + "_get_safe"
	if !strings.Contains(out, getSafe+"(") {
		t.Fatalf("ожидалась функция %q, получено:\n%s", getSafe, out)
	}
	// _get_safe должна идти ПОСЛЕ typedef'а самой коллекции и ПОСЛЕ
	// typedef'а её nullable-обёртки - иначе тип возврата/параметр функции
	// не объявлен на момент её собственного объявления.
	listTypedefIdx := strings.Index(out, "struct "+structName+" {")
	nullableTypedefIdx := strings.Index(out, "} "+mangleType(nullableType.Entity)+";")
	getSafeIdx := strings.Index(out, "static "+mangleType(nullableType.Entity)+" "+getSafe)
	if listTypedefIdx == -1 || nullableTypedefIdx == -1 || getSafeIdx == -1 {
		t.Fatalf("не найдены ожидаемые фрагменты в:\n%s", out)
	}
	if getSafeIdx < listTypedefIdx || getSafeIdx < nullableTypedefIdx {
		t.Fatalf("_get_safe должна идти после обоих typedef'ов, получено:\n%s", out)
	}
}

// TestGenerateFromIRListWithoutIndexUseSkipsSafeGet - Список<T>, ни разу не
// индексированный через "[]" (NeedsSafeIndex == false), не получает
// "_get_safe" вообще.
func TestGenerateFromIRListWithoutIndexUseSkipsSafeGet(t *testing.T) {
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
	if strings.Contains(out, "_get_safe") {
		t.Fatalf("не ожидалась \"_get_safe\" без использования \"[]\", получено:\n%s", out)
	}
}
