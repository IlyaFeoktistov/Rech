package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRPointerAddressAndDeref - "Указатель<Целое>" - "T *" по
// значению (см. package doc internal/backend/cbackend/valuetype.go,
// kindPointer), "&а" -> "(&v_a)", "*п = ..." -> "*<...> = ...;".
func TestGenerateFromIRPointerAddressAndDeref(t *testing.T) {
	ptrType := ir.Type{Kind: ir.KindPointer, Entity: "Указатель<Целое>"}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.IntLiteral{Value: 5}},
		&ir.VarDecl{Name: "п", Value: &ir.AddressOfExpr{
			Receiver: &ir.Identifier{Name: "а", Type: ir.TypeInt},
			Type:     ptrType,
		}},
		&ir.PrintStmt{Value: &ir.DerefExpr{
			Value: &ir.Identifier{Name: "п", Type: ptrType},
			Type:  ir.TypeInt,
		}},
		&ir.DerefAssignStmt{
			Pointer: &ir.Identifier{Name: "п", Type: ptrType},
			Value:   &ir.IntLiteral{Value: 99},
		},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Functions: []*ir.Function{main},
		Pointers:  []ir.PointerInstantiation{{Name: ptrType.Entity, Inner: ir.TypeInt}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"long long * v_p",
		"(&v_a)",
		"(*v_p)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q в сгенерированный C, получено:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "*v_p = 99LL;") {
		t.Fatalf("ожидалась запись через указатель \"*v_p = 99LL;\", получено:\n%s", out)
	}
}

// TestGenerateFromIRPrintPointerUsesPercentP - "вывести &а" (без
// разыменования) печатает адрес через "%p" - не "%s" (было бы неопределённым
// поведением на указателе, не char *, см. printCall).
func TestGenerateFromIRPrintPointerUsesPercentP(t *testing.T) {
	ptrType := ir.Type{Kind: ir.KindPointer, Entity: "Указатель<Целое>"}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.IntLiteral{Value: 5}},
		&ir.PrintStmt{Value: &ir.AddressOfExpr{
			Receiver: &ir.Identifier{Name: "а", Type: ir.TypeInt},
			Type:     ptrType,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Functions: []*ir.Function{main},
		Pointers:  []ir.PointerInstantiation{{Name: ptrType.Entity, Inner: ir.TypeInt}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `printf("%p", (void *)((&v_a)));`) {
		t.Fatalf("ожидался printf(\"%%p\", ...) для указателя, получено:\n%s", out)
	}
}

// TestGenerateFromIRPointerNullLiteralIsNULL - "Ничто" типа "Указатель<T>" -
// буквальный "NULL", НЕ "<T>_empty()" (has/value-обёртка "?Тип") - см.
// package doc ir.NullLiteral/coerceExpr (internal/semantic/nullable.go).
func TestGenerateFromIRPointerNullLiteralIsNULL(t *testing.T) {
	ptrType := ir.Type{Kind: ir.KindPointer, Entity: "Указатель<Целое>"}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "п", Value: &ir.NullLiteral{Type: ptrType}},
		&ir.PrintStmt{Value: &ir.NullCheckExpr{
			Value: &ir.Identifier{Name: "п", Type: ptrType},
		}},
		&ir.PrintStmt{Value: &ir.NullCheckExpr{
			Value: &ir.Identifier{Name: "п", Type: ptrType},
			IsNot: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Functions: []*ir.Function{main},
		Pointers:  []ir.PointerInstantiation{{Name: ptrType.Entity, Inner: ir.TypeInt}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"long long * v_p_INIT = NULL;",
		"(v_p) == NULL",
		"(v_p) != NULL",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q в сгенерированный C, получено:\n%s", want, out)
		}
	}
	if strings.Contains(out, ".has") {
		t.Fatalf("для \"Указатель<T>\" не должна использоваться has/value-обёртка \"?Тип\", получено:\n%s", out)
	}
}
