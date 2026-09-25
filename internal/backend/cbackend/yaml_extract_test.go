package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRYAMLParseExprEntityAndList(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Пользователь", Fields: []ir.Field{
				{Name: "имя", Type: ir.TypeString}, {Name: "возраст", Type: ir.TypeInt},
			}},
			{Name: "ОшибкаРазбора", BaseName: "Исключение", Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}},
			{Name: "Исключение", Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}},
		},
		Generics:         []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt}},
		YAMLParseTargets: []ir.Type{ir.EntityType("Пользователь"), ir.EntityType("Список<Целое>")},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "п", Value: &ir.ParseExpr{
				Format: "YAML", Source: &ir.StringLiteral{Value: "имя: аня"}, To: ir.EntityType("Пользователь"),
			}},
			&ir.VarDecl{Name: "с", Value: &ir.ParseExpr{
				Format: "YAML", Source: &ir.StringLiteral{Value: "[]"}, To: ir.EntityType("Список<Целое>"),
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_yaml_extract_" + "polzovatel(rech_yv v)",
		"rech_yaml_extract_int(rech_yv v)",
		"rech_yaml_extract_spisok_of_celoe(rech_yv v)",
		"static rech_yv rech_yaml_parse(",
		"static void rech_struct_throw(const char *msg)",
		"rech_error_as(RECH_ERR_PARSE, msg)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutYAMLOmitsYAMLRuntime(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_yaml_parse") {
		t.Fatalf("не ожидался YAML-рантайм без \"разобрать YAML\", получено:\n%s", out)
	}
}

// TestGenerateFromIRYAMLSerializeEntity - "сериализовать YAML" сущности с
// не-примитивным полем (Список<T>) генерирует функцию сериализации с
// параметром indent (в отличие от JSON, где отступ не нужен) и вызывает
// вложенный сериализатор при indent+2 - см. package doc yaml_serialize.go.
func TestGenerateFromIRYAMLSerializeEntity(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Пользователь", Fields: []ir.Field{
				{Name: "имя", Type: ir.TypeString},
				{Name: "теги", Type: ir.EntityType("Список<Строка>")},
			}},
		},
		Generics:             []ir.GenericInstantiation{{Name: "Список<Строка>", Template: "Список", ElemType: ir.TypeString}},
		YAMLSerializeTargets: []ir.Type{ir.EntityType("Пользователь")},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "п", Value: &ir.NewExpr{Entity: "Пользователь"}},
			&ir.PrintStmt{Value: &ir.SerializeExpr{Format: "YAML", Value: &ir.Identifier{Name: "п", Type: ir.EntityType("Пользователь")}}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"static char *rech_yaml_serialize_to_string_polzovatel(",
		"static void rech_yaml_serialize_polzovatel(",
		"long long indent",
		"indent + 2",
		"rech_yaml_sb_puts(sb, \"[]\\n\");",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}
