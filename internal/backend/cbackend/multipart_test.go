package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRMultipartBuiltinCall - загрузка
// файлов - "разобрать_multipart" должен получить свои хелперы (генерация
// C, поведение уже проверено живым curl-запросом с реальной multipart-
// формой - см. журнал сессии).
func TestGenerateFromIRMultipartBuiltinCall(t *testing.T) {
	partEnt := &ir.Entity{Name: "ЧастьФормы", Fields: []ir.Field{
		{Name: "имя", Type: ir.TypeString}, {Name: "имя_файла", Type: ir.TypeString},
		{Name: "тип_содержимого", Type: ir.TypeString}, {Name: "данные", Type: ir.EntityType("Список<Байт>")},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "б", Value: &ir.CallExpr{
			Callee: "прочитать_файл_байты", Args: []ir.Expr{&ir.StringLiteral{Value: "путь"}},
			Type: ir.EntityType("Список<Байт>"), HasReturn: true,
		}},
		&ir.VarDecl{Name: "части", Value: &ir.CallExpr{
			Callee: "разобрать_multipart",
			Args:   []ir.Expr{&ir.Identifier{Name: "б", Type: ir.EntityType("Список<Байт>")}, &ir.StringLiteral{Value: "Content-Type: multipart/form-data; boundary=x"}},
			Type:   ir.EntityType("Список<ЧастьФормы>"), HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities:  []*ir.Entity{partEnt},
		Functions: []*ir.Function{main},
		Generics: []ir.GenericInstantiation{
			{Name: "Список<Байт>", Template: "Список", ElemType: ir.TypeByte},
			{Name: "Список<ЧастьФормы>", Template: "Список", ElemType: ir.EntityType("ЧастьФормы")},
		},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"static const unsigned char *rech_memfind(",
		"static char *rech_multipart_extract_boundary(const char *headers)",
		"static char *rech_multipart_disposition_param(",
		"static char *rech_multipart_part_content_type(",
		"static rech_T_spisok_of_chastformy *rech_parse_multipart(",
		`rech_parse_multipart(v_b, "Content-Type: multipart/form-data; boundary=x")`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRWithoutMultipartOmitsHelpers - программа без единого
// вызова "разобрать_multipart" не должна получать эту машинерию.
func TestGenerateFromIRWithoutMultipartOmitsHelpers(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "привет"}}}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_parse_multipart") || strings.Contains(out, "rech_memfind") {
		t.Fatalf("не ожидалась машинерия multipart без вызова разобрать_multipart, получено:\n%s", out)
	}
}
