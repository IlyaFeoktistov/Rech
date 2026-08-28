package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRXMLSerializeExprEntityAndList(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Пользователь", Fields: []ir.Field{{Name: "имя", Type: ir.TypeString}}},
		},
		Generics:            []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt}},
		XMLSerializeTargets: []ir.Type{ir.EntityType("Пользователь"), ir.EntityType("Список<Целое>")},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "п", Value: &ir.NewExpr{Entity: "Пользователь", Args: []ir.Expr{&ir.StringLiteral{Value: "Аня"}}}},
			&ir.PrintStmt{Value: &ir.SerializeExpr{Format: "XML", Value: &ir.Identifier{Name: "п", Type: ir.EntityType("Пользователь")}}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_xml_serialize_polzovatel(",
		"static void rech_xml_serialize_polzovatel(",
		"rech_xml_serialize_to_string_polzovatel(",
		"static char *rech_xml_serialize_to_string_polzovatel(",
		"static char *rech_xml_serialize_to_string_spisok_of_celoe(",
		"rech_xml_write_escaped(",
		"\"значение\"",
		"\"элемент\"",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRXMLSerializeAndParseShareSb(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Пользователь", Fields: []ir.Field{{Name: "имя", Type: ir.TypeString}}},
		},
		XMLParseTargets:     []ir.Type{ir.EntityType("Пользователь")},
		XMLSerializeTargets: []ir.Type{ir.EntityType("Пользователь")},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "а", Value: &ir.ParseExpr{Format: "XML", Source: &ir.StringLiteral{Value: "<a/>"}, To: ir.EntityType("Пользователь")}},
			&ir.PrintStmt{Value: &ir.SerializeExpr{Format: "XML", Value: &ir.Identifier{Name: "а", Type: ir.EntityType("Пользователь")}}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if n := strings.Count(out, "static void rech_xml_sb_init(rech_xml_sb *sb) {"); n != 1 {
		t.Fatalf("rech_xml_sb_init должен быть определён ровно один раз, получено %d раз:\n%s", n, out)
	}
}
