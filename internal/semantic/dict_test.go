package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeDictBasicMethods(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = новый Словарь<Строка, Целое>()
    д.установить("а", 1)
    утверждать д.получить("а") == 1
    утверждать д.содержит("а") == истина
    утверждать д.содержит("б") == ложь
    д.удалить("а")
    утверждать д.длина() == 0
    утверждать д.пусто() == истина
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDictRejectsFloatKey(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = новый Словарь<Действительное, Целое>()
}
`)
	if err == nil || !strings.Contains(err.Error(), "тип ключа") {
		t.Fatalf("ожидалась ошибка про недопустимый тип ключа, получено: %v", err)
	}
}

// TestAnalyzeDictAllowsEntityValueOutsideField - Словарь<К, Сущность>
// допустим как локальная переменная (как и полем другой сущности/структуры -
// см. TestAnalyzeCollectionEntityFieldAllowed в semantic_test.go).
func TestAnalyzeDictAllowsEntityValueOutsideField(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое
}
функция главная
{
    переменная д = новый Словарь<Строка, Точка>()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDictRequiresTwoTypeArgs(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная д = новый Словарь<Строка>()
}
`)
	if err == nil || !strings.Contains(err.Error(), "два аргумента") {
		t.Fatalf("ожидалась ошибка про число аргументов типа, получено: %v", err)
	}
}

func TestAnalyzeDictDoesNotSupportForEach(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная словарь = новый Словарь<Строка, Целое>()
    для x в словаре
    {
        вывести x
    }
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - Словарь не поддерживает \"для x в Y\"")
	}
}
