package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseParseExprJSON(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная текст = "{}"
    переменная п = разобрать JSON текст как Пользователь
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[0].Body[1].(*ast.VarDecl)
	pe, ok := stmt.Value.(*ast.ParseExpr)
	if !ok {
		t.Fatalf("ожидался *ast.ParseExpr, получено %T", stmt.Value)
	}
	if pe.Format != "JSON" {
		t.Fatalf("ожидался формат JSON, получено %q", pe.Format)
	}
	if pe.TypeName != "Пользователь" {
		t.Fatalf("ожидался целевой тип Пользователь, получено %q", pe.TypeName)
	}
	if _, ok := pe.Source.(*ast.Identifier); !ok {
		t.Fatalf("ожидался источник-идентификатор, получено %T", pe.Source)
	}
}

func TestParseParseExprGenericTarget(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная п = разобрать JSON "[]" как Список<Пользователь>
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[0].Body[0].(*ast.VarDecl)
	pe := stmt.Value.(*ast.ParseExpr)
	if pe.TypeName != "Список<Пользователь>" {
		t.Fatalf("ожидался целевой тип Список<Пользователь>, получено %q", pe.TypeName)
	}
}

func TestParseParseExprRequiresAs(t *testing.T) {
	_, err := parseSource(t, `
функция главная
{
    переменная п = разобрать JSON "{}" Пользователь
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка разбора (отсутствует \"как\")")
	}
}
