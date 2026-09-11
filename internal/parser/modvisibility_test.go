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

// TestParsePrivateEnumAgreesWithNeuterGender/TestParsePrivateConstAgreesWithFeminineGender -
// паритет модульной видимости с уже существующей у функций/сущностей
// : "перечисление" среднего рода ("приватное"), "константа"
// женского ("приватная", тем же согласованием, что и checkConstVisibilityAgreement
// у константы уровня сущности).
func TestParsePrivateEnumAgreesWithNeuterGender(t *testing.T) {
	prog, err := parseSource(t, `
приватное перечисление Скрытый
{
    А
}
публичное перечисление Открытый
{
    Б
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Enums[0].Visibility != ast.Private {
		t.Fatalf("ожидалось приватное перечисление, получено %v", prog.Enums[0].Visibility)
	}
	if prog.Enums[1].Visibility != ast.Public {
		t.Fatalf("ожидалось публичное перечисление, получено %v", prog.Enums[1].Visibility)
	}
}

func TestParsePrivateEnumWrongGenderRejected(t *testing.T) {
	_, err := parseSource(t, `
приватная перечисление Скрытый
{
    А
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "приватное") {
		t.Fatalf("ожидалась ошибка о согласовании рода (\"приватное перечисление\"), получено: %v", err)
	}
}

func TestParsePrivateConstAgreesWithFeminineGender(t *testing.T) {
	prog, err := parseSource(t, `
приватная константа Секрет = 1
публичная константа Известный = 2
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Consts) != 2 || prog.Consts[0].Visibility != ast.Private || prog.Consts[1].Visibility != ast.Public {
		t.Fatalf("ожидались приватная и публичная константы, получено %+v", prog.Consts)
	}
}

func TestParsePrivateConstWrongGenderRejected(t *testing.T) {
	_, err := parseSource(t, `
приватный константа Секрет = 1
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "приватная") {
		t.Fatalf("ожидалась ошибка о согласовании рода (\"приватная константа\"), получено: %v", err)
	}
}

// TestParseInternalAgreesWithGenderAcrossKinds - "внутренний" (МЯГКОЕ
// прилагательное, окончание "-ий/-яя/-ее", в отличие от твёрдого "-ый/-ая/
// -ое" у приватный/публичный) согласуется по роду с тем же governing-
// словом, что и остальные модификаторы: функция/сущность/структура/
// константа - женский ("внутренняя"), класс - мужской ("внутренний"),
// перечисление - средний ("внутреннее").
func TestParseInternalAgreesWithGenderAcrossKinds(t *testing.T) {
	prog, err := parseSource(t, `
внутренняя функция помощник(): Целое
{
    вернуть 1
}
внутренний класс Скрытый { }
внутренняя структура Точка
{
    имеет:
        x: Целое
}
внутреннее перечисление Режим
{
    А
}
внутренняя константа Секрет = 1
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Functions[0].Visibility != ast.Internal {
		t.Fatalf("ожидалась внутренняя функция, получено %v", prog.Functions[0].Visibility)
	}
	for i, ent := range prog.Entities {
		if ent.Visibility != ast.Internal {
			t.Fatalf("сущность %d (%s): ожидалась внутренняя, получено %v", i, ent.Name, ent.Visibility)
		}
	}
	if prog.Enums[0].Visibility != ast.Internal {
		t.Fatalf("ожидалось внутреннее перечисление, получено %v", prog.Enums[0].Visibility)
	}
	if prog.Consts[0].Visibility != ast.Internal {
		t.Fatalf("ожидалась внутренняя константа, получено %v", prog.Consts[0].Visibility)
	}
}

func TestParseInternalWrongGenderRejected(t *testing.T) {
	_, err := parseSource(t, `
внутренний функция помощник(): Целое
{
    вернуть 1
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "внутренняя") {
		t.Fatalf("ожидалась ошибка о согласовании рода (\"внутренняя функция\"), получено: %v", err)
	}
}

// TestParseConstDeclWithoutVisibilityStaysPublic - без модификатора
// константа уровня модуля остаётся публичной (обратная совместимость - вся
// программа до этой фичи писала "константа" без модификатора).
func TestParseConstDeclWithoutVisibilityStaysPublic(t *testing.T) {
	prog, err := parseSource(t, `
константа Порог = 10
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(prog.Consts) != 1 || prog.Consts[0].Visibility != ast.Public {
		t.Fatalf("ожидалась публичная константа по умолчанию, получено %+v", prog.Consts)
	}
}
