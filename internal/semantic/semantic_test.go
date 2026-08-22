package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

func parseProgram(t *testing.T, src string) *ir.Program {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	irProg, err := Analyze(prog)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return irProg
}

func TestAnalyzePrintString(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    вывести "Привет, мир!"
}
`)
	if len(irProg.Functions) != 1 {
		t.Fatalf("ожидалась 1 функция, получено %d", len(irProg.Functions))
	}
	fn := irProg.Functions[0]
	if fn.Name != "главная" {
		t.Fatalf("ожидалось имя \"главная\", получено %q", fn.Name)
	}
	if len(fn.Body) != 1 {
		t.Fatalf("ожидалась 1 инструкция, получено %d", len(fn.Body))
	}
	print, ok := fn.Body[0].(*ir.PrintStmt)
	if !ok {
		t.Fatalf("ожидался *ir.PrintStmt, получено %T", fn.Body[0])
	}
	str, ok := print.Value.(*ir.StringLiteral)
	if !ok || str.Value != "Привет, мир!" {
		t.Fatalf("ожидался строковый литерал \"Привет, мир!\", получено %+v", print.Value)
	}
}

func TestAnalyzeVarDeclAndArithmeticPromotion(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 10
    переменная б = 2.5
    вывести а + б
}
`)
	fn := irProg.Functions[0]
	varDecl, ok := fn.Body[0].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался *ir.VarDecl, получено %T", fn.Body[0])
	}
	if ir.TypeOf(varDecl.Value) != ir.TypeInt {
		t.Fatalf("ожидался Целое, получено %v", ir.TypeOf(varDecl.Value))
	}
	print := fn.Body[2].(*ir.PrintStmt)
	sum, ok := print.Value.(*ir.BinaryExpr)
	if !ok || sum.Type != ir.TypeFloat {
		t.Fatalf("ожидалось Число (Целое+Число продвигается) у суммы, получено %+v", print.Value)
	}
}

func TestAnalyzeStringConcat(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная имя = "Илья"
    вывести "Привет, " + имя
}
`)
	print := irProg.Functions[0].Body[1].(*ir.PrintStmt)
	concat, ok := print.Value.(*ir.BinaryExpr)
	if !ok || concat.Type != ir.TypeString || concat.Op != "+" {
		t.Fatalf("ожидалась конкатенация строк, получено %+v", print.Value)
	}
}

func TestAnalyzeReassignTypeMismatchFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная x = 1
    x = "текст"
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "нельзя присвоить значение типа") {
		t.Fatalf("ожидалась ошибка про несовпадение типа присваивания, получено: %v", err)
	}
}

func TestAnalyzeUndeclaredVariableFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести x
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "не объявлена") {
		t.Fatalf("ожидалась ошибка про необъявленную переменную, получено: %v", err)
	}
}

func TestAnalyzeLogicalAndComparison(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 10
    переменная б = 3
    вывести (а > б) и не (а == б)
}
`)
	print := irProg.Functions[0].Body[2].(*ir.PrintStmt)
	and, ok := print.Value.(*ir.BinaryExpr)
	if !ok || and.Op != "и" || and.Type != ir.TypeBool {
		t.Fatalf("ожидалось логическое И, получено %+v", print.Value)
	}
	if _, ok := and.Left.(*ir.BinaryExpr); !ok {
		t.Fatalf("ожидалось сравнение слева от \"и\", получено %+v", and.Left)
	}
	not, ok := and.Right.(*ir.UnaryExpr)
	if !ok || not.Op != "не" {
		t.Fatalf("ожидалось унарное \"не\" справа от \"и\", получено %+v", and.Right)
	}
}

func TestAnalyzeUnsupportedStmtFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    если истина
    {
        вывести "да"
    }
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "не реализована в IR") {
		t.Fatalf("ожидалась ошибка про неподдержанную инструкцию (шаг 2, если - ещё не перенесён), получено: %v", err)
	}
}
