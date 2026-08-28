package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseListLiteralEmpty(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а: Список<Целое> = []
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	lit, ok := decl.Value.(*ast.ListLiteral)
	if !ok || len(lit.Elems) != 0 {
		t.Fatalf("ожидался пустой ListLiteral, получено %+v", decl.Value)
	}
}

func TestParseListLiteralNonEmpty(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = [1, 2, 3]
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	lit, ok := decl.Value.(*ast.ListLiteral)
	if !ok || len(lit.Elems) != 3 {
		t.Fatalf("ожидался ListLiteral из 3 элементов, получено %+v", decl.Value)
	}
}
