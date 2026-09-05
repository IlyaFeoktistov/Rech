package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// TestParseProgramLenientSkipsBrokenFunctionKeepsRest - ключевой сценарий
// докстринга ROADMAP: опечатка в одной функции файла не должна гасить весь
// файл целиком, остальные объявления (в т.ч. ПОСЛЕ сломанного) должны
// разобраться как обычно.
func TestParseProgramLenientSkipsBrokenFunctionKeepsRest(t *testing.T) {
	src := `
функция хорошая1
{
    вывести "1"
}

функция сломанная(
{
    вывести "не разберётся"
}

функция хорошая2
{
    вывести "2"
}
`
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, errs := New(toks).ParseProgramLenient()
	if len(errs) != 1 {
		t.Fatalf("ожидалась ровно одна ошибка, получено %d: %v", len(errs), errs)
	}
	if len(prog.Functions) != 2 {
		t.Fatalf("ожидалось 2 разобранные функции, получено %d", len(prog.Functions))
	}
	if prog.Functions[0].Name != "хорошая1" || prog.Functions[1].Name != "хорошая2" {
		t.Fatalf("неверный набор разобранных функций: %q, %q", prog.Functions[0].Name, prog.Functions[1].Name)
	}
}

// TestParseProgramLenientNoErrorsMatchesStrict - при отсутствии ошибок
// lenient-режим не должен ничем отличаться от обычного ParseProgram.
func TestParseProgramLenientNoErrorsMatchesStrict(t *testing.T) {
	src := `
функция главная
{
    вывести "ок"
}
`
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, errs := New(toks).ParseProgramLenient()
	if len(errs) != 0 {
		t.Fatalf("ошибок не ожидалось, получено %v", errs)
	}
	if len(prog.Functions) != 1 || prog.Functions[0].Name != "главная" {
		t.Fatalf("неверный результат разбора: %+v", prog.Functions)
	}
}

// TestParseProgramLenientMultipleBrokenDecls - несколько независимо
// сломанных объявлений в одном файле дают несколько ошибок, а не одну (и не
// обрывают разбор на первой же).
func TestParseProgramLenientMultipleBrokenDecls(t *testing.T) {
	src := `
функция сломанная1(
{
    вывести "x"
}

сущность СломаннаяСущность
    имеет:

функция хорошая
{
    вывести "ок"
}
`
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, errs := New(toks).ParseProgramLenient()
	if len(errs) < 2 {
		t.Fatalf("ожидалось минимум 2 ошибки, получено %d: %v", len(errs), errs)
	}
	if len(prog.Functions) != 1 || prog.Functions[0].Name != "хорошая" {
		t.Fatalf("ожидалась ровно одна уцелевшая функция \"хорошая\", получено %+v", prog.Functions)
	}
}

// TestParseProgramLenientBrokenEntityKeepsFollowingEntity - ошибка внутри
// тела сущности (не функции) не должна поглотить следующую за ней сущность -
// проверка, что recoverToNextTopLevel считает фигурные скобки, а не просто
// ищет ближайшее ключевое слово.
func TestParseProgramLenientBrokenEntityKeepsFollowingEntity(t *testing.T) {
	src := `
сущность Сломанная
{
    имеет:
        поле Строка
}

сущность Целая
{
    имеет:
        имя: Строка
}
`
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, errs := New(toks).ParseProgramLenient()
	if len(errs) != 1 {
		t.Fatalf("ожидалась ровно одна ошибка (пропущенное \":\" у поля), получено %d: %v", len(errs), errs)
	}
	if len(prog.Entities) != 1 || prog.Entities[0].Name != "Целая" {
		t.Fatalf("ожидалась ровно одна уцелевшая сущность \"Целая\", получено %+v", prog.Entities)
	}
}

// TestParseProgramLenientBrokenMethodParamsKeepsFollowingEntity - как
// TestParseProgramLenientBrokenEntityKeepsFollowingEntity, но поломка - в
// списке параметров МЕТОДА (не поля) - именно то место, где предварительный
// проход collectEntitySignature (natural_method_call.go) сам вызывает
// parseParams (tryCollectMethodHeader), а не просто вслепую пропускает
// токены, как для остального тела сущности - ошибка здесь должна попасть в
// errs ОДИН раз (общий lenient-путь ParseProgram, не задвоенная тем же
// предварительным проходом), а следующая сущность - уцелеть.
func TestParseProgramLenientBrokenMethodParamsKeepsFollowingEntity(t *testing.T) {
	src := `
сущность Сломанная
{
    метод(параметр Строка) { }
}

сущность Целая
{
    имеет:
        имя: Строка
}
`
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, errs := New(toks).ParseProgramLenient()
	if len(errs) != 1 {
		t.Fatalf("ожидалась ровно одна ошибка (пропущенное \":\" у параметра метода), получено %d: %v", len(errs), errs)
	}
	if len(prog.Entities) != 1 || prog.Entities[0].Name != "Целая" {
		t.Fatalf("ожидалась ровно одна уцелевшая сущность \"Целая\", получено %+v", prog.Entities)
	}
}
