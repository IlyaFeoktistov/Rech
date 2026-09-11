package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRBytesIOBuiltinCalls - загрузка/раздача
// файлов - "прочитать_байты"/"записать_байты"/"прочитать_файл_байты"/
// "записать_в_файл_байты" (internal/semantic/bytesio.go) - те же операции,
// что "прочитать"/"записать"/"прочитать_файл"/"записать_в_файл", но через
// "Список<Байт>" вместо "Строка".
func TestGenerateFromIRBytesIOBuiltinCalls(t *testing.T) {
	byteListElemType := ir.TypeByte
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "ф", Value: &ir.CallExpr{Callee: "открыть", Args: []ir.Expr{&ir.StringLiteral{Value: "путь"}}, Type: ir.TypeFile, HasReturn: true}},
		&ir.VarDecl{Name: "б", Value: &ir.CallExpr{
			Callee: "прочитать_байты",
			Args:   []ir.Expr{&ir.Identifier{Name: "ф", Type: ir.TypeFile}, &ir.IntLiteral{Value: 1000}},
			Type:   ir.EntityType("Список<Байт>"), HasReturn: true,
		}},
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "записать_байты", Args: []ir.Expr{
			&ir.Identifier{Name: "ф", Type: ir.TypeFile},
			&ir.Identifier{Name: "б", Type: ir.EntityType("Список<Байт>")},
		}}},
		&ir.VarDecl{Name: "ц", Value: &ir.CallExpr{
			Callee: "прочитать_файл_байты", Args: []ir.Expr{&ir.StringLiteral{Value: "путь2"}},
			Type: ir.EntityType("Список<Байт>"), HasReturn: true,
		}},
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "записать_в_файл_байты", Args: []ir.Expr{
			&ir.StringLiteral{Value: "путь3"},
			&ir.Identifier{Name: "ц", Type: ir.EntityType("Список<Байт>")},
		}}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Байт>", Template: "Список", ElemType: byteListElemType}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"static rech_T_spisok_of_bajt *rech_read_bytes(rech_file_t f, long long max)",
		"static void rech_write_bytes(rech_file_t f, rech_T_spisok_of_bajt *b)",
		"static rech_T_spisok_of_bajt *rech_read_whole_file_bytes(const char *path)",
		"static void rech_write_file_bytes(const char *path, rech_T_spisok_of_bajt *b)",
		"rech_read_bytes(v_f, 1000LL)",
		"rech_write_bytes(v_f, v_b)",
		`rech_read_whole_file_bytes("путь2")`,
		`rech_write_file_bytes("путь3", v_c)`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRWithoutBytesIOOmitsHelpers - программа без единого
// вызова bytesio-функций не должна получать их машинерию.
func TestGenerateFromIRWithoutBytesIOOmitsHelpers(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "привет"}}}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_read_bytes") || strings.Contains(out, "rech_bytes_wrap") {
		t.Fatalf("не ожидалась машинерия bytesio без единого вызова, получено:\n%s", out)
	}
}

// TestGenerateFromIRByteStringConvBuiltinCalls - "строка_из_байт"/"байты_
// из_строки" (мост между Список<Байт> и Строка).
func TestGenerateFromIRByteStringConvBuiltinCalls(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "б", Value: &ir.CallExpr{
			Callee: "байты_из_строки", Args: []ir.Expr{&ir.StringLiteral{Value: "текст"}},
			Type: ir.EntityType("Список<Байт>"), HasReturn: true,
		}},
		&ir.PrintStmt{Value: &ir.CallExpr{
			Callee: "строка_из_байт", Args: []ir.Expr{&ir.Identifier{Name: "б", Type: ir.EntityType("Список<Байт>")}},
			Type: ir.TypeString, HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Байт>", Template: "Список", ElemType: ir.TypeByte}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"static char *rech_string_from_bytes(rech_T_spisok_of_bajt *b)",
		"static rech_T_spisok_of_bajt *rech_bytes_from_string(const char *s)",
		`rech_bytes_from_string("текст")`,
		"rech_string_from_bytes(v_b)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}
