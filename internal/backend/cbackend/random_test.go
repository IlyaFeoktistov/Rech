package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRRandomBuiltinCalls(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.CallExpr{Callee: "случайное", Type: ir.TypeFloat, HasReturn: true}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "случайное_целое",
				Args:   []ir.Expr{&ir.IntLiteral{Value: 1}, &ir.IntLiteral{Value: 10}},
				Type:   ir.TypeInt, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_random()",
		"rech_random_int(1LL, 10LL)",
		"static double rech_random(void)",
		"static long long rech_random_int(long long mn, long long mx)",
		"#define _CRT_RAND_S",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutRandomOmitsHelpers(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_random") {
		t.Fatalf("не ожидался rech_random* без использования случайное/случайное_целое, получено:\n%s", out)
	}
}
