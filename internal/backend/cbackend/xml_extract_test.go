package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRXMLParseExprEntityAndList(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Пользователь", Fields: []ir.Field{
				{Name: "имя", Type: ir.TypeString}, {Name: "возраст", Type: ir.TypeInt},
			}},
		},
		Generics:        []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt}},
		XMLParseTargets: []ir.Type{ir.EntityType("Пользователь"), ir.EntityType("Список<Целое>")},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "п", Value: &ir.ParseExpr{
				Format: "XML", Source: &ir.StringLiteral{Value: "<a/>"}, To: ir.EntityType("Пользователь"),
			}},
			&ir.VarDecl{Name: "с", Value: &ir.ParseExpr{
				Format: "XML", Source: &ir.StringLiteral{Value: "<a/>"}, To: ir.EntityType("Список<Целое>"),
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_xml_extract_polzovatel(rech_xn v)",
		"rech_xml_extract_int(rech_xn v)",
		"rech_xml_extract_spisok_of_celoe(rech_xn v)",
		"static rech_xn rech_xml_parse(",
		"rech_xn_children_named(&v, \"имя\", &m0)",
		"#include <string.h>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutXMLOmitsRuntime(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_xml_parse") {
		t.Fatalf("не ожидался XML-рантайм без \"разобрать XML\", получено:\n%s", out)
	}
}

func TestGenerateFromIRXMLAndJSONShareThrowHelper(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Пользователь", Fields: []ir.Field{{Name: "имя", Type: ir.TypeString}}},
		},
		ParseTargets:    []ir.Type{ir.EntityType("Пользователь")},
		XMLParseTargets: []ir.Type{ir.EntityType("Пользователь")},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "а", Value: &ir.ParseExpr{Format: "JSON", Source: &ir.StringLiteral{Value: "{}"}, To: ir.EntityType("Пользователь")}},
			&ir.VarDecl{Name: "б", Value: &ir.ParseExpr{Format: "XML", Source: &ir.StringLiteral{Value: "<a/>"}, To: ir.EntityType("Пользователь")}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if n := strings.Count(out, "static void rech_struct_throw(const char *msg) {"); n != 1 {
		t.Fatalf("rech_struct_throw должен быть определён ровно один раз при одновременном использовании JSON и XML, получено %d раз:\n%s", n, out)
	}
}
