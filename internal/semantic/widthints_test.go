package semantic

import (
	"strings"
	"testing"
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
	if err == nil || !strings.Contains(err.Error(), "не поддерживается") {
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
    переменная а = (Целое64) 5
    переменная б = 10
    переменная результат = а + б
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - Целое64 не промоутится к Целое неявно, даже при одинаковой ширине")
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
