package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeParseDateReturnsNullableDateTime(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = разобрать_дату("2026-09-10 12:00:00")
    если д != Ничто {
        переменная дд = д ?? время_как_дата(0)
        вывести дату_в_строку(дд)
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeTimeAsDateInZoneReturnsNullableDateTime(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = время_как_дата_в_поясе(1700000000, "Europe/Moscow")
    если д != Ничто {
        переменная дд = д ?? время_как_дата(0)
        вывести дату_в_строку(дд)
    }
    переменная метка = дата_в_поясе_как_время(время_как_дата(0), "Europe/Moscow")
    если метка != Ничто {
        вывести (метка ?? 0)
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDateTimeArithmeticMethods(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = время_как_дата(0)
    переменная позже = д.добавить_дни(1).добавить_часы(2).добавить_минуты(3).добавить_секунды(4)
    переменная раньше = позже.вычесть_дни(1).вычесть_часы(2).вычесть_минуты(3).вычесть_секунды(4)
    вывести дату_в_строку(позже)
    вывести дату_в_строку(раньше)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDateTimeComparisonAndDiffMethods(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = время_как_дата(0)
    переменная б = время_как_дата(100000)
    вывести а.раньше(б)
    вывести б.позже(а)
    вывести а.разница_в_днях(б)
    вывести а.разница_в_часах(б)
    вывести а.разница_в_секундах(б)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDateTimeIsWeekendAndHumanReadable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = время_как_дата(0)
    вывести д.является_выходным()
    вывести д.в_читаемом_виде()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDateTimeInZoneMethod(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = время_как_дата(0)
    переменная москва = д.в_поясе("Europe/Moscow")
    если москва != Ничто {
        вывести дату_в_строку(москва ?? д)
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDateTimeArithmeticMethodRejectsWrongArgType(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = время_как_дата(0)
    переменная х = д.добавить_дни("не число")
    вывести х
}
`)
	if err == nil || !strings.Contains(err.Error(), "Целое") {
		t.Fatalf("ожидалась ошибка про Целое, получено: %v", err)
	}
}

func TestAnalyzeDateTimeInZoneMethodRejectsWrongArgCount(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = время_как_дата(0)
    переменная х = д.в_поясе()
    вывести х
}
`)
	if err == nil || !strings.Contains(err.Error(), "один аргумент") {
		t.Fatalf("ожидалась ошибка про один аргумент, получено: %v", err)
	}
}

func TestAnalyzeDateTimeMethodNameOnRegularEntityStillWorksNormally(t *testing.T) {
	// "добавить_дни" - известное имя метода "ДатаВремя", но у ОБЫЧНОЙ
	// сущности с методом того же имени должен работать обычный путь
	// (analyzeMethodCallExpr), а не desugaring.
	_, err := AnalyzeSource(t, `
сущность Счётчик
{
    имеет:
        значение: Целое

    конструктор(значение: Целое)
    {
        этот.значение = значение
    }

    добавить_дни(n: Целое): Целое
    {
        вернуть этот.значение + n
    }
}
функция главная
{
    переменная сч = новый Счётчик(5)
    вывести сч.добавить_дни(3)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
