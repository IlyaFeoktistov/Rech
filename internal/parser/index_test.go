package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseIndexExprRead(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = 0
    вывести а[0]
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	printStmt, ok := prog.Functions[0].Body[1].(*ast.PrintStmt)
	if !ok {
		t.Fatalf("ожидался PrintStmt, получено %T", prog.Functions[0].Body[1])
	}
	idx, ok := printStmt.Value.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("ожидался IndexExpr, получено %T", printStmt.Value)
	}
	if id, ok := idx.Receiver.(*ast.Identifier); !ok || id.Name != "а" {
		t.Fatalf("ожидался получатель \"а\", получено %+v", idx.Receiver)
	}
}

// TestParseIndexAssignDesugarsToMethodCall - "а[0] = 42" разбирается сразу
// в MethodCallExpr{Method: "установить"} - ast.IndexExpr для формы записи
// не используется вовсе (см. package doc parseIndexAssign).
func TestParseIndexAssignDesugarsToMethodCall(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = 0
    а[0] = 42
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[1].(*ast.ExprStmt)
	if !ok || stmt.MethodCall == nil {
		t.Fatalf("ожидался ExprStmt с MethodCall, получено %+v", prog.Functions[0].Body[1])
	}
	if stmt.MethodCall.Method != "установить" {
		t.Fatalf("ожидался метод \"установить\", получено %q", stmt.MethodCall.Method)
	}
	if len(stmt.MethodCall.Args) != 2 {
		t.Fatalf("ожидалось 2 аргумента (индекс, значение), получено %d", len(stmt.MethodCall.Args))
	}
}
