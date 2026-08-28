package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseAddressOfVariable(t *testing.T) {
	src := "функция главная { переменная а = 5\nвывести &а }\n"
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[0].Body[1].(*ast.PrintStmt)
	if _, ok := stmt.Value.(*ast.FuncRefExpr); !ok {
		t.Fatalf("ожидался FuncRefExpr (неоднозначность разрешается семантикой), получено %#v", stmt.Value)
	}
}

func TestParseAddressOfField(t *testing.T) {
	src := "" +
		"сущность Точка\n{\n    имеет:\n        x: Целое\n}\n" +
		"функция главная { переменная т = новая Точка(1)\nвывести &т.x }\n"
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[0].Body[1].(*ast.PrintStmt)
	addr, ok := stmt.Value.(*ast.AddressOfExpr)
	if !ok {
		t.Fatalf("ожидался AddressOfExpr, получено %#v", stmt.Value)
	}
	if addr.Receiver != "т" || addr.Field != "x" || addr.Index != nil {
		t.Fatalf("неожиданный AddressOfExpr: %#v", addr)
	}
}

func TestParseAddressOfIndex(t *testing.T) {
	src := "функция главная { переменная сп = новый Список<Целое>()\nвывести &сп[0] }\n"
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[0].Body[1].(*ast.PrintStmt)
	addr, ok := stmt.Value.(*ast.AddressOfExpr)
	if !ok {
		t.Fatalf("ожидался AddressOfExpr, получено %#v", stmt.Value)
	}
	if addr.Receiver != "сп" || addr.Field != "" || addr.Index == nil {
		t.Fatalf("неожиданный AddressOfExpr: %#v", addr)
	}
}

func TestParseDerefExpr(t *testing.T) {
	src := "функция главная { переменная п: Указатель<Целое> = &а\nвывести *п }\n"
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[0].Body[1].(*ast.PrintStmt)
	if _, ok := stmt.Value.(*ast.DerefExpr); !ok {
		t.Fatalf("ожидался DerefExpr, получено %#v", stmt.Value)
	}
}

func TestParseDoubleDerefExpr(t *testing.T) {
	src := "функция главная { вывести **пп }\n"
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt := prog.Functions[0].Body[0].(*ast.PrintStmt)
	outer, ok := stmt.Value.(*ast.DerefExpr)
	if !ok {
		t.Fatalf("ожидался внешний DerefExpr, получено %#v", stmt.Value)
	}
	if _, ok := outer.Value.(*ast.DerefExpr); !ok {
		t.Fatalf("ожидался вложенный DerefExpr, получено %#v", outer.Value)
	}
}

func TestParseDerefAssignStmt(t *testing.T) {
	src := "функция главная { *п = 5 }\n"
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[0].(*ast.DerefAssignStmt)
	if !ok {
		t.Fatalf("ожидался DerefAssignStmt, получено %#v", prog.Functions[0].Body[0])
	}
	ident, ok := stmt.Pointer.(*ast.Identifier)
	if !ok || ident.Name != "п" {
		t.Fatalf("ожидался Identifier(\"п\") в качестве Pointer, получено %#v", stmt.Pointer)
	}
}

func TestParseDoubleDerefAssignStmt(t *testing.T) {
	src := "функция главная { **пп = 5 }\n"
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[0].(*ast.DerefAssignStmt)
	if !ok {
		t.Fatalf("ожидался DerefAssignStmt, получено %#v", prog.Functions[0].Body[0])
	}
	if _, ok := stmt.Pointer.(*ast.DerefExpr); !ok {
		t.Fatalf("ожидался DerefExpr в качестве Pointer (запись через один уровень разыменования), получено %#v", stmt.Pointer)
	}
}
