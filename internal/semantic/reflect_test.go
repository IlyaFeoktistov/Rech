package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeTypeOfExprOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная т = тип Целое
    вывести т.имя()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeTypeOfUnknownTypeRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная т = тип НетТакогоТипа
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - неизвестный тип")
	}
}

func TestAnalyzeBoxCastAnyValueOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое
}
функция главная
{
    переменная а = (Значение) 5
    переменная б = (Значение) "текст"
    переменная точка = новая Точка(1)
    переменная г = (Значение) точка
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeAsExprRequiresAnyReceiver(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = 5
    переменная б = а как Целое
}
`)
	if err == nil || !strings.Contains(err.Error(), "\"как\" применим только к \"Значение\"") {
		t.Fatalf("ожидалась ошибка про \"как\" не на Значение, получено: %v", err)
	}
}

func TestAnalyzeAsExprRoundTripOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная коробка = (Значение) 5
    переменная обратно = коробка как Целое
    вывести обратно
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeAnyMethodsOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная коробка = (Значение) 5
    переменная т = коробка.тип_значения()
    вывести т.имя()
    переменная аргументы = новый Список<Значение>()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeAnyUnknownMethodRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная коробка = (Значение) 5
    коробка.несуществующий_метод()
}
`)
	if err == nil || !strings.Contains(err.Error(), "у \"Значение\" нет метода") {
		t.Fatalf("ожидалась ошибка про неизвестный метод Значение, получено: %v", err)
	}
}

func TestAnalyzeTypeInfoMethodsOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое
        y: Целое

    длина(): Целое
    {
        вернуть этот.x
    }
}
функция главная
{
    переменная т = тип Точка
    вывести т.число_полей()
    вывести т.имя_поля(0)
    переменная типПоля = т.тип_поля(0)
    вывести типПоля.имя()
    вывести т.число_методов()
    вывести т.имя_метода(0)
    вывести т.число_параметров_метода(0)
    переменная типРезультата = т.тип_результата_метода(0)
    вывести типРезультата.имя()
    переменная аргументы = новый Список<Значение>()
    переменная экземпляр = т.создать(аргументы)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeTypeInfoUnknownMethodRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная т = тип Целое
    т.несуществующий_метод()
}
`)
	if err == nil || !strings.Contains(err.Error(), "у \"Тип\" нет метода") {
		t.Fatalf("ожидалась ошибка про неизвестный метод Тип, получено: %v", err)
	}
}

// TestAnalyzeReflectAnyValueUsableAsListElement - Список<Значение> обязан
// компилироваться (аргументы "вызвать"/"создать") - Значение не KindEntity,
// поэтому не натыкается на запрет "элементы коллекций - только примитивы,
// не сущности" (см. package doc internal/semantic/reflect.go).
func TestAnalyzeReflectAnyValueUsableAsListElement(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная список = новый Список<Значение>()
    список.добавить((Значение) 5)
    вывести список.длина()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
