package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeNullableVarDeclOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 10
    утверждать а != Ничто
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullLiteralWithoutContextRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = Ничто
}
`)
	if err == nil || !strings.Contains(err.Error(), "неоткуда вывести") {
		t.Fatalf("ожидалась ошибка про отсутствие контекста типа у \"Ничто\", получено: %v", err)
	}
}

func TestAnalyzeNullableUseWithoutNarrowingRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    вывести а + 1
}
`)
	if err == nil || !strings.Contains(err.Error(), "не поддерживается") {
		t.Fatalf("ожидалась ошибка компиляции при использовании nullable-значения без сужения, получено: %v", err)
	}
}

func TestAnalyzeNullableNarrowingInIfAllowsUse(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 5
    если а != Ничто {
        вывести а + 1
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullableNarrowingInElseBranch(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 5
    если а == Ничто {
        вывести 0
    } иначе {
        вывести а + 1
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullCheckOnNonNullableRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = 5
    если а == Ничто {
        вывести 0
    }
}
`)
	if err == nil || !strings.Contains(err.Error(), "nullable-типа") {
		t.Fatalf("ожидалась ошибка сравнения не-nullable значения с \"Ничто\", получено: %v", err)
	}
}

func TestAnalyzeNullCoalesceUnwraps(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    переменная б = а ?? 0
    вывести б + 1
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullCoalesceAssignRequiresNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = 5
    а ?= 10
}
`)
	if err == nil || !strings.Contains(err.Error(), "?=") {
		t.Fatalf("ожидалась ошибка \"?=\" на не-nullable переменной, получено: %v", err)
	}
}

func TestAnalyzeExplicitCastUnwrapsNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    вывести (Целое) а
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullableListElementRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: Список<?Целое> = Ничто
}
`)
	if err == nil || !strings.Contains(err.Error(), "nullable-типов") {
		t.Fatalf("ожидалась ошибка компиляции для \"Список<?Целое>\", получено: %v", err)
	}
}
