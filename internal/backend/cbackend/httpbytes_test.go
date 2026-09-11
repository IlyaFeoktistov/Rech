package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRHTTPBytesBuiltinCalls - загрузка/
// раздача файлов - "принять_http_байты"/"отправить_http_ответ_байты"
// (internal/semantic/httpbytes.go) должны получить свои хелперы плюс
// разделяемые с обычным принять_http/отправить_http_ответ куски
// (rech_recv_http_request/rech_http_parse_head, rech_send_raw).
func TestGenerateFromIRHTTPBytesBuiltinCalls(t *testing.T) {
	reqEnt := &ir.Entity{Name: "ЗапросБайты", Fields: []ir.Field{
		{Name: "метод", Type: ir.TypeString}, {Name: "адрес", Type: ir.TypeString},
		{Name: "порт", Type: ir.TypeInt}, {Name: "путь", Type: ir.TypeString},
		{Name: "заголовки", Type: ir.TypeString}, {Name: "тело", Type: ir.EntityType("Список<Байт>")},
	}}
	respEnt := &ir.Entity{Name: "ОтветБайты", Fields: []ir.Field{
		{Name: "код", Type: ir.TypeInt}, {Name: "заголовки", Type: ir.TypeString}, {Name: "тело", Type: ir.EntityType("Список<Байт>")},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "с", Value: &ir.CallExpr{Callee: "принять_http_байты", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.EntityType("ЗапросБайты"), HasReturn: true}},
		&ir.ExprStmt{Call: &ir.CallExpr{
			Callee: "отправить_http_ответ_байты",
			Args: []ir.Expr{
				&ir.IntLiteral{Value: 0},
				&ir.NewExpr{Entity: "ОтветБайты", Args: []ir.Expr{&ir.IntLiteral{Value: 200}, &ir.StringLiteral{Value: ""}, &ir.Identifier{Name: "с", Type: ir.EntityType("Список<Байт>")}}},
			},
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities:  []*ir.Entity{reqEnt, respEnt},
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Байт>", Template: "Список", ElemType: ir.TypeByte}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"static rech_T_zaprosbajty rech_http_accept_request_bytes(rech_socket_t s)",
		"static void rech_http_send_response_bytes(rech_socket_t s, rech_T_otvetbajty resp)",
		"rech_http_accept_request_bytes(0LL)",
		"rech_http_send_response_bytes(0LL,",
		"rech_bytes_wrap(NULL, 0)",
		"rech_send_raw(s, full,",
		"static rech_T_spisok_of_bajt *rech_bytes_wrap(",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRWithoutHTTPBytesOmitsHelpers - программа без единого
// вызова принять_http_байты/отправить_http_ответ_байты не должна получать
// эту машинерию (даже если использует обычный принять_http).
func TestGenerateFromIRWithoutHTTPBytesOmitsHelpers(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная л = сетевой_узел(СетевойПротокол.TCP, "", 8080)
    переменная клиент = принять(л)
    если клиент != Ничто
    {
        переменная запрос = принять_http(клиент)
        отправить_http_ответ(клиент, новый Ответ(200, "", "ок"))
    }
}
`)
	// Ищем настоящее ОПРЕДЕЛЕНИЕ функции (со скобкой), не любое упоминание
	// имени - package doc rech_http_parse_head (httpserver.go) сам
	// упоминает оба этих имени в прозе, простой strings.Contains по имени
	// дал бы ложное срабатывание на комментарии, а не на код.
	for _, unwanted := range []string{"static rech_T_zaprosbajty rech_http_accept_request_bytes(", "static rech_T_spisok_of_bajt *rech_bytes_wrap("} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("не ожидалась машинерия bytes (%q) без вызова принять_http_байты/отправить_http_ответ_байты, получено:\n%s", unwanted, out)
		}
	}
}
