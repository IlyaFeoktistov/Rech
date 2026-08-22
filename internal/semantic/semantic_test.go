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

func TestAnalyzeIfWhileForWithBlockScoping(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 7
    если а > 5
    {
        переменная а = 100
        вывести а
    }
    иначе
    {
        вывести а
    }

    переменная сумма = 0
    для i от 1 до 3
    {
        сумма = сумма + i
    }
    вывести сумма
    вернуть
}
`)
	fn := irProg.Functions[0]
	ifStmt, ok := fn.Body[1].(*ir.IfStmt)
	if !ok {
		t.Fatalf("ожидался *ir.IfStmt, получено %T", fn.Body[1])
	}
	if len(ifStmt.Then) != 2 || len(ifStmt.Else) != 1 {
		t.Fatalf("неожиданная форма if: %+v", ifStmt)
	}
	// "переменная а = 100" внутри then затеняет внешнюю "а" - не ошибка.
	innerVarDecl := ifStmt.Then[0].(*ir.VarDecl)
	if innerVarDecl.Name != "а" {
		t.Fatalf("ожидалось затенение имени \"а\" внутри then, получено %+v", innerVarDecl)
	}

	forStmt, ok := fn.Body[3].(*ir.ForStmt)
	if !ok {
		t.Fatalf("ожидался *ir.ForStmt, получено %T", fn.Body[3])
	}
	if forStmt.Var != "i" {
		t.Fatalf("неожиданная переменная цикла: %q", forStmt.Var)
	}

	if _, ok := fn.Body[5].(*ir.ReturnStmt); !ok {
		t.Fatalf("ожидался *ir.ReturnStmt, получено %T", fn.Body[5])
	}
}

func TestAnalyzeVariableNotVisibleOutsideBlock(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    если истина
    {
        переменная x = 1
    }
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
		t.Fatalf("ожидалась ошибка про переменную, не видимую за пределами блока, получено: %v", err)
	}
}

func TestAnalyzeIfConditionMustBeBool(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    если 5
    {
        вывести "x"
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
	if err == nil || !strings.Contains(err.Error(), "должно быть логическим") {
		t.Fatalf("ожидалась ошибка про нелогическое условие, получено: %v", err)
	}
}

func TestAnalyzeFunctionCallWithParamsAndReturn(t *testing.T) {
	irProg := parseProgram(t, `
функция сложить(что: Целое, кем: Целое): Целое
{
    вернуть что + кем
}
функция главная
{
    вывести сложить(2, 3)
}
`)
	sum := irProg.Functions[0]
	if len(sum.Params) != 2 || sum.Params[0].Type != ir.TypeInt || !sum.HasReturn || sum.ReturnType != ir.TypeInt {
		t.Fatalf("неожиданная сигнатура: %+v", sum)
	}
	ret, ok := sum.Body[0].(*ir.ReturnStmt)
	if !ok || ret.Value == nil {
		t.Fatalf("ожидался вернуть <значение>, получено %+v", sum.Body[0])
	}

	main := irProg.Functions[1]
	print := main.Body[0].(*ir.PrintStmt)
	call, ok := print.Value.(*ir.CallExpr)
	if !ok || call.Callee != "сложить" || len(call.Args) != 2 || !call.HasReturn || call.Type != ir.TypeInt {
		t.Fatalf("ожидался вызов \"сложить\" с 2 аргументами, получено %+v", print.Value)
	}
}

func TestAnalyzeMutualRecursionForwardReference(t *testing.T) {
	// "б" вызывается из "а" до того, как объявлена в файле - должно
	// работать благодаря предварительному проходу collectFuncSigs.
	irProg := parseProgram(t, `
функция а(): Целое
{
    вернуть б()
}
функция б(): Целое
{
    вернуть 1
}
функция главная
{
    вывести а()
}
`)
	if len(irProg.Functions) != 3 {
		t.Fatalf("ожидалось 3 функции, получено %d", len(irProg.Functions))
	}
}

func TestAnalyzeCallArgTypeMismatchFails(t *testing.T) {
	toks, err := lexer.New(`
функция что(x: Целое) { вывести x }
функция главная
{
    что("текст")
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
	if err == nil || !strings.Contains(err.Error(), "должен быть типа") {
		t.Fatalf("ожидалась ошибка про несовпадение типа аргумента, получено: %v", err)
	}
}

func TestAnalyzeCallWithoutReturnAsValueFails(t *testing.T) {
	toks, err := lexer.New(`
функция ничего() { вывести "x" }
функция главная
{
    вывести ничего()
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
	if err == nil || !strings.Contains(err.Error(), "нельзя использовать как выражение") {
		t.Fatalf("ожидалась ошибка про функцию без результата как значение, получено: %v", err)
	}
}

func TestAnalyzeMissingFinalReturnFails(t *testing.T) {
	toks, err := lexer.New(`
функция х(): Целое
{
    вывести "не вернуть"
}
функция главная { вывести "ок" }
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "должна заканчиваться") {
		t.Fatalf("ожидалась ошибка про отсутствие \"вернуть <значение>\" в конце, получено: %v", err)
	}
}

func TestAnalyzeUnsupportedStmtFails(t *testing.T) {
	toks, err := lexer.New(`
сущность T {}
функция главная
{
    переменная t = новый T()
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
	if err == nil || !strings.Contains(err.Error(), "не реализован") {
		t.Fatalf("ожидалась ошибка про неподдержанную конструкцию (сущности - шаг 4, ещё не перенесён), получено: %v", err)
	}
}
