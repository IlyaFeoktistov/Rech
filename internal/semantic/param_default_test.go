package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestAnalyzeFuncDefaultParamOmitted - вызов функции с трейлинговым
// значением по умолчанию без последнего аргумента: пропущенный аргумент
// дополняется вычисленным значением по умолчанию прямо в ir.CallExpr.Args
// (см. package doc checkCallArgs/resolveParamDefault).
func TestAnalyzeFuncDefaultParamOmitted(t *testing.T) {
	irProg := parseProgram(t, `
функция сложить(а: Целое, б: Целое = 10): Целое
{
    вернуть а + б
}
функция главная
{
    вывести сложить(5)
}
`)
	главная := findFunc(irProg, "главная")
	if главная == nil {
		t.Fatalf("функция \"главная\" не найдена")
	}
	ret, ok := главная.Body[0].(*ir.PrintStmt)
	if !ok {
		t.Fatalf("ожидался PrintStmt, получено %T", главная.Body[0])
	}
	call, ok := ret.Value.(*ir.CallExpr)
	if !ok || call.Callee != "сложить" {
		t.Fatalf("ожидался вызов \"сложить\", получено %#v", ret.Value)
	}
	if len(call.Args) != 2 {
		t.Fatalf("ожидалось 2 аргумента (второй - подставленное значение по умолчанию), получено %d", len(call.Args))
	}
	lit, ok := call.Args[1].(*ir.IntLiteral)
	if !ok || lit.Value != 10 {
		t.Fatalf("ожидалось значение по умолчанию 10, получено %#v", call.Args[1])
	}
}

// TestAnalyzeFuncTooFewArgsWithoutDefaultFails - обязательный (не
// трейлинговый со значением по умолчанию) аргумент нельзя опустить.
func TestAnalyzeFuncTooFewArgsWithoutDefaultFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция сложить(а: Целое, б: Целое = 10): Целое
{
    вернуть а + б
}
функция главная
{
    вывести сложить()
}
`)
	if err == nil || !strings.Contains(err.Error(), "минимум 1") {
		t.Fatalf("ожидалась ошибка про минимальное число аргументов, получено: %v", err)
	}
}

// TestAnalyzeFuncNullableDefaultParam - "?Целое = ничто" как значение по
// умолчанию nullable-параметра.
func TestAnalyzeFuncNullableDefaultParam(t *testing.T) {
	irProg := parseProgram(t, `
функция найти(б: ?Целое = Ничто): Логическое
{
    вернуть б != Ничто
}
функция главная
{
    вывести найти()
}
`)
	главная := findFunc(irProg, "главная")
	if главная == nil {
		t.Fatalf("функция \"главная\" не найдена")
	}
	if _, ok := главная.Body[0].(*ir.PrintStmt); !ok {
		t.Fatalf("ожидался PrintStmt, получено %T", главная.Body[0])
	}
}

// TestAnalyzeMethodDefaultParamOmitted - то же самое для метода сущности
// (checkCallArgs общая для функций и методов).
func TestAnalyzeMethodDefaultParamOmitted(t *testing.T) {
	irProg := parseProgram(t, `
сущность Счётчик
{
    сложить(а: Целое, б: Целое = 10): Целое
    {
        вернуть а + б
    }
}
функция главная
{
    переменная счётчик = новый Счётчик()
    вывести счётчик.сложить(5)
}
`)
	главная := findFunc(irProg, "главная")
	if главная == nil {
		t.Fatalf("функция \"главная\" не найдена")
	}
	ret, ok := главная.Body[1].(*ir.PrintStmt)
	if !ok {
		t.Fatalf("ожидался PrintStmt, получено %T", главная.Body[1])
	}
	call, ok := ret.Value.(*ir.MethodCallExpr)
	if !ok {
		t.Fatalf("ожидался MethodCallExpr, получено %#v", ret.Value)
	}
	if len(call.Args) != 2 {
		t.Fatalf("ожидалось 2 аргумента, получено %d", len(call.Args))
	}
}

// TestAnalyzeCtorPositionalDefaultParamOmitted - явный конструктор с
// трейлинговым значением по умолчанию в позиционной форме.
func TestAnalyzeCtorPositionalDefaultParamOmitted(t *testing.T) {
	irProg := parseProgram(t, `
сущность Точка
{
    имеет:
        Целое x
        Целое y

    конструктор(x: Целое, y: Целое = 10)
    {
        этот.x = x
        этот.y = y
    }
}
функция главная
{
    переменная точка = новая Точка(3)
    вывести точка.x
}
`)
	var точка *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Точка" {
			точка = ent
		}
	}
	if точка == nil || точка.Ctor == nil || len(точка.Ctor.Params) != 2 {
		t.Fatalf("ожидался конструктор Точка с 2 параметрами")
	}
}

// TestAnalyzeCtorNamedFormDefaultParamFilled - именованная форма "новый
// Тип(...)" может опустить параметр конструктора, у которого есть
// значение по умолчанию (в отличие от TestAnalyzeExplicitConstructorNamedFormMissingParamFails,
// где значения по умолчанию нет).
func TestAnalyzeCtorNamedFormDefaultParamFilled(t *testing.T) {
	irProg := parseProgram(t, `
сущность Точка
{
    имеет:
        Целое x
        Целое y

    конструктор(x: Целое, y: Целое = 10)
    {
        этот.x = x
        этот.y = y
    }
}
функция главная
{
    переменная точка = новая Точка(x: 3)
    вывести точка.x
}
`)
	var точка *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Точка" {
			точка = ent
		}
	}
	if точка == nil || точка.Ctor == nil {
		t.Fatalf("ожидался конструктор Точка")
	}
}

// TestAnalyzeDBQueryParamsOmitted - третий аргумент "запросить"/"выполнить"
// (список параметров запроса) необязателен (см. dbNoParamsDefault, db.go)
// - "выполнить(бд, sql)" без плейсхолдеров анализируется без ошибок, с
// подставленным "новый Список<Значение>()" третьим аргументом.
func TestAnalyzeDBQueryParamsOmitted(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    выполнить(бд, "CREATE TABLE t (x INTEGER)")
    переменная строки = запросить(бд, "SELECT * FROM t")
    вывести строки.длина()
    закрыть_бд(бд)
}
`)
	главная := findFunc(irProg, "главная")
	if главная == nil {
		t.Fatalf("функция \"главная\" не найдена")
	}
	call, ok := главная.Body[1].(*ir.ExprStmt)
	if !ok || call.Call == nil || call.Call.Callee != "выполнить" {
		t.Fatalf("ожидался вызов \"выполнить\", получено %#v", главная.Body[1])
	}
	if len(call.Call.Args) != 3 {
		t.Fatalf("ожидалось 3 аргумента (третий - подставленное значение по умолчанию), получено %d", len(call.Call.Args))
	}
	if _, ok := call.Call.Args[2].(*ir.NewExpr); !ok {
		t.Fatalf("ожидался ir.NewExpr (\"новый Список<Значение>()\") третьим аргументом, получено %#v", call.Call.Args[2])
	}
}
