package semantic

import (
	"strings"
	"testing"
)

// Позиционная/именованная формы "новый Тип(...)" с пропуском полей со
// значением по умолчанию доступны только у "структура" - у "сущность"/
// "класс" без явного конструктора допустим только "новый Тип()" без
// аргументов вовсе (docs/SPEC.md, "Явные конструкторы") - см.
// constructor_test.go, TestAnalyzeEntityWithoutOwnCtorRejectsArgs.

func TestAnalyzeNewExprPositionalOmitsTrailingDefault(t *testing.T) {
	src := `
структура Точка
{
    имеет:
        Целое x
        Целое y = 0
}
функция главная
{
    переменная точка = новая Точка(5)
    вывести точка.x
    вывести точка.y
}
`
	parseProgram(t, src)
}

func TestAnalyzeNewExprNamedOmitsDefault(t *testing.T) {
	src := `
структура Точка
{
    имеет:
        Целое x
        Целое y = 0
}
функция главная
{
    переменная точка = новая Точка(x: 5)
    вывести точка.y
}
`
	parseProgram(t, src)
}

func TestAnalyzeNewExprPositionalMissingRequiredIsError(t *testing.T) {
	src := `
структура Точка
{
    имеет:
        Целое x = 0
        Целое y
}
функция главная
{
    переменная точка = новая Точка(1)
}
`
	_, err := AnalyzeSource(t, src)
	if err == nil || !strings.Contains(err.Error(), "y") {
		t.Fatalf("ожидалась ошибка про пропущенное поле y без значения по умолчанию, получено: %v", err)
	}
}

func TestAnalyzeNewExprNamedMissingRequiredIsError(t *testing.T) {
	src := `
структура Точка
{
    имеет:
        Целое x
        Целое y = 0
}
функция главная
{
    переменная точка = новая Точка(y: 1)
}
`
	_, err := AnalyzeSource(t, src)
	if err == nil || !strings.Contains(err.Error(), "x") {
		t.Fatalf("ожидалась ошибка про не указанное поле x без значения по умолчанию, получено: %v", err)
	}
}

func TestAnalyzeFieldDefaultReferencesEntityConstant(t *testing.T) {
	src := `
сущность Точка
{
    константа НАЧАЛО = 0

    имеет:
        Целое x = Точка.НАЧАЛО
}
функция главная
{
    переменная точка = новая Точка()
    вывести точка.x
}
`
	prog := parseProgram(t, src)
	ent := findEntity(prog, "Точка")
	if ent == nil || ent.Fields[0].Default == nil {
		t.Fatalf("ожидалось разрешённое значение по умолчанию поля x")
	}
}

func TestAnalyzeFieldDefaultWrongTypeIsError(t *testing.T) {
	src := `
сущность Точка
{
    имеет:
        Целое x = "не число"
}
функция главная { вывести "ок" }
`
	_, err := AnalyzeSource(t, src)
	if err == nil {
		t.Fatalf("ожидалась ошибка несовпадения типа значения по умолчанию")
	}
}

// TestAnalyzeInheritedAndOwnFieldDefaultsApplyOnEmptyConstruction - "новый
// Круг()" без аргументов (без своего "конструктор") - и унаследованное поле
// без значения по умолчанию зануляется, и своё со значением по умолчанию
// получает его - оба пути идут через один и тот же синтетический пустой
// конструктор (см. package doc analyzeEntity в analyzer.go), не через
// прежнее позиционное заполнение.
func TestAnalyzeInheritedAndOwnFieldDefaultsApplyOnEmptyConstruction(t *testing.T) {
	src := `
сущность Фигура
{
    имеет:
        Целое номер
}
сущность Круг наследует Фигуру
{
    имеет:
        Целое радиус = 1
}
функция главная
{
    переменная круг = новый Круг()
    вывести круг.номер
    вывести круг.радиус
}
`
	parseProgram(t, src)
}
