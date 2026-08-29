package parser

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// TestParseMethodTagParsed - тег в обратных кавычках после типа результата
// метода (шаг 6.3, "противоположность") разбирается в ast.MethodDecl.Tag,
// тем же синтаксисом, что и у FunctionDecl.Tag/Field.Tag.
func TestParseMethodTagParsed(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    состояние: вошедший

    войти(): Пользователь
    {
        этот становится вошедшим
        вернуть этот
    }

    выйти() `+"`противоположность:войти`"+`
    {
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	var exit *ast.MethodDecl
	for _, m := range ent.Methods {
		if m.Name == "выйти" {
			exit = m
		}
	}
	if exit == nil || exit.Tag != "противоположность:войти" {
		t.Fatalf("ожидался Tag \"противоположность:войти\" у метода \"выйти\", получено %+v", exit)
	}
}

// TestParseMethodWithoutTagHasEmptyTag - метод без тега - Tag == "" (не
// nil-панику где-то дальше по коду).
func TestParseMethodWithoutTagHasEmptyTag(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    войти() { }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Entities[0].Methods[0].Tag != "" {
		t.Fatalf("ожидался пустой Tag, получено %q", prog.Entities[0].Methods[0].Tag)
	}
}
