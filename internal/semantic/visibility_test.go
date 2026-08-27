package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeProtectedFieldAccessibleFromSubclassMethod(t *testing.T) {
	irProg := parseProgram(t, `
сущность Животное
{
    имеет:
        защищённое имя: Строка
}
сущность Собака наследует Животное
{
    представиться(): Строка
    {
        вернуть этот.имя
    }
}
функция главная
{
    переменная пёс = новая Собака("Шарик")
    вывести пёс.представиться()
}
`)
	if len(irProg.Entities) < 2 {
		t.Fatalf("ожидались обе сущности в IR, получено %+v", irProg.Entities)
	}
}

func TestAnalyzeProtectedFieldNotAccessibleFromOutsideHierarchy(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Животное
{
    имеет:
        защищённое имя: Строка
}
функция главная
{
    переменная ж = новое Животное("Барсик")
    вывести ж.имя
}
`)
	if err == nil || !strings.Contains(err.Error(), "защищённое") {
		t.Fatalf("ожидалась ошибка доступа к защищённому полю снаружи, получено: %v", err)
	}
}

func TestAnalyzeProtectedFieldNotAccessibleFromUnrelatedEntity(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Животное
{
    имеет:
        защищённое имя: Строка
}
сущность Машина
{
    имеет:
        владелец: Целое

    прочитать(а: Животное): Строка
    {
        вернуть а.имя
    }
}
функция главная { переменная м = новая Машина(1) }
`)
	if err == nil || !strings.Contains(err.Error(), "защищённое") {
		t.Fatalf("ожидалась ошибка - Машина не наследует Животное, доступ к защищённому полю запрещён, получено: %v", err)
	}
}

func TestAnalyzePrivateMethodCannotSatisfyInterface(t *testing.T) {
	_, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Кошка реализует Звучащий
{
    приватный издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция главная { переменная кот = новая Кошка() }
`)
	if err == nil || !strings.Contains(err.Error(), "не публичный") {
		t.Fatalf("ожидалась ошибка - приватный метод не может реализовать интерфейс, получено: %v", err)
	}
}
