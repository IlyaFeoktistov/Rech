package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeFixedWidthIntArithmeticSameTypeOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Целое8) 5
    переменная б = (Целое8) 10
    переменная результат = а + б
    вывести результат
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeFixedWidthIntArithmeticMixedWidthWidens(t *testing.T) {
	assertSumType(t, "(Целое8) 5", "(Целое16) 10", ir.TypeInt16)
}

// assertSumType - тип "а + б" при а = left, б = right.
func assertSumType(t *testing.T, left, right string, want ir.Type) {
	t.Helper()
	irProg, err := AnalyzeSource(t, "функция главная\n{\n    переменная а = "+left+"\n    переменная б = "+right+"\n    переменная результат = а + б\n}\n")
	if err != nil {
		t.Fatalf("%s + %s: неожиданная ошибка: %v", left, right, err)
	}
	if got := ir.TypeOf(irProg.Functions[0].Body[2].(*ir.VarDecl).Value); got != want {
		t.Fatalf("%s + %s: ожидался тип %s, получен %s", left, right, want, got)
	}
}

func TestAnalyzeFixedWidthIntArithmeticMixedSign(t *testing.T) {
	// Знаковый тип, вмещающий оба, - не беззнаковый, как в C.
	assertSumType(t, "(Целое32) 5", "(Натуральное32) 10", ir.TypeInt)
	assertSumType(t, "(Целое8) 5", "(Байт) 10", ir.TypeInt16)
	assertSumType(t, "(Целое32) 5", "(Натуральное16) 10", ir.TypeInt32)
	_, err := AnalyzeSource(t, "функция главная\n{\n    переменная а = (Целое) 5\n    переменная б = (Натуральное) 10\n    переменная результат = а + б\n}\n")
	if err == nil || !strings.Contains(err.Error(), "нет знакового типа") {
		t.Fatalf("ожидалась ошибка Целое + Натуральное, получено %v", err)
	}
}

func TestAnalyzeFixedWidthIntPromotesToInt(t *testing.T) {
	assertSumType(t, "(Целое32) 5", "10", ir.TypeInt)
	assertSumType(t, "(Целое32) 5", "(Целое) 10", ir.TypeInt)
	assertSumType(t, "(Целое32) 5", "1.5", ir.TypeFloat)
}

// TestAnalyzeInt64AndUint64AreSynonyms - "Целое64" - то же, что "Целое",
// "Натуральное64" - то же, что "Натуральное": смешиваются без приведения,
// передаются в параметры друг друга, "Целое64" промоутится к
// "Действительное", как и "Целое".
func TestAnalyzeInt64AndUint64AreSynonyms(t *testing.T) {
	irProg := parseProgram(t, `
функция удвоить(х: Целое64): Целое
{
    вернуть х * 2
}

функция главная
{
    Целое а = 1
    Целое64 б = 2
    Натуральное н = 3
    Натуральное64 м = 4
    вывести а + б
    вывести удвоить(а)
    вывести н + м
    вывести б * 1.5
}
`)
	fn := irProg.Functions[0]
	if fn.Params[0].Type != ir.TypeInt || fn.ReturnType != ir.TypeInt {
		t.Fatalf("Целое64 должно резолвиться в Целое, получено %v -> %v", fn.Params[0].Type, fn.ReturnType)
	}
}

func TestAnalyzeFixedWidthIntCastRoundTrip(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Целое64) 5
    переменная б = 10
    переменная результат = а + (Целое64) б
    вывести (Целое) результат
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeFixedWidthIntComparisonAndEquality(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Натуральное8) 5
    переменная б = (Натуральное8) 10
    вывести а < б
    вывести а == б
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeFixedWidthIntIncDec(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Целое16) 5
    а++
    вывести а
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeFixedWidthIntUnaryMinus(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Целое32) 5
    вывести -а
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeFixedWidthIntCastToStringAndByte(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Целое32) 5
    вывести (Строка) а
    вывести (Байт) а
    переменная б = (Целое32) (Байт) 200
    вывести б
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeByteIsUint8Synonym - "Натуральное8" - то же, что "Байт":
// смешиваются без приведения, коллекции одного типа; у Байт - строгая
// арифметика фиксированной ширины (с Целое без приведения - ошибка).
func TestAnalyzeByteIsUint8Synonym(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    Байт а = 250
    Натуральное8 б = 10
    Байт сум = а + б
    Список<Байт> байты = [а]
    Список<Натуральное8> те_же = байты
    сум++
}
`)
	if typ := irProg.Functions[0].Body[2].(*ir.VarDecl).DeclType; typ != ir.TypeByte {
		t.Fatalf("Натуральное8 должно резолвиться в Байт, получено %v", typ)
	}
	assertSumType(t, "(Байт) 1", "(Целое) 2", ir.TypeInt)
}

// TestAnalyzeSynonymsInsideGenericAndNullable - второе имя синонима внутри
// аргумента generic и после "?" - тот же тип, что и короткое.
func TestAnalyzeSynonymsInsideGenericAndNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    Список<Целое> а = [1]
    Список<Целое64> б = а
    ?Целое64 х = Ничто
    ?Целое г = х
    Словарь<Строка, Натуральное64> д = новый Словарь<Строка, Натуральное>()
    Список<Натуральное8> байты = [(Байт) 1]
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
