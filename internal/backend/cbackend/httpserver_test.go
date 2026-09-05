package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRHTTPServerBuiltinCalls(t *testing.T) {
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
			&ir.VarDecl{Name: "л", Value: &ir.CallExpr{Callee: "слушать_tcp", Args: []ir.Expr{&ir.IntLiteral{Value: 8080}}, Type: ir.TypeSocket, HasReturn: true}},
			&ir.VarDecl{Name: "с", Value: &ir.CallExpr{Callee: "принять", Args: []ir.Expr{&ir.Identifier{Name: "л", Type: ir.TypeSocket}}, Type: ir.NullableType(ir.TypeSocket), HasReturn: true}},
			&ir.VarDecl{Name: "р", Value: &ir.CallExpr{
				Callee: "принять_http", Args: []ir.Expr{&ir.NullUnwrapExpr{Value: &ir.Identifier{Name: "с", Type: ir.NullableType(ir.TypeSocket)}, Type: ir.TypeSocket}},
				Type: ir.EntityType("Запрос"), HasReturn: true,
			}},
			&ir.ExprStmt{Call: &ir.CallExpr{
				Callee: "отправить_http_ответ",
				Args: []ir.Expr{
					&ir.NullUnwrapExpr{Value: &ir.Identifier{Name: "с", Type: ir.NullableType(ir.TypeSocket)}, Type: ir.TypeSocket},
					&ir.NewExpr{Entity: "Ответ", Args: []ir.Expr{&ir.IntLiteral{Value: 200}, &ir.StringLiteral{Value: ""}, &ir.StringLiteral{Value: "ок"}}},
				},
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_http_accept_request(rech_T_n_soket_unwrap(v_s))",
		"rech_http_send_response(rech_T_n_soket_unwrap(v_s),",
		"static rech_T_zapros rech_http_accept_request(rech_socket_t s)",
		"static void rech_http_send_response(rech_socket_t s, rech_T_otvet resp)",
		"static char *rech_dup(const char *s)",
		"static long long rech_http_content_length(",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
	if strings.Count(out, "static char *rech_dup(const char *s)") != 1 {
		t.Fatalf("rech_dup должен быть определён ровно один раз, получено:\n%s", out)
	}
}

// TestGenerateFromIRHTTPServerDecodesPathSegment - НАЙДЕННЫЙ В ROADMAP
// (шаг 41) пробел: "принять_http" обязан percent-декодировать сегмент пути
// (rech_path_url_decode), а не только query-строку (rech_qp_url_decode,
// отдельная функция - у сегмента пути нет "+"-как-пробел).
func TestGenerateFromIRHTTPServerDecodesPathSegment(t *testing.T) {
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
			&ir.VarDecl{Name: "с", Value: &ir.CallExpr{Callee: "принять_http", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.EntityType("Запрос"), HasReturn: true}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static char *rech_path_url_decode(") {
		t.Fatalf("ожидалось определение rech_path_url_decode, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_path_url_decode(raw_path, seg_len)") {
		t.Fatalf("ожидался вызов rech_path_url_decode при разборе пути, получено:\n%s", out)
	}
}

func TestGenerateFromIRHTTPClientAndServerShareRechDup(t *testing.T) {
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
			&ir.VarDecl{Name: "с", Value: &ir.CallExpr{Callee: "принять_http", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.EntityType("Запрос"), HasReturn: true}},
			&ir.VarDecl{Name: "о", Value: &ir.CallExpr{
				Callee: "запрос", Args: []ir.Expr{&ir.Identifier{Name: "с", Type: ir.EntityType("Запрос")}},
				Type: ir.EntityType("Ответ"), HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if n := strings.Count(out, "static char *rech_dup(const char *s)"); n != 1 {
		t.Fatalf("rech_dup должен быть определён ровно один раз при одновременном использовании клиента и сервера, получено %d раз:\n%s", n, out)
	}
}

func TestGenerateFromIRQueryParamsBuiltinCall(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "п", Value: &ir.CallExpr{
				Callee: "параметры_пути", Args: []ir.Expr{&ir.StringLiteral{Value: "/ping?a=1"}},
				Type: ir.EntityType("Словарь<Строка, Строка>"), HasReturn: true,
			}},
		}}},
		Generics: []ir.GenericInstantiation{{Name: "Словарь<Строка, Строка>", Template: "Словарь", KeyType: ir.TypeString, ElemType: ir.TypeString}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_query_params(",
		// Указатель, не значение - НАЙДЕННЫЙ И ИСПРАВЛЕННЫЙ баг: раньше
		// rech_query_params возвращал Словарь по значению, а вызывающий код
		// (VarDecl выше, "rech_T_... * v_п_INIT = rech_query_params(...)")
		// ждёт указатель - несовпадение давало "incompatible types" у
		// настоящего cc (generic-коллекции ссылочные ВЕЗДЕ в cbackend, см.
		// entityIsRef/genericCtorCCode).
		"static " + mangleType("Словарь<Строка, Строка>") + " *rech_query_params(const char *path)",
		"static char *rech_qp_url_decode(",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "* v_p_INIT = rech_query_params(") {
		t.Fatalf("вызывающий код должен присваивать указатель, получено:\n%s", out)
	}
}

func TestGenerateFromIRWithoutQueryParamsOmitsDictType(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_query_params") || strings.Contains(out, mangleType("Словарь<Строка, Строка>")) {
		t.Fatalf("не ожидался Словарь<Строка, Строка> без \"параметры_пути\", получено:\n%s", out)
	}
}

func TestGenerateFromIRWithoutHTTPServerOmitsHelpers(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_http_accept_request") {
		t.Fatalf("не ожидался HTTP-сервер рантайм без принять_http/отправить_http_ответ, получено:\n%s", out)
	}
}

// TestGenerateFromIRHTTPResponseAnnouncesKeepAlive - "отправить_http_
// ответ" шлёт "Connection: keep-alive", не "close" (docs/ROADMAP.md, "HTTP
// keep-alive") - сервер всегда ГОТОВ обслужить следующий запрос на этом же
// соединении, решение продолжать ли цикл - за вызывающим Rech-кодом (см.
// examples/проекты/http_сервер).
func TestGenerateFromIRHTTPResponseAnnouncesKeepAlive(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная л = сетевой_узел("tcp", "", 8080)
    переменная клиент = принять(л)
    если клиент != Ничто
    {
        переменная запрос = принять_http(клиент)
        отправить_http_ответ(клиент, новый Ответ(200, "", "ок"))
    }
}
`)
	if !strings.Contains(out, "Connection: keep-alive") {
		t.Fatalf("ожидался \"Connection: keep-alive\" в rech_http_send_response, получено:\n%s", out)
	}
	if strings.Contains(out, "Connection: close") {
		t.Fatalf("не ожидался \"Connection: close\" в rech_http_send_response, получено:\n%s", out)
	}
}

// TestGenerateFromIRSocketCreationIgnoresSigpipe - НАЙДЕННЫЙ ЖИВЬЁМ баг
// (docs/ROADMAP.md, "HTTP keep-alive"): запись в сокет, который ПИР уже
// закрыл (типичное HTTP keep-alive соединение, где клиент отключился между
// запросами), убивала ВЕСЬ процесс через SIGPIPE (POSIX-умолчание) - без
// единой строчки паники, silent crash. Каждая функция создания сокета
// теперь игнорирует SIGPIPE (идемпотентно, тем же приёмом, что и
// rech_shutdown_requested, shutdown.go).
func TestGenerateFromIRSocketCreationIgnoresSigpipe(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная л = сетевой_узел("tcp", "", 8080)
    закрыть_сокет(л)
}
`)
	if !strings.Contains(out, "signal(SIGPIPE, SIG_IGN)") {
		t.Fatalf("ожидался \"signal(SIGPIPE, SIG_IGN)\" в рантайме сокетов, получено:\n%s", out)
	}
	if !strings.Contains(out, "#include <signal.h>") {
		t.Fatalf("ожидался #include <signal.h> при использовании сокетов, получено:\n%s", out)
	}
}
