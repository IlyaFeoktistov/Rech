package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseSwitchStmt(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная число = 2
    выбрать число
    {
        когда 1:
            вывести "один"
        когда 2|3:
            вывести "два или три"
        иначе:
            вывести "другое"
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if len(fn.Body) != 2 {
		t.Fatalf("ожидалось 2 инструкции в теле главная, получено %d", len(fn.Body))
	}
	sw, ok := fn.Body[1].(*ast.SwitchStmt)
	if !ok {
		t.Fatalf("ожидался SwitchStmt, получено %T", fn.Body[1])
	}
	if _, ok := sw.Subject.(*ast.Identifier); !ok {
		t.Fatalf("ожидался Identifier в Subject, получено %T", sw.Subject)
	}
	if len(sw.Cases) != 2 {
		t.Fatalf("ожидалось 2 ветки \"когда\", получено %d", len(sw.Cases))
	}
	if len(sw.Cases[0].Values) != 1 {
		t.Fatalf("ожидалось 1 значение в первой ветке, получено %d", len(sw.Cases[0].Values))
	}
	if len(sw.Cases[1].Values) != 2 {
		t.Fatalf("ожидалось 2 значения во второй ветке (\"2|3\"), получено %d", len(sw.Cases[1].Values))
	}
	if sw.Default == nil {
		t.Fatalf("ожидалась ветка \"иначе\"")
	}
}

func TestParseSwitchStmtWithoutDefault(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная число = 1
    выбрать число
    {
        когда 1:
            вывести "один"
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	sw := prog.Functions[0].Body[1].(*ast.SwitchStmt)
	if sw.Default != nil {
		t.Fatalf("ожидалось отсутствие ветки \"иначе\", получено %+v", sw.Default)
	}
}

func TestParseSwitchStmtEmptyCaseFails(t *testing.T) {
	_, err := parseSource(t, `
функция главная
{
    переменная число = 1
    выбрать число
    {
        когда 1:
        иначе:
            вывести "x"
    }
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка на пустой ветке \"когда\"")
	}
}

func TestParseSwitchStmtNoCasesFails(t *testing.T) {
	_, err := parseSource(t, `
функция главная
{
    переменная число = 1
    выбрать число
    {
        иначе:
            вывести "x"
    }
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка на \"выбрать\" без единой ветки \"когда\"")
	}
}

func TestParseSwitchStmtDuplicateDefaultFails(t *testing.T) {
	_, err := parseSource(t, `
функция главная
{
    переменная число = 1
    выбрать число
    {
        когда 1:
            вывести "a"
        иначе:
            вывести "b"
        иначе:
            вывести "c"
    }
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка на повторном \"иначе\"")
	}
}

// TestParseSwitchValuesStopsAtBitOr - "|" в "когда" разделяет значения, не
// строит одно побитовое ИЛИ (см. package doc parseSwitchValues) - каждое
// значение остаётся отдельным литералом.
func TestParseSwitchValuesStopsAtBitOr(t *testing.T) {
	prog, err := parseSource(t, `
функция главная
{
    переменная число = 1
    выбрать число
    {
        когда 1|2|3:
            вывести "a"
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	sw := prog.Functions[0].Body[1].(*ast.SwitchStmt)
	if len(sw.Cases[0].Values) != 3 {
		t.Fatalf("ожидалось 3 отдельных значения, получено %d: %+v", len(sw.Cases[0].Values), sw.Cases[0].Values)
	}
	for i, v := range sw.Cases[0].Values {
		lit, ok := v.(*ast.IntLiteral)
		if !ok {
			t.Fatalf("значение %d: ожидался IntLiteral, получено %T", i, v)
		}
		if lit.Value != int64(i+1) {
			t.Fatalf("значение %d: ожидалось %d, получено %d", i, i+1, lit.Value)
		}
	}
}
