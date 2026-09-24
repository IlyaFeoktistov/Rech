package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRDictStringKeyMethods(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "д", Value: &ir.NewExpr{Entity: "Словарь<Строка, Целое>"}},
		&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "д", Type: ir.EntityType("Словарь<Строка, Целое>")},
			Entity:   "Словарь<Строка, Целое>", Method: "установить",
			Args: []ir.Expr{&ir.StringLiteral{Value: "а"}, &ir.IntLiteral{Value: 1}},
		}},
	}}
	prog := &ir.Program{
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Словарь<Строка, Целое>", Template: "Словарь", KeyType: ir.TypeString, ElemType: ir.TypeInt}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "#include <string.h>") {
		t.Fatalf("ожидался string.h для строкового ключа, получено:\n%s", out)
	}
	if !strings.Contains(out, "strcmp(a, b)") {
		t.Fatalf("ожидалось сравнение ключей через strcmp, получено:\n%s", out)
	}
	if !strings.Contains(out, mangleType("Словарь<Строка, Целое>")+";") {
		t.Fatalf("ожидался typedef struct для Словарь<Строка, Целое>, получено:\n%s", out)
	}
	if !strings.Contains(out, mangleMethod("Словарь<Строка, Целое>", "установить")) {
		t.Fatalf("ожидался вызов установить, получено:\n%s", out)
	}
}
