package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeCannotConstructAbstractEntity(t *testing.T) {
	_, err := AnalyzeSource(t, `
абстрактная сущность Животное
{
    голос(): Строка
}
функция главная
{
    переменная х = новое Животное()
}
`)
	if err == nil || !strings.Contains(err.Error(), "абстрактная сущность") {
		t.Fatalf("ожидалась ошибка - нельзя создать абстрактную сущность, получено: %v", err)
	}
}

func TestAnalyzeConcreteEntityMustImplementAbstractMethod(t *testing.T) {
	_, err := AnalyzeSource(t, `
абстрактная сущность Животное
{
    голос(): Строка
}
сущность Кот наследует Животное
{
}
функция главная
{
    переменная х = новый Кот()
}
`)
	if err == nil || !strings.Contains(err.Error(), "не реализован абстрактный метод") {
		t.Fatalf("ожидалась ошибка - не реализован абстрактный метод, получено: %v", err)
	}
}

func TestAnalyzeConcreteEntityImplementingAbstractMethodSucceeds(t *testing.T) {
	irProg := parseProgram(t, `
абстрактная сущность Животное
{
    имеет:
        имя: Строка

    голос(): Строка

    представиться(): Строка
    {
        вернуть этот.имя
    }
}
сущность Кот наследует Животное
{
    конструктор(имя: Строка)
    {
        этот.имя = имя
    }

    голос(): Строка
    {
        вернуть "мяу"
    }
}
функция главная
{
    переменная кот = новый Кот("Барсик")
    вывести кот.представиться()
    вывести кот.голос()
}
`)
	var кот *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Кот" {
			кот = ent
		}
	}
	if кот == nil {
		t.Fatal("сущность \"Кот\" не найдена в IR")
	}
	if len(кот.Methods) != 1 || кот.Methods[0].Name != "голос" || кот.Methods[0].IsAbstract {
		t.Fatalf("ожидался один собственный метод \"голос\" с реализацией, получено %+v", кот.Methods)
	}
}

func TestAnalyzeAbstractSubclassMayLeaveMethodUnimplemented(t *testing.T) {
	irProg := parseProgram(t, `
абстрактная сущность А
{
    голос(): Строка
}
абстрактная сущность Б наследует А
{
    движение(): Строка
}
сущность В наследует Б
{
    голос(): Строка
    {
        вернуть "голос"
    }
    движение(): Строка
    {
        вернуть "движение"
    }
}
функция главная
{
    переменная х = новый В()
    вывести х.голос()
    вывести х.движение()
}
`)
	var б *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Б" {
			б = ent
		}
	}
	if б == nil {
		t.Fatal("сущность \"Б\" не найдена в IR")
	}
	if !б.IsAbstract {
		t.Fatalf("ожидалась абстрактная сущность \"Б\", получено %+v", б)
	}
}

func TestAnalyzeAbstractMethodDeclaredReturnTypeDoesNotRequireReturnStatement(t *testing.T) {
	_, err := AnalyzeSource(t, `
абстрактная сущность Животное
{
    голос(): Строка
}
сущность Кот наследует Животное
{
    голос(): Строка
    {
        вернуть "мяу"
    }
}
функция главная
{
    переменная кот = новый Кот()
    вывести кот.голос()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
