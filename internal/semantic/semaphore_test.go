package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestAnalyzeSemaphoreFieldTypeChecksAndAppearsFirstInIR - "Семафор" как тип
// поля пользовательской сущности - должен пройти проверку типов
// (захватить/освободить без аргументов и без результата) и появиться в IR
// ПЕРВЫМ (та же причина, что у "Блокировки" - см. package doc
// internal/semantic/analyzer.go, buildIR).
func TestAnalyzeSemaphoreFieldTypeChecksAndAppearsFirstInIR(t *testing.T) {
	irProg := parseProgram(t, `
сущность Пул
{
    имеет:
        счёт: Целое
        семафор: Семафор

    конструктор(счёт: Целое, семафор: Семафор)
    {
        этот.счёт = счёт
        этот.семафор = семафор
    }

    занять()
    {
        этот.семафор.захватить()
        этот.счёт = этот.счёт - 1
    }
}
функция главная
{
    переменная пул = новый Пул(0, новый Семафор(3))
    пул.занять()
}
`)
	if len(irProg.Entities) < 2 || irProg.Entities[0].Name != "Семафор" {
		t.Fatalf("ожидался \"Семафор\" первой сущностью IR, получено %+v", irProg.Entities)
	}
	if !irProg.Entities[0].IsSemaphore {
		t.Fatalf("ожидался IsSemaphore == true, получено %+v", irProg.Entities[0])
	}
}

// TestAnalyzeSemaphoreUsedOnlyInFunctionBodyStillRegistersInIR - регрессионный
// тест на находку, уже проверенную для "Блокировки" (см. mutex_test.go):
// "Семафор" может встретиться впервые не в поле сущности, а внутри тела
// функции - тоже обязан попасть в IR.
func TestAnalyzeSemaphoreUsedOnlyInFunctionBodyStillRegistersInIR(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная сем = новый Семафор(1)
    сем.захватить()
    сем.освободить()
}
`)
	var found *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Семафор" {
			found = ent
		}
	}
	if found == nil || !found.IsSemaphore {
		t.Fatalf("ожидался зарегистрированный \"Семафор\" в IR, получено %+v", irProg.Entities)
	}
}

// TestAnalyzeSemaphoreUnknownMethodFails - у "Семафора" только два метода.
func TestAnalyzeSemaphoreUnknownMethodFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная сем = новый Семафор(1)
    сем.освободитьвсё()
}
`)
	if err == nil || !strings.Contains(err.Error(), "нет метода") {
		t.Fatalf("ожидалась ошибка - у \"Семафора\" нет такого метода, получено: %v", err)
	}
}

// TestAnalyzeSemaphoreMethodsRejectArguments - "захватить"/"освободить" не
// принимают аргументов.
func TestAnalyzeSemaphoreMethodsRejectArguments(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная сем = новый Семафор(1)
    сем.захватить(1)
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка - \"захватить\" не принимает аргументов")
	}
}

// TestAnalyzeSemaphoreCtorRequiresInitialValue - в отличие от "Блокировки"
// (конструктор без аргументов), "Семафор" обязан получить ровно одно
// начальное значение.
func TestAnalyzeSemaphoreCtorRequiresInitialValue(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная сем = новый Семафор()
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка - конструктору \"Семафор\" нужно начальное значение")
	}
}

// TestAnalyzeUserEntityCannotBeNamedSemafor - "Семафор" зарезервировано
// (internal/builtins.ReservedEntityNames), как и "Блокировка" - нельзя
// объявить свою сущность с этим именем.
func TestAnalyzeUserEntityCannotBeNamedSemafor(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Семафор
{
    имеет:
        х: Целое
}
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя") {
		t.Fatalf("ожидалась ошибка - \"Семафор\" зарезервированное имя, получено: %v", err)
	}
}
