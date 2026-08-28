package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeListIndexReturnsNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = новый Список<Целое>()
    а.добавить(10)
    переменная б = а[0]
    вывести б ?? -1
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDictIndexReturnsNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная карта = новый Словарь<Строка, Целое>()
    карта.установить("а", 1)
    переменная б = карта["а"]
    вывести б ?? -1
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeIndexOnNonCollectionRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = 5
    вывести а[0] ?? -1
}
`)
	if err == nil || !strings.Contains(err.Error(), "индексация") {
		t.Fatalf("ожидалась ошибка индексации не-коллекции, получено: %v", err)
	}
}

func TestAnalyzeIndexWrongIndexTypeRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = новый Список<Целое>()
    вывести а["ключ"] ?? -1
}
`)
	if err == nil || !strings.Contains(err.Error(), "Целое") {
		t.Fatalf("ожидалась ошибка типа индекса, получено: %v", err)
	}
}

func TestAnalyzeIndexUsedDirectlyWithoutCoalesceOrCastRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = новый Список<Целое>()
    а.добавить(1)
    вывести а[0] + 1
}
`)
	if err == nil || !strings.Contains(err.Error(), "не поддерживается") {
		t.Fatalf("ожидалась ошибка компиляции при прямом использовании nullable-результата индексации, получено: %v", err)
	}
}

func TestAnalyzeIndexAssignDesugarsToUstanovit(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = новый Список<Целое>()
    а.добавить(1)
    а[0] = 42
    вывести а.получить(0)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
