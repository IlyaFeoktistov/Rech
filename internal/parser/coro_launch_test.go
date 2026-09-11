package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseCoroLaunchExpr(t *testing.T) {
	prog, err := parseSource(t, `
асинхронная функция обслужить(): Целое
{
    вернуть 1
}
функция главная
{
    переменная задача = поручить обслужить()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	main := prog.Functions[1]
	decl, ok := main.Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", main.Body[0])
	}
	launch, ok := decl.Value.(*ast.CoroLaunchExpr)
	if !ok {
		t.Fatalf("ожидался CoroLaunchExpr, получено %T", decl.Value)
	}
	if launch.Call.Callee != "обслужить" {
		t.Fatalf("ожидался callee \"обслужить\", получено %q", launch.Call.Callee)
	}
}

func TestParseCoroLaunchRequiresCallForm(t *testing.T) {
	_, err := parseSource(t, `
функция главная
{
    переменная х = поручить 5
}
`)
	if err == nil || !strings.Contains(err.Error(), "поручить") {
		t.Fatalf("ожидалась ошибка о форме \"поручить <имя>(...)\", получено: %v", err)
	}
}
