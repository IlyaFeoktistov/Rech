package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRStringOpsBuiltinCalls(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "длина_строки", Args: []ir.Expr{&ir.StringLiteral{Value: "текст"}},
				Type: ir.TypeInt, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "подстрока",
				Args:   []ir.Expr{&ir.StringLiteral{Value: "текст"}, &ir.IntLiteral{Value: 0}, &ir.IntLiteral{Value: 2}},
				Type:   ir.TypeString, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "позиция_в_строке",
				Args:   []ir.Expr{&ir.StringLiteral{Value: "текст"}, &ir.StringLiteral{Value: "кс"}},
				Type:   ir.TypeInt, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_string_length(",
		"rech_substring(",
		"rech_string_find(",
		"static long long rech_string_length(const char *s)",
		"static char *rech_substring(const char *s, long long start, long long length)",
		"static long long rech_string_find(const char *s, const char *needle)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutStringOpsOmitsHelpers(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_string_length") || strings.Contains(out, "rech_substring") || strings.Contains(out, "rech_string_find") {
		t.Fatalf("не ожидались string_ops хелперы без использования длина_строки/подстрока/позиция_в_строке, получено:\n%s", out)
	}
}
