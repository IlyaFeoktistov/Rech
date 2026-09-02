package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeListLiteralNonEmptyInfersType(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = [1, 2, 3]
    вывести а.получить(0)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeListLiteralEmptyWithAnnotationOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: Список<Целое> = []
    вывести а.длина()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeListLiteralEmptyWithoutAnnotationRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = []
}
`)
	if err == nil || !strings.Contains(err.Error(), "требует известный тип") {
		t.Fatalf("ожидалась ошибка про пустой литерал без известного типа, получено: %v", err)
	}
}

func TestAnalyzeListLiteralMixedTypesRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = [1, "два"]
}
`)
	if err == nil || !strings.Contains(err.Error(), "строго типизированы") {
		t.Fatalf("ожидалась ошибка смешанных типов, получено: %v", err)
	}
}

func TestAnalyzeListLiteralEntityElementRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка {
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
функция главная {
    переменная а = новая Точка(1)
    переменная б = [а]
}
`)
	if err == nil || !strings.Contains(err.Error(), "пользовательских сущностей") {
		t.Fatalf("ожидалась ошибка про недопустимый элемент-сущность, получено: %v", err)
	}
}
