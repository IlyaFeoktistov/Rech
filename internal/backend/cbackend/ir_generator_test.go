package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRPrintString(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.StringLiteral{Value: "Привет, мир!"}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `int main(void) {`) {
		t.Fatalf("ожидалась сигнатура main, получено:\n%s", out)
	}
	if !strings.Contains(out, `printf("%s\n", "Привет, мир!");`) {
		t.Fatalf("ожидался вызов printf, получено:\n%s", out)
	}
}

func TestGenerateFromIRVarDeclAssignAndArithmetic(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "а", Value: &ir.IntLiteral{Value: 10}},
			&ir.Assign{Name: "а", Value: &ir.BinaryExpr{
				Op: "+", Left: &ir.Identifier{Name: "а", Type: ir.TypeInt}, Right: &ir.IntLiteral{Value: 1}, Type: ir.TypeInt,
			}},
			&ir.PrintStmt{Value: &ir.Identifier{Name: "а", Type: ir.TypeInt}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "long long v_a = 10LL;") {
		t.Fatalf("ожидалось объявление переменной, получено:\n%s", out)
	}
	if !strings.Contains(out, "v_a = (v_a + 1LL);") {
		t.Fatalf("ожидалось присваивание с арифметикой, получено:\n%s", out)
	}
}

func TestGenerateFromIRStringConcatUsesHelper(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.BinaryExpr{
				Op: "+", Left: &ir.StringLiteral{Value: "Привет, "}, Right: &ir.StringLiteral{Value: "мир!"}, Type: ir.TypeString,
			}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_str_concat(") || !strings.Contains(out, "static char *rech_str_concat") {
		t.Fatalf("ожидался вызов и определение rech_str_concat, получено:\n%s", out)
	}
}

func TestGenerateFromIRIntFloatPromotion(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.BinaryExpr{
				Op: "+", Left: &ir.IntLiteral{Value: 1}, Right: &ir.FloatLiteral{Value: 2.5}, Type: ir.TypeFloat,
			}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "((double)1LL) + 2.5") {
		t.Fatalf("ожидалось приведение Целое к double при смешении с Число, получено:\n%s", out)
	}
}

func TestGenerateFromIRIfWhileForReturn(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.IfStmt{
				Cond: &ir.BoolLiteral{Value: true},
				Then: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "да"}}},
				Else: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "нет"}}},
			},
			&ir.WhileStmt{
				Cond: &ir.BoolLiteral{Value: false},
				Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "x"}}},
			},
			&ir.ForStmt{
				Var:  "i",
				From: &ir.IntLiteral{Value: 1},
				To:   &ir.IntLiteral{Value: 5},
				Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.Identifier{Name: "i", Type: ir.TypeInt}}},
			},
			&ir.ReturnStmt{},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"if (true) {", "} else {",
		"while (false) {",
		"for (long long v_i = 1LL; v_i <= 5LL; v_i++) {",
		"return 0;",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRMissingEntryPointFails(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "чтототам", Body: nil},
	}}
	_, err := GenerateFromIR(prog)
	if err == nil || !strings.Contains(err.Error(), "не найдена функция") {
		t.Fatalf("ожидалась ошибка про отсутствие точки входа, получено: %v", err)
	}
}
