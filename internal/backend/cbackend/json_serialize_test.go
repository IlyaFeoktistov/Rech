package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRSerializeExprEntityAndList(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Пользователь", Fields: []ir.Field{
				{Name: "имя", Type: ir.TypeString}, {Name: "возраст", Type: ir.TypeInt},
			}},
		},
		Generics:         []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt}},
		SerializeTargets: []ir.Type{ir.EntityType("Пользователь"), ir.EntityType("Список<Целое>")},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "п", Value: &ir.NewExpr{Entity: "Пользователь", Args: []ir.Expr{&ir.StringLiteral{Value: "Аня"}, &ir.IntLiteral{Value: 30}}}},
			&ir.PrintStmt{Value: &ir.SerializeExpr{Format: "JSON", Value: &ir.Identifier{Name: "п", Type: ir.EntityType("Пользователь")}}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_json_serialize_to_string_polzovatel(",
		"static char *rech_json_serialize_to_string_polzovatel(",
		"static void rech_json_serialize_polzovatel(",
		"static void rech_json_serialize_string(const char *v",
		"static void rech_json_serialize_int(long long v",
		"rech_json_sb_put_escaped(sb, \"имя\");",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutSerializeExprOmitsRuntime(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_json_sb_init") {
		t.Fatalf("не ожидался JSON-сериализатор без \"сериализовать\", получено:\n%s", out)
	}
}
