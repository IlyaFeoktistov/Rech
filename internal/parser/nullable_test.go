package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseVarDeclWithNullableAnnotation(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl, ok := prog.Functions[0].Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", prog.Functions[0].Body[0])
	}
	if decl.Type != "?Целое" {
		t.Fatalf("ожидалась аннотация \"?Целое\", получено %q", decl.Type)
	}
	if _, ok := decl.Value.(*ast.NullLiteral); !ok {
		t.Fatalf("ожидался NullLiteral, получено %+v", decl.Value)
	}
}

func TestParseVarDeclWithoutAnnotationStillWorks(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а = 5
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl, ok := prog.Functions[0].Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", prog.Functions[0].Body[0])
	}
	if decl.Type != "" {
		t.Fatalf("ожидалась пустая аннотация, получено %q", decl.Type)
	}
}

// TestParseBangDesugarsToEqualNichto - "!x" (НАЙДЕННЫЙ по ходу
// examples/проекты/чат_сервер запрос пользователя) -
// лексер/парсер производят ТОТ ЖЕ AST, что и явное "x == Ничто" -
// BinaryExpr("==", x, NullLiteral), не отдельный узел - вся семантика
// (internal/semantic/nullable.go, analyzeNullCheck) переиспользуется
// бесплатно.
func TestParseBangDesugarsToEqualNichto(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    если !а {
        вернуть
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ifStmt, ok := prog.Functions[0].Body[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("ожидался IfStmt, получено %T", prog.Functions[0].Body[0])
	}
	cond, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("ожидался BinaryExpr, получено %+v", ifStmt.Cond)
	}
	if cond.Op != "==" {
		t.Fatalf("ожидался оператор \"==\", получено %q", cond.Op)
	}
	if _, ok := cond.Right.(*ast.NullLiteral); !ok {
		t.Fatalf("ожидался NullLiteral справа, получено %+v", cond.Right)
	}
	ident, ok := cond.Left.(*ast.Identifier)
	if !ok || ident.Name != "а" {
		t.Fatalf("ожидался Identifier \"а\" слева, получено %+v", cond.Left)
	}
}

func TestParseNullCoalesce(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    вывести а ?? 0
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	printStmt, ok := prog.Functions[0].Body[1].(*ast.PrintStmt)
	if !ok {
		t.Fatalf("ожидался PrintStmt, получено %T", prog.Functions[0].Body[1])
	}
	bin, ok := printStmt.Value.(*ast.BinaryExpr)
	if !ok || bin.Op != "??" {
		t.Fatalf("ожидался BinaryExpr с оператором \"??\", получено %+v", printStmt.Value)
	}
}

func TestParseNullCoalesceAssign(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а ?= 5
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[1].(*ast.NullCoalesceAssign)
	if !ok {
		t.Fatalf("ожидался NullCoalesceAssign, получено %T", prog.Functions[0].Body[1])
	}
	if stmt.Name != "а" {
		t.Fatalf("ожидалось имя \"а\", получено %q", stmt.Name)
	}
}

func TestParseNullableEntityTypeName(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь {
    имеет:
        Строка имя
}
функция найти(): ?Пользователь {
    вернуть Ничто
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Functions[0].ReturnType != "?Пользователь" {
		t.Fatalf("ожидался тип результата \"?Пользователь\", получено %q", prog.Functions[0].ReturnType)
	}
}
