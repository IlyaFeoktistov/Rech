package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRHTTPClientEmitsTLSConnect - docs/ROADMAP.md, HTTP
// highload пункт 5 - программа с "запрос" должна получить TLS-осведомлённый
// вариант сокетов (socketHelpersTLS, usesTLS) БЕЗУСЛОВНО, даже без единого
// "слушать_tcp_защищённо" - "адрес" запроса решается только в рантайме,
// компилятор не может заранее знать, встретится ли когда-нибудь "https://".
func TestGenerateFromIRHTTPClientEmitsTLSConnect(t *testing.T) {
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
	for _, want := range []string{
		"#include <openssl/ssl.h>",
		"static rech_socket_t rech_connect_tcp_tls(const char *host, long long port)",
		"is_https = strncmp(req.",
		"is_https ? rech_connect_tcp_tls(host,",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRWithoutHTTPOmitsTLS - программа без единого сетевого
// примитива вообще не должна тянуть OpenSSL.
func TestGenerateFromIRWithoutHTTPOmitsTLS(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "привет"}}}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "openssl") || strings.Contains(out, "rech_connect_tcp_tls") {
		t.Fatalf("не ожидался OpenSSL без единого сетевого примитива, получено:\n%s", out)
	}
}
