package lsp

import (
	"strings"
	"testing"
)

func TestSignatureHelpOnUserFunctionCall(t *testing.T) {
	src := `
/** Складывает два числа. */
функция сложить(кого: Целое, кому: Целое): Целое
{
    вернуть кого + кому
}

функция главная
{
    переменная итог = сложить(1, 2)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "сложить(1, 2)")
	col := runeCol(t, "    переменная итог = сложить(1, 2)", "2)")
	sig, ok := d.signatureHelp(line, col)
	if !ok {
		t.Fatalf("ожидалась signature help внутри вызова")
	}
	if sig.Label != "сложить(кого: Целое, кому: Целое): Целое" {
		t.Fatalf("неожиданный label: %q", sig.Label)
	}
	if sig.ActiveParam != 1 {
		t.Fatalf("ожидался активный параметр 1 (второй), получено %d", sig.ActiveParam)
	}
	if !strings.Contains(sig.Documentation, "Складывает два числа.") {
		t.Fatalf("ожидался doc-комментарий в Documentation, получено %q", sig.Documentation)
	}
}

func TestSignatureHelpFirstParamActive(t *testing.T) {
	src := `
функция сложить(кого: Целое, кому: Целое): Целое
{
    вернуть кого + кому
}

функция главная
{
    переменная итог = сложить(1
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "сложить(1")
	col := runeCol(t, "    переменная итог = сложить(1", "1")
	sig, ok := d.signatureHelp(line, col+1)
	if !ok {
		t.Fatalf("ожидалась signature help внутри незакрытого вызова")
	}
	if sig.ActiveParam != 0 {
		t.Fatalf("ожидался активный параметр 0 (первый), получено %d", sig.ActiveParam)
	}
}

func TestSignatureHelpOnConstructor(t *testing.T) {
	src := `
сущность Точка
{
    имеет:
        x: Целое
        y: Целое

    конструктор(x: Целое, y: Целое)
    {
        этот.x = x
        этот.y = y
    }
}

функция главная
{
    переменная т = новая Точка(1, 2)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "новая Точка(1, 2)")
	col := runeCol(t, "    переменная т = новая Точка(1, 2)", "2)")
	sig, ok := d.signatureHelp(line, col)
	if !ok {
		t.Fatalf("ожидалась signature help внутри конструктора")
	}
	if sig.Label != "новый Точка(x: Целое, y: Целое)" {
		t.Fatalf("неожиданный label: %q", sig.Label)
	}
	if sig.ActiveParam != 1 {
		t.Fatalf("ожидался активный параметр 1, получено %d", sig.ActiveParam)
	}
}

func TestSignatureHelpOnMethodCall(t *testing.T) {
	src := `
сущность Точка
{
    имеет:
        x: Целое

    /** Расстояние от начала координат. */
    длина(множитель: Целое): Целое
    {
        вернуть этот.x
    }
}

функция главная
{
    переменная т = новая Точка()
    переменная д = т.длина(3)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "т.длина(3)")
	col := runeCol(t, "    переменная д = т.длина(3)", "3)")
	sig, ok := d.signatureHelp(line, col)
	if !ok {
		t.Fatalf("ожидалась signature help внутри вызова метода")
	}
	if sig.Label != "Точка.длина(множитель: Целое): Целое" {
		t.Fatalf("неожиданный label: %q", sig.Label)
	}
	if !strings.Contains(sig.Documentation, "Расстояние от начала координат.") {
		t.Fatalf("ожидался doc-комментарий метода, получено %q", sig.Documentation)
	}
}

func TestSignatureHelpOnBuiltinFunction(t *testing.T) {
	src := `
функция главная
{
    переменная с = подстрока("привет", 0, 3)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "подстрока(")
	col := runeCol(t, `    переменная с = подстрока("привет", 0, 3)`, "3)")
	sig, ok := d.signatureHelp(line, col)
	if !ok {
		t.Fatalf("ожидалась signature help для встроенной функции")
	}
	if sig.Label != "подстрока(Строка, Целое, Целое): Строка" {
		t.Fatalf("неожиданный label: %q", sig.Label)
	}
	if sig.ActiveParam != 2 {
		t.Fatalf("ожидался активный параметр 2 (третий), получено %d", sig.ActiveParam)
	}
}

// TestSignatureHelpRightBeforeClosingParen - живой баг: курсор сразу перед
// уже введённой (не только что напечатанной) закрывающей скобкой ошибочно
// считал вызов уже закрытым (lastTokenAtOrBefore включала саму RPAREN, раз
// её позиция совпадала с позицией курсора) - signatureHelp молча переставал
// работать на последнем символе перед ")", хотя весь остальной вызов ещё
// редактируется.
func TestSignatureHelpRightBeforeClosingParen(t *testing.T) {
	src := `
функция сложить(кого: Целое, кому: Целое): Целое
{
    вернуть кого + кому
}

функция главная
{
    переменная итог = сложить(1, 2)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "сложить(1, 2)")
	col := runeCol(t, "    переменная итог = сложить(1, 2)", ")")
	sig, ok := d.signatureHelp(line, col)
	if !ok {
		t.Fatalf("ожидалась signature help сразу перед закрывающей скобкой")
	}
	if sig.ActiveParam != 1 {
		t.Fatalf("ожидался активный параметр 1, получено %d", sig.ActiveParam)
	}
}

func TestSignatureHelpNoneOutsideCall(t *testing.T) {
	src := `
функция главная
{
    переменная а = 1 + 2
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "1 + 2")
	col := runeCol(t, "    переменная а = 1 + 2", "1")
	if _, ok := d.signatureHelp(line, col); ok {
		t.Fatalf("не ожидалась signature help вне вызова")
	}
}

func TestSignatureHelpIgnoresNestedListLiteralCommas(t *testing.T) {
	src := `
функция сложить(кого: Целое, кому: Целое): Целое
{
    вернуть кого + кому
}

функция главная
{
    переменная итог = сложить([1, 2, 3].длина(), 5)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "сложить([1, 2, 3]")
	col := runeCol(t, "    переменная итог = сложить([1, 2, 3].длина(), 5)", "5)")
	sig, ok := d.signatureHelp(line, col)
	if !ok {
		t.Fatalf("ожидалась signature help")
	}
	if sig.ActiveParam != 1 {
		t.Fatalf("запятые внутри [1, 2, 3] не должны считаться - ожидался активный параметр 1, получено %d", sig.ActiveParam)
	}
}

// TestSignatureHelpShowsParamDefaultValue - параметр со значением по
// умолчанию ("шаг: Целое = 1") должен нести его и в Label, и в
// ParamLabels[i] - раньше ir.Param (единственный источник и того, и
// другого) не несёт default-выражение вообще, оно только в AST
// (funcParamDefaultsAST/appendParamDefaults). Оба должны совпадать буквально
// (ParamLabels[i] - подстрока Label) - от этого зависит подсветка активного
// параметра на стороне клиента (VS Code ищет подстроку).
func TestSignatureHelpShowsParamDefaultValue(t *testing.T) {
	src := `
функция шагнуть(число: Целое, шаг: Целое = 1): Целое
{
    вернуть число + шаг
}

функция главная
{
    переменная итог = шагнуть(10, 2)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "шагнуть(10, 2)")
	col := runeCol(t, "    переменная итог = шагнуть(10, 2)", "2)")
	sig, ok := d.signatureHelp(line, col)
	if !ok {
		t.Fatalf("ожидалась signature help")
	}
	if len(sig.ParamLabels) != 2 || sig.ParamLabels[1] != "шаг: Целое = 1" {
		t.Fatalf("ожидался параметр \"шаг: Целое = 1\", получено %+v", sig.ParamLabels)
	}
	if !strings.Contains(sig.Label, sig.ParamLabels[1]) {
		t.Fatalf("ParamLabels[1] %q должен быть подстрокой Label %q", sig.ParamLabels[1], sig.Label)
	}
}

// TestSignatureHelpShowsConstructorParamDefaultValue - то же, что и
// TestSignatureHelpShowsParamDefaultValue, но для НЕЯВНОГО конструктора
// структуры (позиционные поля - ctorParamDefaultsAST, ветка "нет
// ed.Ctor", не ast.ConstructorDecl.Params).
func TestSignatureHelpShowsConstructorParamDefaultValue(t *testing.T) {
	src := `
структура Точка
{
    имеет:
        x: Целое
        y: Целое = 0
}
функция главная
{
    переменная т = новая Точка(1, 2)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "новая Точка(1, 2)")
	col := runeCol(t, "    переменная т = новая Точка(1, 2)", "2)")
	sig, ok := d.signatureHelp(line, col)
	if !ok {
		t.Fatalf("ожидалась signature help")
	}
	if len(sig.ParamLabels) != 2 || sig.ParamLabels[1] != "y: Целое = 0" {
		t.Fatalf("ожидался параметр \"y: Целое = 0\", получено %+v", sig.ParamLabels)
	}
}
