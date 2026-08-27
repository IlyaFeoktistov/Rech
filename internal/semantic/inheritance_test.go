package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeEntityInheritsFieldsAndMethods(t *testing.T) {
	irProg := parseProgram(t, `
сущность Животное
{
    имеет:
        имя: Строка

    представиться(): Строка
    {
        вернуть этот.имя
    }
}
сущность Собака наследует Животное
{
    имеет:
        порода: Строка
}
функция главная
{
    переменная пёс = новая Собака("Шарик", "дворняга")
    вывести пёс.представиться()
    вывести пёс.порода
}
`)
	var собака *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Собака" {
			собака = ent
		}
	}
	if собака == nil {
		t.Fatal("сущность \"Собака\" не найдена в IR")
	}
	if len(собака.Fields) != 2 || собака.Fields[0].Name != "имя" || собака.Fields[1].Name != "порода" {
		t.Fatalf("ожидались поля [имя, порода] (базовое первым), получено %+v", собака.Fields)
	}
	if собака.BaseName != "Животное" {
		t.Fatalf("ожидался BaseName == \"Животное\", получено %q", собака.BaseName)
	}
}

func TestAnalyzeMethodOverrideMustMatchBaseSignature(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Животное
{
    издать_звук(): Строка
    {
        вернуть "..."
    }
}
сущность Кошка наследует Животное
{
    издать_звук(): Целое
    {
        вернуть 1
    }
}
функция главная { переменная к = новая Кошка() }
`)
	if err == nil || !strings.Contains(err.Error(), "не совпадает по сигнатуре с методом базовой сущности") {
		t.Fatalf("ожидалась ошибка о несовпадении сигнатуры переопределения, получено: %v", err)
	}
}

func TestAnalyzeMethodOverrideWithMatchingSignatureIsAllowed(t *testing.T) {
	irProg := parseProgram(t, `
сущность Животное
{
    издать_звук(): Строка
    {
        вернуть "..."
    }
}
сущность Кошка наследует Животное
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция главная
{
    переменная к = новая Кошка()
    вывести к.издать_звук()
}
`)
	var кошка *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Кошка" {
			кошка = ent
		}
	}
	if кошка == nil || len(кошка.Methods) != 1 || кошка.Methods[0].Name != "издать_звук" {
		t.Fatalf("ожидался ровно один собственный метод \"издать_звук\" (переопределение), получено %+v", кошка)
	}
}

func TestAnalyzeInheritFromUndeclaredEntityFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Кошка наследует Незнакомец
{
    имеет:
        x: Целое
}
функция главная { переменная к = новая Кошка(1) }
`)
	if err == nil || !strings.Contains(err.Error(), "базовая сущность не объявлена") {
		t.Fatalf("ожидалась ошибка про необъявленную базовую сущность, получено: %v", err)
	}
}

func TestAnalyzeInheritFromStructFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Точка
{
    имеет:
        x: Целое
}
сущность Производная наследует Точка
{
    имеет:
        y: Целое
}
функция главная { переменная д = новая Производная(1, 2) }
`)
	if err == nil || !strings.Contains(err.Error(), "структура не участвует в наследовании") {
		t.Fatalf("ожидалась ошибка - структура не может быть базовой, получено: %v", err)
	}
}

func TestAnalyzeFieldCollisionWithInheritedFieldFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Животное
{
    имеет:
        имя: Строка
}
сущность Собака наследует Животное
{
    имеет:
        имя: Целое
}
функция главная { переменная д = новая Собака("Шарик", 1) }
`)
	if err == nil || !strings.Contains(err.Error(), "конфликтует с унаследованным полем") {
		t.Fatalf("ожидалась ошибка о конфликте с унаследованным полем, получено: %v", err)
	}
}

func TestAnalyzeDerivedValueAssignableToBaseTypedParam(t *testing.T) {
	irProg := parseProgram(t, `
сущность Животное
{
    имеет:
        имя: Строка
}
сущность Собака наследует Животное
{
    имеет:
        порода: Строка
}
функция представить(а: Животное): Строка
{
    вернуть а.имя
}
функция главная
{
    переменная пёс = новая Собака("Шарик", "дворняга")
    вывести представить(пёс)
}
`)
	if len(irProg.Functions) < 1 {
		t.Fatal("ожидалась хотя бы одна функция в IR")
	}
}

func TestAnalyzeUnrelatedEntityNotAssignableToBaseTypedParam(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Животное
{
    имеет:
        имя: Строка
}
сущность Машина
{
    имеет:
        марка: Строка
}
функция представить(а: Животное): Строка
{
    вернуть а.имя
}
функция главная
{
    переменная м = новая Машина("Лада")
    вывести представить(м)
}
`)
	if err == nil || !strings.Contains(err.Error(), "должен быть типа") {
		t.Fatalf("ожидалась ошибка типа - Машина не Животное, получено: %v", err)
	}
}
