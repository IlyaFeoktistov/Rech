package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRHTTPServerEmitsChunkedHelpers - docs/ROADMAP.md, HTTP
// highload пункт 4 - программа с "принять_http" должна получить хелперы
// разбора chunked transfer encoding, программа без единого HTTP-примитива -
// нет.
func TestGenerateFromIRHTTPServerEmitsChunkedHelpers(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Запрос", Fields: []ir.Field{
				{Name: "метод", Type: ir.TypeString}, {Name: "адрес", Type: ir.TypeString},
				{Name: "порт", Type: ir.TypeInt}, {Name: "путь", Type: ir.TypeString},
				{Name: "заголовки", Type: ir.TypeString}, {Name: "тело", Type: ir.TypeString},
			}},
		},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "с", Value: &ir.CallExpr{Callee: "принять_http", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.EntityType("Запрос"), HasReturn: true}},
		}}},
		HTTPMaxRequestBytes: 1 << 20,
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"static int rech_http_is_chunked(",
		"static char *rech_http_dechunk(",
		"static int rech_http_try_dechunk(",
		"rech_http_is_chunked(buf, sep)",
		"rech_http_try_dechunk(body_start, avail, &dechunked, &dechunked_len)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutHTTPOmitsChunkedHelpers(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "привет"}}}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_http_is_chunked") || strings.Contains(out, "rech_http_dechunk") {
		t.Fatalf("не ожидались chunked-хелперы без единого HTTP-примитива, получено:\n%s", out)
	}
}

// TestGenerateFromIRHTTPClientUsesChunkedDecode - "запрос" (HTTP-клиент)
// тоже должен проверять chunked при разборе ОТВЕТА, не только сервер -
// запроса.
func TestGenerateFromIRHTTPClientUsesChunkedDecode(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Запрос", Fields: []ir.Field{
				{Name: "метод", Type: ir.TypeString}, {Name: "адрес", Type: ir.TypeString},
				{Name: "порт", Type: ir.TypeInt}, {Name: "путь", Type: ir.TypeString},
				{Name: "заголовки", Type: ir.TypeString}, {Name: "тело", Type: ir.TypeString},
			}},
			{Name: "Ответ", Fields: []ir.Field{
				{Name: "код", Type: ir.TypeInt}, {Name: "заголовки", Type: ir.TypeString}, {Name: "тело", Type: ir.TypeString},
			}},
		},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "р", Value: &ir.CallExpr{
				Callee: "запрос", Args: []ir.Expr{&ir.Identifier{Name: "р", Type: ir.EntityType("Запрос")}},
				Type: ir.EntityType("Ответ"), HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_http_is_chunked(raw, body_sep)") {
		t.Fatalf("ожидалась проверка chunked в HTTP-клиенте, получено:\n%s", out)
	}
}
