package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRTLSListenBuiltinCall - "слушать_tcp_защищённо(порт,
// файл_сертификата, файл_ключа)" (TLS для http_сервера) -
// эмитирует rech_listen_tcp_secure, подключает OpenSSL, переключает
// rech_accept/rech_send/rech_recv/rech_close_socket на TLS-осведомлённые
// версии (socketHelpersTLS вместо socketHelpers, tls.go).
func TestGenerateFromIRTLSListenBuiltinCall(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "слушатель", Value: &ir.CallExpr{
			Callee: "слушать_tcp_защищённо",
			Args: []ir.Expr{
				&ir.IntLiteral{Value: 8443},
				&ir.StringLiteral{Value: "cert.pem"},
				&ir.StringLiteral{Value: "key.pem"},
			},
			Type: ir.TypeSocket, HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		`rech_listen_tcp_secure(8443LL, "cert.pem", "key.pem")`,
		"#include <openssl/ssl.h>",
		"#include <openssl/err.h>",
		"static rech_socket_t rech_listen_tcp_secure(long long port, const char *certfile, const char *keyfile)",
		"SSL_CTX_use_certificate_file(ctx, certfile, SSL_FILETYPE_PEM)",
		"SSL_CTX_use_PrivateKey_file(ctx, keyfile, SSL_FILETYPE_PEM)",
		"rech_tls_registry",
		"rech_tls_lock",
		"SSL_accept(ssl)",
		"SSL_write(ssl,",
		"SSL_read(ssl,",
		"SSL_shutdown(entry->ssl)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRHTTPServerOverTLSReadsThroughSharedPrimitive - регрессия
// на найденный баг (TLS): "принять_http" читал сырым
// recv() напрямую, минуя rech_recv - для TLS-соединения это читало ещё
// зашифрованные байты, HTTP-парсер ждал "\r\n\r\n" и зависал насмерть
// (найдено сквозным curl-прогоном, не этим тестом - IR здесь строится
// вручную и не гоняет настоящий сокет). Проверяет структурно: и
// "принять_http", и "получить" обязаны идти через ОДИН и тот же
// rech_socket_read_raw, определённый ровно один раз (TLS-осведомлённый
// вариант, tls.go).
func TestGenerateFromIRHTTPServerOverTLSReadsThroughSharedPrimitive(t *testing.T) {
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
			&ir.VarDecl{Name: "л", Value: &ir.CallExpr{
				Callee: "слушать_tcp_защищённо",
				Args:   []ir.Expr{&ir.IntLiteral{Value: 8443}, &ir.StringLiteral{Value: "cert.pem"}, &ir.StringLiteral{Value: "key.pem"}},
				Type:   ir.TypeSocket, HasReturn: true,
			}},
			&ir.VarDecl{Name: "с", Value: &ir.CallExpr{Callee: "принять", Args: []ir.Expr{&ir.Identifier{Name: "л", Type: ir.TypeSocket}}, Type: ir.NullableType(ir.TypeSocket), HasReturn: true}},
			&ir.VarDecl{Name: "р", Value: &ir.CallExpr{
				Callee: "принять_http", Args: []ir.Expr{&ir.NullUnwrapExpr{Value: &ir.Identifier{Name: "с", Type: ir.NullableType(ir.TypeSocket)}, Type: ir.TypeSocket}},
				Type: ir.EntityType("Запрос"), HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Count(out, "static long long rech_socket_read_raw(") != 1 {
		t.Fatalf("rech_socket_read_raw должен быть определён ровно один раз, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_socket_read_raw(s, buf + len,") {
		t.Fatalf("rech_recv_http_request должен читать через rech_socket_read_raw, получено:\n%s", out)
	}
	if strings.Contains(out, "recv(s, buf + len,") {
		t.Fatalf("rech_recv_http_request не должен звать recv() напрямую (обходит TLS), получено:\n%s", out)
	}
}

// TestGenerateFromIRPlainSocketsOmitTLS - программа без "слушать_tcp_
// защищённо" - обычный socketHelpers (без TLS): ни OpenSSL, ни реестра
// TLS-соединений в выводе быть не должно - регрессия на "TLS всегда
// эмитируется вместе с обычными сокетами".
func TestGenerateFromIRPlainSocketsOmitTLS(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "слушатель", Value: &ir.CallExpr{
			Callee: "сетевой_узел",
			Args:   []ir.Expr{&ir.StringLiteral{Value: "tcp"}, &ir.StringLiteral{Value: ""}, &ir.IntLiteral{Value: 8080}},
			Type:   ir.TypeSocket, HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, notWant := range []string{"openssl", "SSL_CTX", "rech_tls_registry", "rech_listen_tcp_secure"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("программа без TLS не должна содержать %q, получено:\n%s", notWant, out)
		}
	}
}
