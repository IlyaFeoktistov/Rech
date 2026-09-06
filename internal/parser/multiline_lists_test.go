package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// TestParseMultilineFunctionParams - список параметров функции можно
// разбить на несколько строк (перенос после "(" и после каждой ",") - тот
// же приём, что и у многострочного "использовать" (parseUseDecl).
func TestParseMultilineFunctionParams(t *testing.T) {
	src := `
функция сложить(
    кого: Целое,
    кому: Целое
): Целое
{
    вернуть кого + кому
}
функция главная { вывести "ок" }
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if len(fn.Params) != 2 || fn.Params[0].Name != "кого" || fn.Params[1].Name != "кому" {
		t.Fatalf("неожиданные параметры: %+v", fn.Params)
	}
}

// TestParseTrailingCommaParamsRequiresNewlineBeforeParen - висячая запятая
// перед ")" списка параметров ДОПУСТИМА, только если закрывающая скобка на
// своей отдельной строке - по решению пользователя, чтобы не появлялось
// двух неразличимых стилей одного и того же списка ("f(a, b)" и
// "f(a, b,)" в одну строку).
func TestParseTrailingCommaParamsRequiresNewlineBeforeParen(t *testing.T) {
	src := `
функция сложить(
    кого: Целое,
    кому: Целое,
): Целое
{
    вернуть кого + кому
}
функция главная { вывести "ок" }
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if len(fn.Params) != 2 {
		t.Fatalf("ожидалось 2 параметра при висячей запятой, получено %d: %+v", len(fn.Params), fn.Params)
	}
}

// TestParseTrailingCommaSameLineIsError - та же висячая запятая, но БЕЗ
// переноса строки перед ")" (одной строкой) - ошибка, не молча принятый
// синоним обычного списка.
func TestParseTrailingCommaSameLineIsError(t *testing.T) {
	src := `
функция главная
{
    вывести сложить(1, 2,)
}
функция сложить(a: Целое, b: Целое): Целое
{
    вернуть a + b
}
`
	_, err := parseSource(t, src)
	if err == nil {
		t.Fatal("ожидалась ошибка парсера у висячей запятой без переноса строки")
	}
	if !strings.Contains(err.Error(), "висячая запятая") {
		t.Fatalf("ожидалось сообщение про висячую запятую, получено: %v", err)
	}
}

// TestParseMultilineCallArgsWithTrailingComma - вызов функции с аргументами
// на нескольких строках и висячей запятой перед ")" на своей строке.
func TestParseMultilineCallArgsWithTrailingComma(t *testing.T) {
	src := `
функция главная
{
    вывести сложить(
        1,
        2,
    )
}
функция сложить(a: Целое, b: Целое): Целое
{
    вернуть a + b
}
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	call, ok := prog.Functions[len(prog.Functions)-2].Body[0].(*ast.PrintStmt).Value.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		t.Fatalf("ожидался вызов \"сложить\" с 2 аргументами, получено: %+v", prog.Functions[0].Body[0])
	}
}

// TestParseMultilineListLiteralWithTrailingComma - литерал списка "[...]" с
// переносами строк и висячей запятой перед "]".
func TestParseMultilineListLiteralWithTrailingComma(t *testing.T) {
	src := `
функция главная
{
    Список<Целое> числа = [
        1,
        2,
        3,
    ]
    вывести числа.длина()
}
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	decl, ok := prog.Functions[0].Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("ожидалось объявление переменной, получено %+v", prog.Functions[0].Body[0])
	}
	lit, ok := decl.Value.(*ast.ListLiteral)
	if !ok || len(lit.Elems) != 3 {
		t.Fatalf("ожидался литерал списка из 3 элементов, получено %+v", decl.Value)
	}
}

// TestParseMultilineGenericCallTypeArgsWithTrailingComma - явный вызов
// generic-функции с аргументами типа на нескольких строках и висячей
// запятой перед ">" на своей строке.
func TestParseMultilineGenericCallTypeArgsWithTrailingComma(t *testing.T) {
	src := `
функция первый<T>(список: Список<T>): T
{
    вернуть список.получить(0)
}
функция главная
{
    переменная сп = новый Список<Целое>()
    сп.добавить(5)
    вывести первый<
        Целое,
    >(сп)
}
`
	_, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
