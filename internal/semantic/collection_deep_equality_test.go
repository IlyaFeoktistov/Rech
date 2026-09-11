package semantic

import (
	"strings"
	"testing"
)

// TestAnalyzeListEqualComparesContent - "равен" у "Список<T>" (docs/
// найдено при обсуждении юнит-тестирования: "==" сравнивает
// Список<T> по identity, содержимое сравнить было нечем без ручного цикла)
// - метод существует и типизирован как Логическое(Список<T>).
func TestAnalyzeListEqualComparesContent(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = [1, 2, 3]
    переменная б = [1, 2, 3]
    утверждать а.равен(б)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeListEqualNestedList - элемент сам "Список<T>" - "равен"
// внешнего списка доступен, только если внутренний тоже Equatable
// (elemEquatable, cast.go) - у Список<Целое> он всегда таков.
func TestAnalyzeListEqualNestedList(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = [[1, 2], [3, 4]]
    переменная б = [[1, 2], [3, 4]]
    утверждать а.равен(б)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeListEqualStructElement - элемент "структура" - "равен"
// доступен, structFieldsEquatable уже проверила её поля (та же проверка,
// что и у "==" самой структуры).
func TestAnalyzeListEqualStructElement(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Точка
{
    имеет:
        x: Целое
}
функция главная
{
    переменная а = [новая Точка(1)]
    переменная б = [новая Точка(1)]
    утверждать а.равен(б)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeListEqualRejectsUncomparableElement - элемент "Вызываемый(...)"
// - elemEquatable отказывает (не примитив/Строка/сущность/коллекция), "равен"
// не регистрируется вовсе у ЭТОЙ инстанциации - вызов ловится как обычный
// "неизвестный метод", той же общей проверкой, что и у любого несуществующего
// метода, без отдельного специального сообщения.
func TestAnalyzeListEqualRejectsUncomparableElement(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция применить(x: Целое): Целое
{
    вернуть x
}
функция главная
{
    переменная а: Список<Вызываемый(Целое): Целое> = [&применить]
    переменная б: Список<Вызываемый(Целое): Целое> = [&применить]
    вывести а.равен(б)
}
`)
	if err == nil || !strings.Contains(err.Error(), "равен") {
		t.Fatalf("ожидалась ошибка про отсутствующий метод \"равен\", получено: %v", err)
	}
}

// TestAnalyzeDictEqualComparesContent - "равен" у "Словарь<К, В>" - тот же
// принцип, что и у Список<T> выше, только по значению В (ключ К уже всегда
// сравним - dictKeyKinds ограничивает его примитивами).
func TestAnalyzeDictEqualComparesContent(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новый Словарь<Строка, Целое>()
    переменная б = новый Словарь<Строка, Целое>()
    а.установить("x", 1)
    б.установить("x", 1)
    утверждать а.равен(б)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
