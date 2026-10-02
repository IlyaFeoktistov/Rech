package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestAnalyzeSwitchStmtDesugarsToBlockWithVarDeclAndIfChain - "выбрать"
// десахарируется в ir.BlockStmt{VarDecl, IfStmt-цепочка} - см. package doc
// analyzeSwitchStmt.
func TestAnalyzeSwitchStmtDesugarsToBlockWithVarDeclAndIfChain(t *testing.T) {
	prog := parseProgram(t, `
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
	fn := findFunc(prog, "главная")
	if len(fn.Body) != 2 {
		t.Fatalf("ожидалось 2 инструкции, получено %d", len(fn.Body))
	}
	block, ok := fn.Body[1].(*ir.BlockStmt)
	if !ok {
		t.Fatalf("ожидался BlockStmt, получено %T", fn.Body[1])
	}
	if len(block.Body) != 2 {
		t.Fatalf("ожидалось 2 инструкции внутри BlockStmt (VarDecl + цепочка если), получено %d", len(block.Body))
	}
	if _, ok := block.Body[0].(*ir.VarDecl); !ok {
		t.Fatalf("ожидался VarDecl первой инструкцией, получено %T", block.Body[0])
	}
	outer, ok := block.Body[1].(*ir.IfStmt)
	if !ok {
		t.Fatalf("ожидался IfStmt, получено %T", block.Body[1])
	}
	if _, ok := outer.Cond.(*ir.BinaryExpr); !ok {
		t.Fatalf("ожидался BinaryExpr (\"==\") условием первой ветки, получено %T", outer.Cond)
	}
	if len(outer.Else) != 1 {
		t.Fatalf("ожидалась ровно одна инструкция в \"иначе\" (вложенный если следующей ветки), получено %d", len(outer.Else))
	}
	second, ok := outer.Else[0].(*ir.IfStmt)
	if !ok {
		t.Fatalf("ожидался вложенный IfStmt второй ветки, получено %T", outer.Else[0])
	}
	// "2|3" - Cond второй ветки должен быть "или" двух "==".
	cond, ok := second.Cond.(*ir.BinaryExpr)
	if !ok || cond.Op != "или" {
		t.Fatalf("ожидался BinaryExpr(\"или\") для \"2|3\", получено %+v", second.Cond)
	}
	if len(second.Else) != 1 {
		t.Fatalf("ожидалась ветка \"иначе\" самого выбора на дне цепочки, получено %d инструкций", len(second.Else))
	}
}

// TestAnalyzeSwitchStmtTypePatternBuildsIsExpr - голое имя сущности в
// "когда" сопоставляется через "относится к" (ir.IsExpr), не через "==".
func TestAnalyzeSwitchStmtTypePatternBuildsIsExpr(t *testing.T) {
	prog := parseProgram(t, `
сущность Пользователь
{
    имеет:
        Строка имя
    конструктор(имя: Строка) { этот.имя = имя }
}
сущность Администратор наследует Пользователя
{
    конструктор(имя: Строка) { этот.имя = имя }
}
функция обработать(п: Пользователь)
{
    выбрать п
    {
        когда Администратор:
            вывести "администратор"
        когда Пользователь:
            вывести "пользователь"
    }
}
`)
	fn := findFunc(prog, "обработать")
	block := fn.Body[0].(*ir.BlockStmt)
	outer := block.Body[1].(*ir.IfStmt)
	isExpr, ok := outer.Cond.(*ir.IsExpr)
	if !ok {
		t.Fatalf("ожидался IsExpr для типового паттерна, получено %T", outer.Cond)
	}
	if isExpr.Target != "Администратор" {
		t.Fatalf("ожидался Target \"Администратор\", получено %q", isExpr.Target)
	}
}

// TestAnalyzeSwitchStmtValueNamedAfterVariableStaysEquality - имя, СОВПАДАЮЩЕЕ
// с уже объявленной в этой области видимости переменной, всегда значение
// ("=="), даже если так же называется тип - переменная побеждает.
func TestAnalyzeSwitchStmtValueNamedAfterVariableStaysEquality(t *testing.T) {
	prog := parseProgram(t, `
функция главная
{
    переменная а = 1
    переменная б = 1
    выбрать а
    {
        когда б:
            вывести "равно"
    }
}
`)
	fn := findFunc(prog, "главная")
	block := fn.Body[2].(*ir.BlockStmt)
	outer := block.Body[1].(*ir.IfStmt)
	if _, ok := outer.Cond.(*ir.BinaryExpr); !ok {
		t.Fatalf("ожидался BinaryExpr (\"==\") при сравнении с переменной, получено %T", outer.Cond)
	}
}

// TestAnalyzeSwitchStmtNullableSubjectFails - нельзя выбирать напрямую по
// nullable-значению, тем же принципом, что и "вывести"/"показать".
func TestAnalyzeSwitchStmtNullableSubjectFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    ?Целое а = Ничто
    выбрать а
    {
        когда 1:
            вывести "один"
    }
}
`)
	if err == nil || !strings.Contains(err.Error(), "nullable") {
		t.Fatalf("ожидалась ошибка про nullable-тип, получено: %v", err)
	}
}

// TestAnalyzeSwitchStmtUnrelatedTypePatternFails - типовой паттерн "когда"
// проверяется той же логикой, что и "относится к" (analyzeIsExpr) -
// несвязанные типы не проходят компиляцию.
func TestAnalyzeSwitchStmtUnrelatedTypePatternFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        Целое x = 0
}
функция главная
{
    переменная т = новая Точка()
    выбрать т
    {
        когда Исключение:
            вывести "да"
    }
}
`)
	if err == nil || !strings.Contains(err.Error(), "не связаны наследованием/интерфейсом") {
		t.Fatalf("ожидалась ошибка о несвязанных типах, получено: %v", err)
	}
}
