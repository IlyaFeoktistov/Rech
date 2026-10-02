package semantic

import (
	"strings"
	"testing"
)

// TestAnalyzeEntityEqualityComparesByReference - НАЙДЕННЫЙ по ходу
// examples/проекты/чат_сервер недочёт: "=="/"!=" не
// поддерживался ни для одной сущности вовсе. Сущность/класс (ссылочная
// семантика) теперь сравнивается по ссылке - тем же принципом, что
// entityIsRefType (cast.go).
func TestAnalyzeEntityEqualityComparesByReference(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        Целое x

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
функция главная
{
    переменная а = новая Точка(1)
    переменная б = а
    утверждать а == б
    утверждать не (а == новая Точка(1))
    утверждать а != новая Точка(1)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeStructEqualityFieldByField - "структура" (значение, не ссылка) -
// "=="/"!=" сравнивает поле за полем, а не по identity -
// две РАЗНЫЕ структуры с одинаковыми полями равны, structFieldsEquatable
// (cast.go) допускает примитивные/Строка поля без ограничений.
func TestAnalyzeStructEqualityFieldByField(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Точка
{
    имеет:
        Целое x
        Строка имя
}
функция главная
{
    переменная а = новая Точка(1, "а")
    переменная б = новая Точка(1, "а")
    утверждать а == б
    утверждать не (а != б)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeStructEqualityNestedStruct - вложенная "структура" полем -
// рекурсивное сравнение (structFieldsEquatable рекурсирует в неё же).
func TestAnalyzeStructEqualityNestedStruct(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Точка
{
    имеет:
        Целое x
        Целое y
}
структура Отрезок
{
    имеет:
        Точка начало
        Точка конец
}
функция главная
{
    переменная а = новый Отрезок(новая Точка(0, 0), новая Точка(1, 1))
    переменная б = новый Отрезок(новая Точка(0, 0), новая Точка(1, 1))
    утверждать а == б
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeStructEqualityAllowsGenericCollectionField - generic-коллекция
// полем ("товары: Список<Строка>") - тоже KindEntity, той же ref-семантикой,
// что и обычная сущность полем (entityIsRefType, cast.go считает generic-
// инстанциацию ссылочной безусловно) - разрешена, сравнивается по IDENTITY
// самой коллекции (тот же ли это список), не по содержимому - тот же
// принцип, что и у обычного ref-поля в generateIRStructEqFunc.
func TestAnalyzeStructEqualityAllowsGenericCollectionField(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Корзина
{
    имеет:
        Список<Строка> товары
}
функция главная
{
    переменная список = новый Список<Строка>()
    переменная а = новая Корзина(список)
    переменная б = новая Корзина(список)
    утверждать а == б
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeStructEqualityRejectsUnsupportedField - структура с полем,
// которое НЕ сравнимо field-by-field (nullable - ни equatableTypes, ни
// KindEntity) - "==" отвергается компиляцией с понятным именем проблемного
// поля.
func TestAnalyzeStructEqualityRejectsUnsupportedField(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Корзина
{
    имеет:
        ?Целое скидка
}
функция главная
{
    переменная а = новая Корзина(Ничто)
    переменная б = новая Корзина(Ничто)
    вывести а == б
}
`)
	if err == nil || !strings.Contains(err.Error(), "скидка") {
		t.Fatalf("ожидалась ошибка про несравнимое поле \"скидка\", получено: %v", err)
	}
}

// TestAnalyzeGenericCollectionEqualityComparesByReference - Список<T> и
// подобные - тоже ссылочные (ctx.entityIsRef, cbackend), значит тот же
// принцип: "==" сравнивает, тот ли это САМЫЙ список, не совпадение
// содержимого.
func TestAnalyzeGenericCollectionEqualityComparesByReference(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новый Список<Целое>()
    переменная б = а
    переменная третий = новый Список<Целое>()
    утверждать а == б
    утверждать а != третий
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeEntityEqualityRequiresSameType - разные типы сущностей - всё
// ещё ошибка компиляции (та же ветка "lt != rt", что и раньше).
func TestAnalyzeEntityEqualityRequiresSameType(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность А {}
сущность Б {}
функция главная
{
    переменная а = новая А()
    переменная б = новый Б()
    вывести а == б
}
`)
	if err == nil || !strings.Contains(err.Error(), "не поддерживается для типов") {
		t.Fatalf("ожидалась ошибка про несовпадающие типы, получено: %v", err)
	}
}
