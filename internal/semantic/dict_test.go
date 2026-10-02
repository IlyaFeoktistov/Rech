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
        Целое x
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

// TestAnalyzeDictForEachTypes - "для з в словаре" даёт значения (тип В),
// "для к, з в словаре" - ключ (тип К) и значение.
func TestAnalyzeDictForEachTypes(t *testing.T) {
	if _, err := AnalyzeSource(t, `
функция главная
{
    переменная словарь = новый Словарь<Строка, Целое>()
    для ключ, значение в словаре {
        переменная копия: Строка = ключ
        переменная сумма: Целое = значение + 1
    }
    для значение в словаре {
        переменная сумма: Целое = значение + 1
    }
}
`); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная словарь = новый Словарь<Строка, Целое>()
    для значение в словаре {
        переменная ошибка: Строка = значение
    }
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка типа - значение Словарь<Строка, Целое> - Целое, не Строка")
	}
}
