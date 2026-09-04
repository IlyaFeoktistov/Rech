package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseTypedVarDeclSimple(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    Целое x = 5 + 3
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	if decl.Name != "x" || decl.Type != "Целое" {
		t.Fatalf("ожидался VarDecl{Name: x, Type: Целое}, получено %+v", decl)
	}
}

func TestParseTypedVarDeclGeneric(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    Список<Целое> числа = [1, 2, 3]
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	if decl.Name != "числа" || decl.Type != "Список<Целое>" {
		t.Fatalf("ожидался VarDecl{Name: числа, Type: Список<Целое>}, получено %+v", decl)
	}
	if _, ok := decl.Value.(*ast.ListLiteral); !ok {
		t.Fatalf("ожидался ListLiteral, получено %+v", decl.Value)
	}
}

func TestParseTypedVarDeclNullable(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    ?Целое опция = Ничто
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl := prog.Functions[0].Body[0].(*ast.VarDecl)
	if decl.Name != "опция" || decl.Type != "?Целое" {
		t.Fatalf("ожидался VarDecl{Name: опция, Type: ?Целое}, получено %+v", decl)
	}
}

// TestParseTypedVarDeclDoesNotBreakPlainAssign - "x = 5" (без типа) должно
// по-прежнему разбираться как обычное присваивание, не как объявление
// переменной с типом "x" - tryParseTypedVarDecl требует ДВУХ IDENT перед
// "=" (тип и имя), у присваивания только один.
func TestParseTypedVarDeclDoesNotBreakPlainAssign(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная x = 1
    x = 5
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if _, ok := prog.Functions[0].Body[1].(*ast.Assign); !ok {
		t.Fatalf("ожидался ast.Assign, получено %+v", prog.Functions[0].Body[1])
	}
}

// TestParseTypedVarDeclDoesNotBreakDerefAssign - "*p = значение" должно
// по-прежнему разбираться как разыменование указателя, не как объявление
// переменной типа "Указатель<p>" - между "*" и "=" ровно один IDENT
// (значит это разыменование, tryParseTypedVarDecl требует ДВУХ).
func TestParseTypedVarDeclDoesNotBreakDerefAssign(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    Указатель<Целое> p = &x
    *p = 5
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if _, ok := prog.Functions[0].Body[1].(*ast.DerefAssignStmt); !ok {
		t.Fatalf("ожидался ast.DerefAssignStmt, получено %+v", prog.Functions[0].Body[1])
	}
}
