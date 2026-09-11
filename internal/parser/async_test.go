package parser

import (
	"strings"
	"testing"
)

func TestParseAsyncFunctionAgreesWithFunctionGender(t *testing.T) {
	prog, err := parseSource(t, `
асинхронная функция получить_данные(): Целое
{
    вернуть 1
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !prog.Functions[0].IsAsync {
		t.Fatalf("ожидалась IsAsync == true у \"получить_данные\"")
	}
	if prog.Functions[1].IsAsync {
		t.Fatalf("\"главная\" не должна быть асинхронной")
	}
}

func TestParseAsyncWrongGenderRejected(t *testing.T) {
	_, err := parseSource(t, `
асинхронный функция получить_данные(): Целое
{
    вернуть 1
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "асинхронная") {
		t.Fatalf("ожидалась ошибка о согласовании рода (\"асинхронная\"), получено: %v", err)
	}
}

func TestParseAsyncOnlyBeforeFunctionRejected(t *testing.T) {
	_, err := parseSource(t, `
асинхронная сущность А
{
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "\"функция\"") {
		t.Fatalf("ожидалась ошибка о том, что \"асинхронная\" поддерживается только перед \"функция\", получено: %v", err)
	}
}

func TestParseAsyncMainRejected(t *testing.T) {
	_, err := parseSource(t, `
асинхронная функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "точка входа") {
		t.Fatalf("ожидалась ошибка о точке входа программы, получено: %v", err)
	}
}

func TestParseAsyncWithVisibilityOrder(t *testing.T) {
	prog, err := parseSource(t, `
приватная асинхронная функция получить_данные(): Целое
{
    вернуть 1
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !prog.Functions[0].IsAsync {
		t.Fatalf("ожидалась IsAsync == true")
	}
}
