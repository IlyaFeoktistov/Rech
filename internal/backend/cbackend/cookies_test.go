package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func cookieTestEntities() []*ir.Entity {
	return []*ir.Entity{
		{Name: "Запрос", IsStruct: true, Fields: []ir.Field{
			{Name: "метод", Type: ir.TypeString}, {Name: "адрес", Type: ir.TypeString},
			{Name: "порт", Type: ir.TypeInt}, {Name: "путь", Type: ir.TypeString},
			{Name: "заголовки", Type: ir.TypeString}, {Name: "тело", Type: ir.TypeString},
		}},
		{Name: "ОпцииКуки", IsStruct: true, Fields: cookieOptionsFieldsForTest()},
	}
}

// cookieOptionsFieldsForTest - те же поля, что internal/semantic/
// cookieOptionsFields (пакет cbackend не импортирует internal/semantic) -
// достаточно для проверки кодогенерации напрямую по IR, без семантического
// анализа.
func cookieOptionsFieldsForTest() []ir.Field {
	return []ir.Field{
		{Name: "путь", Type: ir.TypeString},
		{Name: "домен", Type: ir.TypeString},
		{Name: "максимальный_возраст", Type: ir.NullableType(ir.TypeInt)},
		{Name: "только_http", Type: ir.TypeBool},
		{Name: "безопасный", Type: ir.TypeBool},
		{Name: "тот_же_сайт", Type: ir.TypeString},
	}
}

func TestGenerateFromIRCookieBuiltinCalls(t *testing.T) {
	prog := &ir.Program{
		Entities: cookieTestEntities(),
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "запрос", Value: &ir.NewExpr{Entity: "Запрос", Args: []ir.Expr{
				&ir.StringLiteral{Value: "GET"}, &ir.StringLiteral{Value: ""}, &ir.IntLiteral{Value: 0},
				&ir.StringLiteral{Value: "/"}, &ir.StringLiteral{Value: "Cookie: a=1"}, &ir.StringLiteral{Value: ""},
			}}},
			&ir.VarDecl{Name: "имя", Value: &ir.CallExpr{
				Callee: "значение_куки",
				Args:   []ir.Expr{&ir.Identifier{Name: "запрос", Type: ir.EntityType("Запрос")}, &ir.StringLiteral{Value: "a"}},
				Type:   ir.NullableType(ir.TypeString), HasReturn: true,
			}},
			&ir.VarDecl{Name: "заголовок", Value: &ir.CallExpr{
				Callee: "строка_куки",
				Args: []ir.Expr{
					&ir.StringLiteral{Value: "a"}, &ir.StringLiteral{Value: "1"},
					&ir.NewExpr{Entity: "ОпцииКуки", Args: []ir.Expr{
						&ir.StringLiteral{Value: "/"}, &ir.StringLiteral{Value: ""}, &ir.NullLiteral{Type: ir.NullableType(ir.TypeInt)},
						&ir.BoolLiteral{Value: true}, &ir.BoolLiteral{Value: false}, &ir.StringLiteral{Value: ""},
					}},
				},
				Type: ir.TypeString, HasReturn: true,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"static rech_T_n_stroka rech_cookie_value(const char *headers, const char *name) {",
		"static char *rech_cookie_string(const char *name, const char *value, rech_T_opciikuki opts) {",
		"rech_cookie_value((v_zapros).f_zagolovki, \"a\")",
		"rech_cookie_string(\"a\", \"1\",",
		"static char *rech_str_concat(const char *a, const char *b) {",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "#include <string.h>") || !strings.Contains(out, "#include <stdlib.h>") {
		t.Fatalf("значение_куки/строка_куки используют strlen/strstr/malloc - ожидались string.h/stdlib.h, получено:\n%s", out)
	}
}

func TestGenerateFromIRWithoutCookiesOmitsCookieRuntime(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "привет"}}}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, unwanted := range []string{"rech_cookie_value", "rech_cookie_string", "rech_str_concat"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("программа без cookies не должна содержать %q, получено:\n%s", unwanted, out)
		}
	}
}
