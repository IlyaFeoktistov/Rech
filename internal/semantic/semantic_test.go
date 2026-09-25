package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// AnalyzeSource разбирает и анализирует src, возвращая саму ошибку (в
// отличие от parseProgram, который вызывает t.Fatalf на любой ошибке) -
// для тестов, которые проверяют именно текст/факт ошибки Analyze.
func AnalyzeSource(t *testing.T, src string) (*ir.Program, error) {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	return Analyze(prog)
}

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

func TestAnalyzeBitwiseByteResultStaysByte(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = (Байт) 6
    переменная б = (Байт) 3
    вывести а | б
    вывести ~а
}
`)
	fn := irProg.Functions[0]
	orPrint := fn.Body[2].(*ir.PrintStmt)
	or, ok := orPrint.Value.(*ir.BinaryExpr)
	if !ok || or.Type != ir.TypeByte {
		t.Fatalf("ожидался Байт | Байт -> Байт (без роста до Целое), получено %+v", orPrint.Value)
	}
	notPrint := fn.Body[3].(*ir.PrintStmt)
	not, ok := notPrint.Value.(*ir.UnaryExpr)
	if !ok || not.Type != ir.TypeByte {
		t.Fatalf("ожидался ~Байт -> Байт, получено %+v", notPrint.Value)
	}
}

func TestAnalyzeBitwiseMixedByteAndIntFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Байт) 6
    переменная б = 3
    вывести а & б
}
`)
	if err == nil || !strings.Contains(err.Error(), "побитовый оператор") {
		t.Fatalf("ожидалась ошибка про побитовый оператор при смешении Байт и Целое, получено: %v", err)
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

func TestAnalyzeBuiltinFunctionCallResolves(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная x = корень(16.0)
    вывести x
}
`)
	if len(irProg.Functions) != 1 {
		t.Fatalf("ожидалась 1 функция, получено %d", len(irProg.Functions))
	}
}

func TestAnalyzeFileBuiltinsResolve(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная ф = открыть("данные.txt")
    переменная кусок = прочитать(ф, 10)
    вывести кусок
    закрыть(ф)
}
`)
	if len(irProg.Functions) != 1 {
		t.Fatalf("ожидалась 1 функция, получено %d", len(irProg.Functions))
	}
}

func TestAnalyzeFileCallAsExpressionFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная ф = открыть("данные.txt")
    вывести закрыть(ф)
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
		t.Fatalf("ожидалась ошибка про использование функции без результата как выражения, получено: %v", err)
	}
}

func TestAnalyzeBuiltinNameCollisionFails(t *testing.T) {
	toks, err := lexer.New(`
функция корень(x: Целое): Целое
{
    вернуть x
}
функция главная { вывести 1 }
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя встроенной функции") {
		t.Fatalf("ожидалась ошибка про зарезервированное имя, получено: %v", err)
	}
}

// TestAnalyzeExternFuncBuildsIR - Analyze сам по себе успешно строит IR с
// ExternFuncs (заполняет ir.Program.ExternFuncs, регистрирует Alias в
// funcs) - реальный вызов (dlopen/dlsym через purego) находится в
// internal/interp (см. TestRunCallsExternFunc) и в cbackend.GenerateFromIR
// (настоящая линковка), не здесь.
func TestAnalyzeExternFuncBuildsIR(t *testing.T) {
	irProg := parseProgram(t, `
внешняя функция abs как модуль_числа(n: Целое): Целое
функция главная { вывести модуль_числа(0 - 5) }
`)
	if len(irProg.ExternFuncs) != 1 {
		t.Fatalf("ожидался 1 ExternFuncDecl, получено %d", len(irProg.ExternFuncs))
	}
	ext := irProg.ExternFuncs[0]
	if ext.CName != "abs" || ext.Alias != "модуль_числа" {
		t.Fatalf("неожиданный ExternFuncDecl: %+v", ext)
	}
	if !ext.HasReturn || ext.ReturnType != ir.TypeInt || len(ext.Params) != 1 || ext.Params[0] != ir.TypeInt {
		t.Fatalf("неожиданная сигнатура ExternFuncDecl: %+v", ext)
	}
}

func TestAnalyzeEntityConstantResolvesToLiteral(t *testing.T) {
	irProg := parseProgram(t, `
сущность Ответчик
{
	константа ОТВЕТ = 42
}

функция главная
{
	вывести Ответчик.ОТВЕТ
}
`)
	fn := irProg.Functions[0]
	if len(fn.Body) != 1 {
		t.Fatalf("ожидалась 1 инструкция, получено %d", len(fn.Body))
	}
	print := fn.Body[0].(*ir.PrintStmt)
	lit, ok := print.Value.(*ir.IntLiteral)
	if !ok || lit.Value != 42 {
		t.Fatalf("ожидался ir.IntLiteral{42}, получено %+v", print.Value)
	}
	// Проверить, что константа попала в ir.Entity.Constants
	var found *ir.Entity
	for _, e := range irProg.Entities {
		if e.Name == "Ответчик" {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatalf("ожидалась сущность Ответчик в IR")
	}
	if len(found.Constants) != 1 || found.Constants[0].Name != "ОТВЕТ" {
		t.Fatalf("константы сущности не заполнены корректно: %+v", found.Constants)
	}
}

func TestAnalyzeEntityInstanceConstantAccess(t *testing.T) {
	irProg := parseProgram(t, `
сущность Профиль
{
	константа ЛИМИТ = 100
	имеет:
		имя: Строка
		счет: Целое

	конструктор(имя: Строка, счет: Целое)
	{
		этот.имя = имя
		этот.счет = счет
	}
}

функция главная
{
	переменная p = новый Профиль("Иван", 10)
	вывести p.ЛИМИТ
}
`)
	print := irProg.Functions[0].Body[1].(*ir.PrintStmt)
	lit, ok := print.Value.(*ir.IntLiteral)
	if !ok || lit.Value != 100 {
		t.Fatalf("ожидался ir.IntLiteral{100}, получено %+v", print.Value)
	}
}

func TestAssignToEntityConstantViaInstanceFails(t *testing.T) {
	toks, err := lexer.New(`
сущность Профиль
{
	константа ЛИМИТ = 100
	имеет:
		счет: Целое

	конструктор(счет: Целое)
	{
		этот.счет = счет
	}
}

функция главная
{
	переменная p = новый Профиль(10)
	p.ЛИМИТ = 200
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
	if err == nil || !strings.Contains(err.Error(), "констант") {
		t.Fatalf("ожидалась ошибка про присваивание константе сущности, получено: %v", err)
	}
}

func TestPrivateEntityConstantAccessibleInMethod(t *testing.T) {
	toks, err := lexer.New(`
сущность Секрет
{
	приватная константа КОД = 1234
	имеет:
		поле: Целое

	конструктор(поле: Целое)
	{
		этот.поле = поле
	}

	раскрыть(): Целое
	{
		вернуть Секрет.КОД
	}
}

функция главная
{
	переменная s = новый Секрет(0)
	вывести s.раскрыть()
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
	if err != nil {
		t.Fatalf("ожидался успешный анализ доступа к приватной константе внутри метода, получено: %v", err)
	}
}

func TestPublicEntityConstantExplicitModifierAccessibleExternally(t *testing.T) {
	irProg := parseProgram(t, `
сущность Настройки
{
	публичная константа ВЕРСИЯ = 3
}

функция главная
{
	вывести Настройки.ВЕРСИЯ
}
`)
	print := irProg.Functions[0].Body[0].(*ir.PrintStmt)
	lit, ok := print.Value.(*ir.IntLiteral)
	if !ok || lit.Value != 3 {
		t.Fatalf("ожидался ir.IntLiteral{3}, получено %+v", print.Value)
	}
}

func TestPrivateEntityConstantExternalAccessDenied(t *testing.T) {
	toks, err := lexer.New(`
сущность Секрет
{
	приватная константа КОД = 1234
	имеет:
		поле: Целое
}

функция главная
{
	вывести Секрет.КОД
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
	if err == nil || !strings.Contains(err.Error(), "константа") || !strings.Contains(err.Error(), "приватная") {
		t.Fatalf("ожидалась ошибка про приватную константу (без слова \"поле\"), получено: %v", err)
	}
	if strings.Contains(err.Error(), "поле") {
		t.Fatalf("ошибка не должна называть константу полем: %v", err)
	}
}

func TestEntityConstantSelfReferenceFailsCleanly(t *testing.T) {
	toks, err := lexer.New(`
сущность А
{
	константа X = А.X
}

функция главная
{
	вывести А.X
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
	if err == nil || !strings.Contains(err.Error(), "сама на себя") {
		t.Fatalf("ожидалась ошибка о циклической ссылке константы, получено: %v", err)
	}
}

func TestEntityConstantMutualCycleFailsCleanly(t *testing.T) {
	toks, err := lexer.New(`
сущность А
{
	константа X = Б.Y
}

сущность Б
{
	константа Y = А.X
}

функция главная
{
	вывести А.X
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
	if err == nil || !strings.Contains(err.Error(), "сама на себя") {
		t.Fatalf("ожидалась ошибка о циклической ссылке константы, получено: %v", err)
	}
}

// TestAnalyzeExternFuncRejectsNonScalarType - параметр/результат внешней
// функции - только Целое/Действительное/Строка/Логическое (нет стабильного
// C-представления у сущностей/generic-коллекций).
func TestAnalyzeExternFuncRejectsNonScalarType(t *testing.T) {
	toks, err := lexer.New(`
сущность Точка
{
    имеет:
        x: Целое
}
внешняя функция f(p: Точка)
функция главная { }
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "допустимы только") {
		t.Fatalf("ожидалась ошибка про допустимые типы внешней функции, получено: %v", err)
	}
}

// TestAnalyzeExternFuncCollidesWithBuiltinFails - та же коллизия имени с
// зарезервированным builtin'ом, что и у обычной функции.
func TestAnalyzeExternFuncCollidesWithBuiltinFails(t *testing.T) {
	toks, err := lexer.New(`
внешняя функция sqrt как корень(x: Число): Число
функция главная { }
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя встроенной функции") {
		t.Fatalf("ожидалась ошибка про зарезервированное имя, получено: %v", err)
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

// TestAnalyzeTryCatchBothReturningCountsAsFinalReturn - функция, тело
// которой заканчивается "попытаться {...вернуть...} поймать (...)
// {...вернуть...}" с возвратом значения В ОБЕИХ ветках, должна проходить
// проверку "должна заканчиваться вернуть <значение>" - раньше
// endsWithReturnValue смотрел только на буквально последнюю инструкцию
// (сам ReturnStmt), try/catch последней инструкцией не считался вовсе,
// даже когда обе ветки реально возвращают значение - обходили через
// накопление в переменную (examples/проекты/http_сервер).
func TestAnalyzeTryCatchBothReturningCountsAsFinalReturn(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция х(): Целое
{
    попытаться
    {
        вернуть 1
    }
    поймать (иск: Исключение)
    {
        вернуть 0
    }
}
функция главная { вывести х() }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeTryCatchOnlyOneBranchReturningStillFails - симметричная
// проверка: если только ОДНА из веток try/catch возвращает значение,
// ошибка "должна заканчиваться" остаётся (не путь исполнения гарантирует
// значение, значит проверка обязана сработать).
func TestAnalyzeTryCatchOnlyOneBranchReturningStillFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция х(): Целое
{
    попытаться
    {
        вернуть 1
    }
    поймать (иск: Исключение)
    {
        вывести "без возврата"
    }
}
функция главная { вывести х() }
`)
	if err == nil || !strings.Contains(err.Error(), "должна заканчиваться") {
		t.Fatalf("ожидалась ошибка про отсутствие \"вернуть <значение>\" в конце, получено: %v", err)
	}
}

// TestAnalyzeEndingWithThrowCountsAsFinalReturn - функция, тело которой
// заканчивается безусловным "выбросить" (без значения - оно и не нужно,
// исключение всё равно не даёт исполнению провалиться дальше) - должна
// проходить проверку "должна заканчиваться вернуть <значение>" без
// лишнего "вернуть" после "выбросить" (репортнуто пользователем - именно
// такой лишний "вернуть строка" в examples/бд/чанкование.rech,
// значение_курсора).
func TestAnalyzeEndingWithThrowCountsAsFinalReturn(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция х(): Целое
{
    выбросить новое Исключение("нет значения")
}
функция главная { вывести х() }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeIfElseBothReturningCountsAsFinalReturn - функция, тело
// которой заканчивается "если <усл> {...вернуть...} иначе {...вернуть...}"
// с возвратом значения в ОБЕИХ ветках - тоже не должно требовать ничего
// после (та же логика, что и у try/catch выше, распространённая на
// если/иначе).
func TestAnalyzeIfElseBothReturningCountsAsFinalReturn(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция х(а: Целое): Целое
{
    если а > 0
    {
        вернуть а
    }
    иначе
    {
        вернуть 0
    }
}
функция главная { вывести х(1) }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeIfWithoutElseStillFails - "если <усл> {...вернуть...}" БЕЗ
// "иначе" - ошибка "должна заканчиваться" остаётся (путь, где условие
// ложно, ничего не возвращает и не расходится).
func TestAnalyzeIfWithoutElseStillFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция х(а: Целое): Целое
{
    если а > 0
    {
        вернуть а
    }
}
функция главная { вывести х(1) }
`)
	if err == nil || !strings.Contains(err.Error(), "должна заканчиваться") {
		t.Fatalf("ожидалась ошибка про отсутствие \"вернуть <значение>\" в конце, получено: %v", err)
	}
}

// TestAnalyzeIfElseOnlyOneBranchThrowingCountsAsFinalReturn - "если
// {...вернуть...} иначе {...выбросить...}" - смешанная форма (одна ветка
// возвращает значение, другая безусловно расходится через исключение) -
// тоже должна проходить проверку.
func TestAnalyzeIfElseOnlyOneBranchThrowingCountsAsFinalReturn(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция х(а: Целое): Целое
{
    если а > 0
    {
        вернуть а
    }
    иначе
    {
        выбросить новое Исключение("отрицательное")
    }
}
функция главная { вывести х(1) }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeEntityFieldsConstructorAndFieldAccess(t *testing.T) {
	irProg := parseProgram(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
        возраст: Целое

    конструктор(имя: Строка, возраст: Целое)
    {
        этот.имя = имя
        этот.возраст = возраст
    }
}
функция главная
{
    переменная пользователь = новый Пользователь("Илья", 30)
    вывести пользователь.имя
    пользователь.возраст = 31
}
`)
	// 1 пользовательская + "Запрос"/"Ответ"/"ДатаВремя"/"ОпцииКуки"/
	// "Дейтаграмма"/"RGBЦвет"/"Процесс"/"РезультатКоманды"/"СостояниеБД"/
	// "Регистры" (структуры-данные) + "Исключение" + 7 встроенных паник
	// (rtmsg.PanicKinds) - Analyze добавляет их
	// во все out.Entities всегда, не только когда программа их использует -
	// см. package doc registerHTTPEntity и buildIR (internal/semantic/
	// analyzer.go).
	if want := 12 + len(rtmsg.PanicKinds); len(irProg.Entities) != want {
		t.Fatalf("ожидалась 1 пользовательская сущность + 11 встроенных (Запрос/Ответ/ДатаВремя/ОпцииКуки/Дейтаграмма/RGBЦвет/Процесс/РезультатКоманды/СостояниеБД/Регистры/Исключение) + паники, получено %d", len(irProg.Entities))
	}
	entity := irProg.Entities[0]
	if entity.Name != "Пользователь" || len(entity.Fields) != 2 {
		t.Fatalf("неожиданная сущность: %+v", entity)
	}
	if entity.Fields[0].Name != "имя" || entity.Fields[0].Type != ir.TypeString {
		t.Fatalf("неожиданное поле 0: %+v", entity.Fields[0])
	}
	if entity.Fields[1].Name != "возраст" || entity.Fields[1].Type != ir.TypeInt {
		t.Fatalf("неожиданное поле 1: %+v", entity.Fields[1])
	}

	main := irProg.Functions[0]
	varDecl, ok := main.Body[0].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался *ir.VarDecl, получено %T", main.Body[0])
	}
	newExpr, ok := varDecl.Value.(*ir.NewExpr)
	if !ok || newExpr.Entity != "Пользователь" || len(newExpr.Args) != 2 {
		t.Fatalf("ожидался *ir.NewExpr для \"Пользователь\" с 2 аргументами, получено %+v", varDecl.Value)
	}
	if ir.TypeOf(varDecl.Value) != ir.EntityType("Пользователь") {
		t.Fatalf("ожидался тип сущности \"Пользователь\", получено %v", ir.TypeOf(varDecl.Value))
	}

	print := main.Body[1].(*ir.PrintStmt)
	fieldAccess, ok := print.Value.(*ir.FieldAccess)
	if !ok || fieldAccess.Field != "имя" || fieldAccess.Type != ir.TypeString {
		t.Fatalf("ожидался доступ к полю \"имя\", получено %+v", print.Value)
	}

	fieldAssign, ok := main.Body[2].(*ir.FieldAssign)
	if !ok || fieldAssign.Field != "возраст" {
		t.Fatalf("ожидалось присваивание полю \"возраст\", получено %+v", main.Body[2])
	}
}

func TestAnalyzeNamedConstructorArgsNormalizedToPositional(t *testing.T) {
	irProg := parseProgram(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
        возраст: Целое

    конструктор(имя: Строка, возраст: Целое)
    {
        этот.имя = имя
        этот.возраст = возраст
    }
}
функция главная
{
    переменная пользователь = новый Пользователь(возраст: 30, имя: "Илья")
}
`)
	varDecl := irProg.Functions[0].Body[0].(*ir.VarDecl)
	newExpr := varDecl.Value.(*ir.NewExpr)
	if _, ok := newExpr.Args[0].(*ir.StringLiteral); !ok {
		t.Fatalf("ожидалось, что аргумент 0 (имя) - строка, получено %+v", newExpr.Args[0])
	}
	if _, ok := newExpr.Args[1].(*ir.IntLiteral); !ok {
		t.Fatalf("ожидалось, что аргумент 1 (возраст) - число, получено %+v", newExpr.Args[1])
	}
}

func TestAnalyzeMethodAndThisMutation(t *testing.T) {
	irProg := parseProgram(t, `
сущность Пользователь
{
    имеет:
        возраст: Целое

    конструктор(возраст: Целое)
    {
        этот.возраст = возраст
    }

    состариться()
    {
        этот.возраст = этот.возраст + 1
    }

    представиться(): Целое
    {
        вернуть этот.возраст
    }
}
функция главная
{
    переменная пользователь = новый Пользователь(30)
    пользователь.состариться()
    вывести пользователь.представиться()
}
`)
	entity := irProg.Entities[0]
	if len(entity.Methods) != 2 {
		t.Fatalf("ожидалось 2 метода, получено %d", len(entity.Methods))
	}
	ageMethod := entity.Methods[0]
	assign, ok := ageMethod.Body[0].(*ir.FieldAssign)
	if !ok {
		t.Fatalf("ожидалось *ir.FieldAssign, получено %T", ageMethod.Body[0])
	}
	if _, ok := assign.Receiver.(*ir.ThisExpr); !ok {
		t.Fatalf("ожидался получатель \"этот\", получено %+v", assign.Receiver)
	}

	introduceMethod := entity.Methods[1]
	if !introduceMethod.HasReturn || introduceMethod.ReturnType != ir.TypeInt {
		t.Fatalf("неожиданная сигнатура метода: %+v", introduceMethod)
	}

	main := irProg.Functions[0]
	exprStmt, ok := main.Body[1].(*ir.ExprStmt)
	if !ok || exprStmt.MethodCall == nil || exprStmt.MethodCall.Method != "состариться" {
		t.Fatalf("ожидался вызов метода \"состариться\" как инструкция, получено %+v", main.Body[1])
	}
	print := main.Body[2].(*ir.PrintStmt)
	methodCall, ok := print.Value.(*ir.MethodCallExpr)
	if !ok || methodCall.Method != "представиться" || !methodCall.HasReturn {
		t.Fatalf("ожидался вызов метода \"представиться\" как значение, получено %+v", print.Value)
	}
}

func TestAnalyzeMethodCallOnTemporaryEntityAllowed(t *testing.T) {
	// Получатель вызова метода - результат вызова функции (временное
	// значение, без постоянного адреса), но ссылочной сущности (не
	// "структура") - допустим: сущность уже указатель сам по себе,
	// адресуемость получателя (переменная/поле/"этот") ей не нужна.
	// Синтаксически это достижимо только в позиции выражения (сам вызов
	// как самостоятельная инструкция не разбирается как цепочка, только
	// как один CallExpr).
	toks, err := lexer.New(`
сущность Пользователь
{
    имеет:
        возраст: Целое

    конструктор(возраст: Целое)
    {
        этот.возраст = возраст
    }

    прочитать(): Целое
    {
        вернуть этот.возраст
    }
}
функция создать(): Пользователь
{
    вернуть новый Пользователь(1)
}
функция главная
{
    переменная x = создать().прочитать()
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	if _, err = Analyze(prog); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeMethodCallOnTemporaryStructAllowed(t *testing.T) {
	// В отличие от ссылочной сущности, получатель-"структура" (значение, не
	// указатель) как результат вызова НЕ адресуем напрямую в C - но это
	// больше не ошибка семантики: cbackend материализует временную
	// переменную (materializeIfNeeded, internal/backend/cbackend/
	// ir_generator.go), семантика получателя не ограничивает вовсе.
	toks, err := lexer.New(`
структура Точка
{
    имеет:
        x: Целое

    прочитать(): Целое
    {
        вернуть этот.x
    }
}
функция создать(): Точка
{
    вернуть новая Точка(1)
}
функция главная
{
    переменная x = создать().прочитать()
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	if _, err = Analyze(prog); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeFieldAssignOnTemporaryEntityAllowed(t *testing.T) {
	// Присваивание полю ссылочной сущности, полученной как результат
	// вызова метода (не переменная/поле/"этот") - допустимо, тем же
	// принципом, что и TestAnalyzeMethodCallOnTemporaryEntityAllowed:
	// сущность уже указатель сама по себе. Проверяет и вложенную цепочку
	// вызовов (`контейнер.обёртка().счётчик().число = ...`).
	toks, err := lexer.New(`
сущность Счётчик
{
    имеет:
        число: Целое
}
сущность Обёртка
{
    имеет:
        приватный счётчик_: Счётчик

    конструктор(счётчик: Счётчик)
    {
        этот.счётчик_ = счётчик
    }

    счётчик(): Счётчик
    {
        вернуть этот.счётчик_
    }
}
функция создать(): Обёртка
{
    вернуть новая Обёртка(новый Счётчик())
}
функция главная
{
    создать().счётчик().число = 5
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	if _, err = Analyze(prog); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeFieldAssignOnTemporaryStructAllowed(t *testing.T) {
	// Получатель-"структура" (значение) как результат вызова функции - как
	// и у TestAnalyzeMethodCallOnTemporaryStructAllowed, у неё нет
	// постоянного C-адреса, но семантика это больше не проверяет вовсе -
	// cbackend материализует временную переменную (materializeIfNeeded).
	toks, err := lexer.New(`
структура Точка
{
    имеет:
        x: Целое
}
функция создать(): Точка
{
    вернуть новая Точка(0)
}
функция главная
{
    создать().x = 5
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	if _, err = Analyze(prog); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeBecomeStmtAndDerivedType(t *testing.T) {
	irProg := parseProgram(t, `
сущность Пользователь
{
    состояние: неавторизован авторизован

    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }

    авторизовать()
    {
        этот становится авторизованным
    }
}
функция показатьПрофиль(пользователь: АвторизованныйПользователь)
{
    вывести пользователь.имя
}
функция главная
{
    переменная пользователь = новый Пользователь("Илья")
    пользователь.авторизовать()
    показатьПрофиль(пользователь)
}
`)
	entity := irProg.Entities[0]
	become, ok := entity.Methods[0].Body[0].(*ir.BecomeStmt)
	if !ok || become.State != "авторизован" {
		t.Fatalf("ожидался *ir.BecomeStmt(\"авторизован\"), получено %+v", entity.Methods[0].Body[0])
	}
	if _, ok := become.Receiver.(*ir.ThisExpr); !ok {
		t.Fatalf("ожидался получатель \"этот\", получено %+v", become.Receiver)
	}

	// АвторизованныйПользователь - псевдоним без своего представления:
	// параметр разрешается в тип базовой сущности "Пользователь".
	show := irProg.Functions[0]
	if len(show.Params) != 1 || show.Params[0].Type != ir.EntityType("Пользователь") {
		t.Fatalf("ожидался параметр типа \"Пользователь\" (псевдоним разрешён), получено %+v", show.Params)
	}
}

func TestAnalyzeBecomeUndeclaredStateFails(t *testing.T) {
	toks, err := lexer.New(`
сущность Пользователь
{
    состояние: неавторизован авторизован
}
функция главная
{
    переменная пользователь = новый Пользователь()
    пользователь становится удалён
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
	if err == nil || !strings.Contains(err.Error(), "не объявлено состояние") {
		t.Fatalf("ожидалась ошибка про необъявленное состояние, получено: %v", err)
	}
}

// TestAnalyzeBecomeStmtRejectsWrongGenderForm - "становится" требует
// творительный падеж, согласованный с родом сущности: для "Пользователь"
// (муж. род) форма женского рода "авторизованной" неверна.
func TestAnalyzeBecomeStmtRejectsWrongGenderForm(t *testing.T) {
	toks, err := lexer.New(`
сущность Пользователь
{
    состояние: авторизован
}
функция главная
{
    переменная пользователь = новый Пользователь()
    пользователь становится авторизованной
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
	if err == nil || !strings.Contains(err.Error(), "не объявлено состояние") {
		t.Fatalf("ожидалась ошибка про необъявленное состояние, получено: %v", err)
	}
}

// TestAnalyzeBecomeStmtSuggestsInstrumentalFormForOldShortSyntax -
// программист написал старую краткую форму ("авторизован" вместо
// "авторизованным") - ошибка должна подсказать правильную форму
// (suggestStateForm).
func TestAnalyzeBecomeStmtSuggestsInstrumentalFormForOldShortSyntax(t *testing.T) {
	toks, err := lexer.New(`
сущность Пользователь
{
    состояние: авторизован
}
функция главная
{
    переменная пользователь = новый Пользователь()
    пользователь становится авторизован
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
	if err == nil || !strings.Contains(err.Error(), "\"авторизованным\"") {
		t.Fatalf("ожидалась ошибка с подсказкой \"авторизованным\", получено: %v", err)
	}
}

// TestAnalyzeBecomeStmtAcceptsFeminineInstrumentalForm - сущность женского
// рода согласуется по-другому ("авторизованной", не "авторизованным").
func TestAnalyzeBecomeStmtAcceptsFeminineInstrumentalForm(t *testing.T) {
	irProg := parseProgram(t, `
сущность Система
{
    состояние: авторизован

    авторизовать()
    {
        этот становится авторизованной
    }
}
функция главная
{
    переменная система = новая Система()
    система.авторизовать()
}
`)
	entity := irProg.Entities[0]
	become, ok := entity.Methods[0].Body[0].(*ir.BecomeStmt)
	if !ok || become.State != "авторизован" {
		t.Fatalf("ожидался *ir.BecomeStmt(\"авторизован\"), получено %+v", entity.Methods[0].Body[0])
	}
}

func TestAnalyzeAssertStmt(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 2
    утверждать а + 3 == 5
}
`)
	main := irProg.Functions[0]
	assert, ok := main.Body[1].(*ir.AssertStmt)
	if !ok {
		t.Fatalf("ожидался *ir.AssertStmt, получено %+v", main.Body[1])
	}
	if ir.TypeOf(assert.Cond) != ir.TypeBool {
		t.Fatalf("условие должно быть Логическое, получено %s", ir.TypeOf(assert.Cond))
	}
}

func TestAnalyzeAssertNonBoolConditionFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    утверждать 1
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
		t.Fatalf("ожидалась ошибка про тип условия, получено: %v", err)
	}
}

// TestAnalyzeAssertStmtWithMessage - "утверждать <условие>, <сообщение>"
// (внятные сообщения о несовпадении) - сообщение
// анализируется и требует Строка.
func TestAnalyzeAssertStmtWithMessage(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 2
    утверждать а + 3 == 5, "а + 3 должно быть 5"
}
`)
	main := irProg.Functions[0]
	assert, ok := main.Body[1].(*ir.AssertStmt)
	if !ok {
		t.Fatalf("ожидался *ir.AssertStmt, получено %+v", main.Body[1])
	}
	if assert.Message == nil {
		t.Fatal("ожидалось непустое Message")
	}
	if ir.TypeOf(assert.Message) != ir.TypeString {
		t.Fatalf("сообщение должно быть Строка, получено %s", ir.TypeOf(assert.Message))
	}
}

// TestAnalyzeAssertMessageNonStringFails - сообщение обязано быть Строка,
// не любой тип (в отличие от "показать", который умеет любой).
func TestAnalyzeAssertMessageNonStringFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    утверждать 1 == 2, 42
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
	if err == nil || !strings.Contains(err.Error(), "должно быть Строка") {
		t.Fatalf("ожидалась ошибка про тип сообщения, получено: %v", err)
	}
}

func TestAnalyzeListConstructorMethodsAndForEach(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная числа = новый Список<Целое>()
    числа.добавить(5)
    для x в числах
    {
        вывести x
    }
}
`)
	main := irProg.Functions[0]
	varDecl := main.Body[0].(*ir.VarDecl)
	if ir.TypeOf(varDecl.Value) != ir.EntityType("Список<Целое>") {
		t.Fatalf("ожидался тип \"Список<Целое>\", получено %v", ir.TypeOf(varDecl.Value))
	}
	exprStmt, ok := main.Body[1].(*ir.ExprStmt)
	if !ok || exprStmt.MethodCall == nil || exprStmt.MethodCall.Method != "добавить" {
		t.Fatalf("ожидался вызов метода \"добавить\", получено %+v", main.Body[1])
	}
	forEach, ok := main.Body[2].(*ir.ForEachStmt)
	if !ok || forEach.Var != "x" || forEach.ElemType != ir.TypeInt {
		t.Fatalf("ожидался *ir.ForEachStmt с переменной типа Целое, получено %+v", main.Body[2])
	}

	if len(irProg.Generics) != 1 || irProg.Generics[0].Name != "Список<Целое>" || irProg.Generics[0].Template != "Список" || irProg.Generics[0].ElemType != ir.TypeInt {
		t.Fatalf("ожидалась одна инстанциация \"Список<Целое>\", получено %+v", irProg.Generics)
	}
}

func TestAnalyzeStackAndQueueGenerics(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная стопка = новая Стопка<Строка>()
    стопка.положить("x")
    переменная очередь = новая Очередь<Логическое>()
    очередь.добавить(истина)
    вывести стопка.снять()
    вывести очередь.извлечь()
}
`)
	if len(irProg.Generics) != 2 {
		t.Fatalf("ожидалось 2 инстанциации, получено %+v", irProg.Generics)
	}
	byName := map[string]ir.GenericInstantiation{}
	for _, g := range irProg.Generics {
		byName[g.Name] = g
	}
	if g, ok := byName["Стопка<Строка>"]; !ok || g.Template != "Стопка" || g.ElemType != ir.TypeString {
		t.Fatalf("неожиданная инстанциация Стопки: %+v", byName["Стопка<Строка>"])
	}
	if g, ok := byName["Очередь<Логическое>"]; !ok || g.Template != "Очередь" || g.ElemType != ir.TypeBool {
		t.Fatalf("неожиданная инстанциация Очереди: %+v", byName["Очередь<Логическое>"])
	}
}

// TestAnalyzeCollectionEntityElementAllowed - Список<Сущность> как локальная
// переменная - допустимо (как и полем сущности/структуры,
// см. TestAnalyzeCollectionEntityFieldAllowed ниже).
func TestAnalyzeCollectionEntityElementAllowed(t *testing.T) {
	toks, err := lexer.New(`
сущность Пользователь
{
    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
функция главная
{
    переменная x = новый Пользователь("Илья")
    переменная список = новый Список<Пользователь>()
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	if _, err := Analyze(prog); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeCollectionEntityFieldAllowed - ПОЛЕ сущности/структуры МОЖЕТ
// быть встроенной коллекцией с сущностью-элементом (например, "Отдел" со
// "Список<Пользователь> сотрудники" ниже) - раньше это было запрещено
// (internal/semantic.collectEntities, collectionRealEntityElem), потому что
// понадобился бы полный топологический порядок typedef'ов вперемешку с
// сущностями в cbackend вместо фиксированного порядка бакетов; теперь
// cbackend forward-декларирует C-typedef КАЖДОЙ generic-коллекции ДО всех
// остальных typedef'ов (opaque "typedef struct rech_T_... rech_T_...;" -
// см. package doc genericForwardDecl, internal/backend/cbackend/
// ir_generator.go) - полю достаточно указателя на ещё неполный тип, само
// тело коллекции по-прежнему эмитится в свой обычный бакет
// (genericsCodeAfterEntities и т. п.), после typedef сущности-элемента.
func TestAnalyzeCollectionEntityFieldAllowed(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
}
сущность Отдел
{
    имеет:
        сотрудники: Список<Пользователь>
}
функция главная
{
    вывести "ок"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeForEachNonListFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная число = 5
    для x в числе
    {
        вывести x
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
	if err == nil || !strings.Contains(err.Error(), "ожидает переменную-Список/Стопку/Очередь/Словарь") {
		t.Fatalf("ожидалась ошибка про несоответствие типа коллекции, получено: %v", err)
	}
}

func TestAnalyzeBitwiseOperators(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 6 | 3
    переменная б = 6 & 3
    переменная ц = 6 ^ 3
    переменная г = 1 << 2
    переменная д = 8 >> 1
    переменная е = ~0
}
`)
	main := irProg.Functions[0]
	for i, want := range []ir.Type{ir.TypeInt, ir.TypeInt, ir.TypeInt, ir.TypeInt, ir.TypeInt, ir.TypeInt} {
		decl := main.Body[i].(*ir.VarDecl)
		if got := ir.TypeOf(decl.Value); got != want {
			t.Fatalf("инструкция %d: ожидался тип %s, получено %s", i, want, got)
		}
	}
}

func TestAnalyzeModuloOnInt(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 17 % 5
}
`)
	main := irProg.Functions[0]
	decl := main.Body[0].(*ir.VarDecl)
	if got := ir.TypeOf(decl.Value); got != ir.TypeInt {
		t.Fatalf("ожидался тип Целое, получено %s", got)
	}
}

func TestAnalyzeModuloOnFloat(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 7.5 % 2.0
    переменная б = (Дробное) 7.5 % (Дробное) 2.0
}
`)
	main := irProg.Functions[0]
	if got := ir.TypeOf(main.Body[0].(*ir.VarDecl).Value); got != ir.TypeFloat {
		t.Fatalf("Действительное %% Действительное: ожидался тип Действительное, получено %s", got)
	}
	if got := ir.TypeOf(main.Body[1].(*ir.VarDecl).Value); got != ir.TypeFloat32 {
		t.Fatalf("Дробное %% Дробное: ожидался тип Дробное, получено %s", got)
	}
}

func TestAnalyzeBitwiseOnFloatFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная а = 1.0 | 2
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
	if err == nil || !strings.Contains(err.Error(), "побитовый оператор") {
		t.Fatalf("ожидалась ошибка про тип операнда, получено: %v", err)
	}
}

// TestAnalyzeBitwiseOnSameTypeFloatFails - "1.0 | 2.0" (одного типа с обеих
// сторон, в отличие от TestAnalyzeBitwiseOnFloatFails выше) - падает на
// ВТОРОЙ проверке (не целочисленный Kind), не на несовпадении типов - тоже
// должно быть отклонено.
func TestAnalyzeBitwiseOnSameTypeFloatFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = 1.0 | 2.0
}
`)
	if err == nil || !strings.Contains(err.Error(), "побитовый оператор") {
		t.Fatalf("ожидалась ошибка про тип операнда, получено: %v", err)
	}
}

func TestAnalyzeIncDecExprPostfixAndPrefix(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 5
    вывести а++
    вывести ++а
}
`)
	main := irProg.Functions[0]
	post := main.Body[1].(*ir.PrintStmt).Value.(*ir.IncDecExpr)
	if post.Prefix || post.Op != "++" || post.Type != ir.TypeInt {
		t.Fatalf("ожидался постфикс а++ типа Целое, получено %+v", post)
	}
	pre := main.Body[2].(*ir.PrintStmt).Value.(*ir.IncDecExpr)
	if !pre.Prefix || pre.Op != "++" {
		t.Fatalf("ожидался префикс ++а, получено %+v", pre)
	}
}

func TestAnalyzeIncDecOnStringFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная а = "текст"
    а++
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
	if err == nil || !strings.Contains(err.Error(), "применим только к Целое/Дробное/Действительное") {
		t.Fatalf("ожидалась ошибка про тип переменной, получено: %v", err)
	}
}

func TestAnalyzeConstDeclAndRejectsReassign(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    константа x = 5
    x = 6
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
	if err == nil || !strings.Contains(err.Error(), "константа") {
		t.Fatalf("ожидалась ошибка про присваивание константе, получено: %v", err)
	}
}

func TestAnalyzeConstDeclRejectsIncDec(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    константа x = 5
    x++
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
	if err == nil || !strings.Contains(err.Error(), "константа") {
		t.Fatalf("ожидалась ошибка про ++ на константе, получено: %v", err)
	}
}

func TestAnalyzeEnumMemberResolvesToIntLiteral(t *testing.T) {
	irProg := parseProgram(t, `
перечисление Приоритет
{
    Низкий,
    Средний = 5,
    Высокий
}
функция главная
{
    вывести Приоритет.Высокий
    вывести HTTPКод.НеНайдено
}
`)
	printLow := irProg.Functions[0].Body[0].(*ir.PrintStmt)
	lit, ok := printLow.Value.(*ir.IntLiteral)
	if !ok || lit.Value != 6 {
		t.Fatalf("ожидался ir.IntLiteral{6} (Средний=5, Высокий=Средний+1), получено: %+v", printLow.Value)
	}
	printHTTP := irProg.Functions[0].Body[1].(*ir.PrintStmt)
	lit2, ok := printHTTP.Value.(*ir.IntLiteral)
	if !ok || lit2.Value != 404 {
		t.Fatalf("ожидался ir.IntLiteral{404}, получено: %+v", printHTTP.Value)
	}
}

func TestAnalyzeEnumUnknownMemberFails(t *testing.T) {
	toks, err := lexer.New(`
перечисление Цвет { Красный }
функция главная
{
    вывести Цвет.Зелёный
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
	if err == nil || !strings.Contains(err.Error(), "нет члена") {
		t.Fatalf("ожидалась ошибка про неизвестный член перечисления, получено: %v", err)
	}
}

func TestAnalyzeEnumCollidesWithEntityFails(t *testing.T) {
	toks, err := lexer.New(`
сущность Цвет
{
    имеет:
        имя: Строка
}
перечисление Цвет { Красный }
функция главная { вывести 1 }
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "совпадает с именем уже объявленной сущности") {
		t.Fatalf("ожидалась ошибка про коллизию перечисления с сущностью, получено: %v", err)
	}
}

func TestAnalyzeLocalVariableShadowsEnumName(t *testing.T) {
	// Переменная с тем же именем, что и перечисление, должна победить -
	// "HTTPКод.поле" внутри этой области видимости - обычный FieldAccess
	// сущности, не обращение к встроенному перечислению.
	irProg := parseProgram(t, `
сущность HTTPКод
{
    имеет:
        поле: Целое

    конструктор(поле: Целое)
    {
        этот.поле = поле
    }
}
функция главная
{
    переменная HTTPКод = новый HTTPКод(1)
    вывести HTTPКод.поле
}
`)
	fn := irProg.Functions[0]
	print := fn.Body[1].(*ir.PrintStmt)
	if _, ok := print.Value.(*ir.FieldAccess); !ok {
		t.Fatalf("ожидался ir.FieldAccess (обычное поле сущности, не член перечисления), получено %T", print.Value)
	}
}

func TestAnalyzeChisloIsNoLongerAValidTypeName(t *testing.T) {
	toks, err := lexer.New(`
функция сложить(а: Число): Число { вернуть а }
функция главная { вывести сложить(1.5) }
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	if _, err := Analyze(prog); err == nil {
		t.Fatal("ожидалась ошибка - \"Число\" больше не имя типа, только \"Действительное\"")
	}
}

func TestAnalyzeTypeErrorReportsDeystvitelnoye(t *testing.T) {
	toks, err := lexer.New(`
функция обработать(а: Действительное) { вывести а }
функция главная { обработать("текст") }
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "должен быть типа Действительное") {
		t.Fatalf("ожидалась ошибка с каноническим именем типа \"Действительное\", получено: %v", err)
	}
	if strings.Contains(err.Error(), "типа Число") {
		t.Fatalf("сообщение об ошибке не должно называть тип устаревшим именем \"Число\": %v", err)
	}
}

func TestAnalyzeCastIntToStringAndBack(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = (Строка) 42
    переменная б = (Целое) "42"
    вывести а
    вывести б
}
`)
	fn := irProg.Functions[0]
	first := fn.Body[0].(*ir.VarDecl)
	if ir.TypeOf(first.Value) != ir.TypeString {
		t.Fatalf("ожидался тип Строка у первой переменной, получено %v", ir.TypeOf(first.Value))
	}
	second := fn.Body[1].(*ir.VarDecl)
	if ir.TypeOf(second.Value) != ir.TypeInt {
		t.Fatalf("ожидался тип Целое у второй переменной, получено %v", ir.TypeOf(second.Value))
	}
}

func TestAnalyzeCastSameTypeIsIdentity(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = (Целое) 42
}
`)
	decl := irProg.Functions[0].Body[0].(*ir.VarDecl)
	if _, ok := decl.Value.(*ir.CastExpr); ok {
		t.Fatalf("каст типа в самого себя должен свернуться в исходное значение, получен *ir.CastExpr: %+v", decl.Value)
	}
	if _, ok := decl.Value.(*ir.IntLiteral); !ok {
		t.Fatalf("ожидался IntLiteral без обёртки в каст, получено %+v", decl.Value)
	}
}

func TestAnalyzeCastEntityRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое
}
функция главная
{
    переменная p = новая Точка(1)
    переменная а = (Целое) p
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - каст в имя сущности не поддерживается")
	}
}

func TestAnalyzeCastDisallowedPairRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Логическое) 1
}
`)
	if err == nil || !strings.Contains(err.Error(), "нельзя привести") {
		t.Fatalf("ожидалась ошибка \"нельзя привести\" для Целое -> Логическое, получено: %v", err)
	}
}

func TestAnalyzeCharLiteralType(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 'я'
}
`)
	decl := irProg.Functions[0].Body[0].(*ir.VarDecl)
	if ir.TypeOf(decl.Value) != ir.TypeChar {
		t.Fatalf("ожидался тип Символ, получено %v", ir.TypeOf(decl.Value))
	}
}

func TestAnalyzeMixedArithmeticPromotesToWidestType(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная целое = 1
    переменная дробное = (Дробное) 1.5
    переменная действительное = 2.5
    переменная а = целое + дробное
    переменная б = дробное + действительное
}
`)
	fn := irProg.Functions[0]
	a := fn.Body[3].(*ir.VarDecl)
	if ir.TypeOf(a.Value) != ir.TypeFloat32 {
		t.Fatalf("Целое + Дробное должно дать Дробное, получено %v", ir.TypeOf(a.Value))
	}
	b := fn.Body[4].(*ir.VarDecl)
	if ir.TypeOf(b.Value) != ir.TypeFloat {
		t.Fatalf("Дробное + Действительное должно дать Действительное, получено %v", ir.TypeOf(b.Value))
	}
}

func TestAnalyzeCharAndByteAreOrderableButNotArithmetic(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = 'а' < 'б'
    переменная б = (Байт) 1 < (Байт) 2
}
`)
	fn := irProg.Functions[0]
	if ir.TypeOf(fn.Body[0].(*ir.VarDecl).Value) != ir.TypeBool {
		t.Fatalf("сравнение Символ должно дать Логическое")
	}
	if ir.TypeOf(fn.Body[1].(*ir.VarDecl).Value) != ir.TypeBool {
		t.Fatalf("сравнение Байт должно дать Логическое")
	}

	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = 'а' + 'б'
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - арифметика над Символ не поддерживается")
	}
}

func TestAnalyzeStructDeclSetsIRIsStruct(t *testing.T) {
	irProg := parseProgram(t, `
структура Точка
{
    имеет:
        x: Целое
}
сущность Пользователь
{
    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
функция главная
{
    переменная p = новая Точка(1)
    переменная u = новый Пользователь("Аня")
}
`)
	var точка, пользователь *ir.Entity
	for _, ent := range irProg.Entities {
		switch ent.Name {
		case "Точка":
			точка = ent
		case "Пользователь":
			пользователь = ent
		}
	}
	if точка == nil || !точка.IsStruct {
		t.Fatalf("ожидался ir.Entity(\"Точка\").IsStruct == true, получено %+v", точка)
	}
	if пользователь == nil || пользователь.IsStruct {
		t.Fatalf("ожидался ir.Entity(\"Пользователь\").IsStruct == false, получено %+v", пользователь)
	}
}

// TestAnalyzeCurrentOSBuiltin - "текущая_ос()" (Целое) и встроенное
// перечисление "ОС" (StandardEnums, internal/semantic/types.go) -
// используются вместе тем же способом, что и остальные builtin-
// перечисления (СигналОС/СетевойПротокол) - сравнение с "==".
func TestAnalyzeCurrentOSBuiltin(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
	если текущая_ос() == ОС.Linux { вывести "linux" }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
