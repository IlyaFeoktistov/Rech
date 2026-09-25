package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRParseExprEntityAndList(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Пользователь", Fields: []ir.Field{
				{Name: "имя", Type: ir.TypeString}, {Name: "возраст", Type: ir.TypeInt},
			}},
			{Name: "ОшибкаРазбора", BaseName: "Исключение", Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}},
			{Name: "Исключение", Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}},
		},
		Generics:     []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt}},
		ParseTargets: []ir.Type{ir.EntityType("Пользователь"), ir.EntityType("Список<Целое>")},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "п", Value: &ir.ParseExpr{
				Format: "JSON", Source: &ir.StringLiteral{Value: "{}"}, To: ir.EntityType("Пользователь"),
			}},
			&ir.VarDecl{Name: "с", Value: &ir.ParseExpr{
				Format: "JSON", Source: &ir.StringLiteral{Value: "[]"}, To: ir.EntityType("Список<Целое>"),
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_json_extract_" + "polzovatel(rech_jv v)",
		"rech_json_extract_int(rech_jv v)",
		"rech_json_extract_spisok_of_celoe(rech_jv v)",
		"static rech_jv rech_json_parse(",
		"static void rech_struct_throw(const char *msg)",
		"rech_error_as(RECH_ERR_PARSE, msg)",
		"#include <string.h>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutParseExprOmitsJSONRuntime(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_json_parse") {
		t.Fatalf("не ожидался JSON-рантайм без \"разобрать\", получено:\n%s", out)
	}
}

func TestGenerateFromIRParseExprNestedEntityGeneratesBothExtractors(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Адрес", Fields: []ir.Field{{Name: "город", Type: ir.TypeString}}},
			{Name: "Пользователь", Fields: []ir.Field{
				{Name: "имя", Type: ir.TypeString}, {Name: "адрес", Type: ir.EntityType("Адрес")},
			}},
			{Name: "ОшибкаРазбора", BaseName: "Исключение", Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}},
			{Name: "Исключение", Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}},
		},
		ParseTargets: []ir.Type{ir.EntityType("Пользователь")},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "п", Value: &ir.ParseExpr{
				Format: "JSON", Source: &ir.StringLiteral{Value: "{}"}, To: ir.EntityType("Пользователь"),
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{"rech_json_extract_polzovatel(rech_jv v)", "rech_json_extract_adres(rech_jv v)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}
