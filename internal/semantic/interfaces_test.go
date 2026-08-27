package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeInterfaceMethodCallResolvesThroughContract(t *testing.T) {
	irProg := parseProgram(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Кошка реализует Звучащий
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция озвучить(х: Звучащий): Строка
{
    вернуть х.издать_звук()
}
функция главная
{
    переменная кошка = новая Кошка()
    вывести озвучить(кошка)
}
`)
	if len(irProg.Interfaces) != 1 || irProg.Interfaces[0].Name != "Звучащий" {
		t.Fatalf("ожидался один интерфейс \"Звучащий\", получено %+v", irProg.Interfaces)
	}
}

func TestAnalyzeEntityNotImplementingInterfaceFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Камень реализует Звучащий
{
    имеет:
        вес: Целое
}
функция главная { переменная кот = новый Камень(1) }
`)
	if err == nil || !strings.Contains(err.Error(), "не реализует метод") {
		t.Fatalf("ожидалась ошибка про нереализованный метод, получено: %v", err)
	}
}

func TestAnalyzeInterfaceMethodSignatureMismatchFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Кошка реализует Звучащий
{
    издать_звук(): Целое
    {
        вернуть 1
    }
}
функция главная { переменная кот = новая Кошка() }
`)
	if err == nil || !strings.Contains(err.Error(), "не совпадает по сигнатуре") {
		t.Fatalf("ожидалась ошибка про несовпадение сигнатуры, получено: %v", err)
	}
}

func TestAnalyzeCannotConstructInterface(t *testing.T) {
	_, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
функция главная { переменная х = новый Звучащий() }
`)
	if err == nil || !strings.Contains(err.Error(), "интерфейс") {
		t.Fatalf("ожидалась ошибка - интерфейс нельзя создать через \"новый\", получено: %v", err)
	}
}

func TestAnalyzeFieldAccessOnInterfaceTypedValueFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Кошка реализует Звучащий
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция описать(х: Звучащий): Строка
{
    вернуть х.издать_звук()
}
функция главная
{
    переменная кошка = новая Кошка()
    вывести кошка.издать_звук()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeInterfaceTypeInField(t *testing.T) {
	irProg := parseProgram(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Кошка реализует Звучащий
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
сущность Клетка
{
    имеет:
        обитатель: Звучащий
}
функция главная
{
    переменная кошка = новая Кошка()
    переменная клетка = новая Клетка(кошка)
    вывести клетка.обитатель.издать_звук()
}
`)
	var клетка *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Клетка" {
			клетка = ent
		}
	}
	if клетка == nil || len(клетка.Fields) != 1 || клетка.Fields[0].Type.Kind != ir.KindInterface {
		t.Fatalf("ожидалось поле \"обитатель\" интерфейсного типа, получено %+v", клетка)
	}
}
