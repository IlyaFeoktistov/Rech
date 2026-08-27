package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeNewExprPositionalOmitsTrailingDefault(t *testing.T) {
	src := `
сущность Точка
{
    имеет:
        x: Целое
        y: Целое = 0
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
сущность Точка
{
    имеет:
        x: Целое
        y: Целое = 0
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
сущность Точка
{
    имеет:
        x: Целое = 0
        y: Целое
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
сущность Точка
{
    имеет:
        x: Целое
        y: Целое = 0
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
        x: Целое = Точка.НАЧАЛО
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
        x: Целое = "не число"
}
функция главная { вывести "ок" }
`
	_, err := AnalyzeSource(t, src)
	if err == nil {
		t.Fatalf("ожидалась ошибка несовпадения типа значения по умолчанию")
	}
}

func TestAnalyzeInheritedFieldDefaultUsableInPositionalOmission(t *testing.T) {
	src := `
сущность Фигура
{
    имеет:
        номер: Целое
}
сущность Круг наследует Фигуру
{
    имеет:
        радиус: Целое = 1
}
функция главная
{
    переменная круг = новый Круг(5)
    вывести круг.номер
    вывести круг.радиус
}
`
	parseProgram(t, src)
}
