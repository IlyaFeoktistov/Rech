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

func TestAnalyzeFixedWidthIntArithmeticMixedWidthRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Целое8) 5
    переменная б = (Целое16) 10
    переменная результат = а + б
}
`)
	if err == nil || !strings.Contains(err.Error(), "Целое8 и Целое16 - разные типы") {
		t.Fatalf("ожидалась ошибка смешанной арифметики Целое8/Целое16, получено: %v", err)
	}
}

func TestAnalyzeFixedWidthIntArithmeticMixedSignRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Целое32) 5
    переменная б = (Натуральное32) 10
    переменная результат = а + б
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка смешанной знаковой/беззнаковой арифметики")
	}
}

func TestAnalyzeFixedWidthIntDoesNotPromoteToInt(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = (Целое32) 5
    переменная б = 10
    переменная результат = а + б
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - Целое32 не промоутится к Целое неявно")
	}
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
