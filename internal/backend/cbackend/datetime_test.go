package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func dateTimeFieldsForTest() []ir.Field {
	return []ir.Field{
		{Name: "год", Type: ir.TypeInt}, {Name: "месяц", Type: ir.TypeInt},
		{Name: "день", Type: ir.TypeInt}, {Name: "час", Type: ir.TypeInt},
		{Name: "минута", Type: ir.TypeInt}, {Name: "секунда", Type: ir.TypeInt},
	}
}

func TestGenerateFromIRDateTimeBuiltinCalls(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{{Name: "ДатаВремя", IsStruct: true, Fields: dateTimeFieldsForTest()}},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "м", Value: &ir.CallExpr{Callee: "время", Type: ir.TypeInt, HasReturn: true}},
			&ir.VarDecl{Name: "д", Value: &ir.CallExpr{
				Callee: "время_как_дата", Args: []ir.Expr{&ir.Identifier{Name: "м", Type: ir.TypeInt}},
				Type: ir.EntityType("ДатаВремя"), HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "дата_в_строку", Args: []ir.Expr{&ir.Identifier{Name: "д", Type: ir.EntityType("ДатаВремя")}},
				Type: ir.TypeString, HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.CallExpr{
				Callee: "дата_как_время", Args: []ir.Expr{&ir.Identifier{Name: "д", Type: ir.EntityType("ДатаВремя")}},
				Type: ir.TypeInt, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"#include <time.h>",
		"rech_time_now()",
		"rech_time_to_date(v_m)",
		"rech_date_to_string(v_d)",
		"rech_date_to_time(v_d)",
		mangleType("ДатаВремя") + ";",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutDateTimeOmitsTimeH(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "#include <time.h>") {
		t.Fatalf("не ожидался time.h без использования Время/ДатаВремя, получено:\n%s", out)
	}
}
