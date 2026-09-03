package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParsePrivateFunctionAgreesWithFunctionGender(t *testing.T) {
	prog, err := parseSource(t, `
приватная функция помощник(): Целое
{
    вернуть 1
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Functions[0].Visibility != ast.Private {
		t.Fatalf("ожидалась приватная функция, получено %v", prog.Functions[0].Visibility)
	}
	if prog.Functions[1].Visibility != ast.Public {
		t.Fatalf("ожидалась публичная \"главная\" без модификатора, получено %v", prog.Functions[1].Visibility)
	}
}

func TestParsePrivateFunctionWrongGenderRejected(t *testing.T) {
	_, err := parseSource(t, `
приватный функция помощник(): Целое
{
    вернуть 1
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "приватная") {
		t.Fatalf("ожидалась ошибка о согласовании рода (\"приватная функция\"), получено: %v", err)
	}
}

func TestParsePrivateEntityAgreesWithEntityKeyword(t *testing.T) {
	prog, err := parseSource(t, `
приватная сущность А
{
    имеет:
        x: Целое
}
приватный класс Б { }
приватная структура В
{
    имеет:
        y: Целое
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for i, ent := range prog.Entities {
		if ent.Visibility != ast.Private {
			t.Fatalf("сущность %d (%s): ожидалась приватная, получено %v", i, ent.Name, ent.Visibility)
		}
	}
}

func TestParsePrivateClassWrongGenderRejected(t *testing.T) {
	_, err := parseSource(t, `
приватная класс Б { }
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "приватный") {
		t.Fatalf("ожидалась ошибка о согласовании рода (\"приватный класс\"), получено: %v", err)
	}
}

func TestParseProtectedRejectedAtModuleLevel(t *testing.T) {
	_, err := parseSource(t, `
защищённая функция помощник(): Целое
{
    вернуть 1
}
функция главная { вывести "ок" }
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка - \"защищённый\" не имеет смысла на верхнем уровне файла")
	}
}

// TestParsePrivateCombinesWithAbstractAndExport - модификатор видимости
// сочетается с "абстрактный"/"экспортировать" в любом порядке (см. package
// doc governingModuleKeyword).
func TestParsePrivateCombinesWithAbstractAndExport(t *testing.T) {
	prog, err := parseSource(t, `
приватная абстрактная сущность Фигура { }
приватная экспортировать функция помощник(): Целое
{
    вернуть 1
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Entities[0].Visibility != ast.Private || !prog.Entities[0].IsAbstract {
		t.Fatalf("ожидалась приватная абстрактная сущность, получено %+v", prog.Entities[0])
	}
	if prog.Functions[0].Visibility != ast.Private || !prog.Functions[0].Exported {
		t.Fatalf("ожидалась приватная экспортированная функция, получено %+v", prog.Functions[0])
	}
}

func TestParseVisibilityModifierRejectedBeforeUse(t *testing.T) {
	_, err := parseSource(t, `
приватная использовать сложить из "утилиты"
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "неуместен") {
		t.Fatalf("ожидалась ошибка о неуместном модификаторе, получено: %v", err)
	}
}
