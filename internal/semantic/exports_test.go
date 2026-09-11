package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeExportedFunctionWithPrimitiveTypes(t *testing.T) {
	prog, err := AnalyzeSource(t, `
экспортировать функция сумма(а: Целое, б: Строка): Логическое
{
    вернуть истина
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	found := false
	for _, fn := range prog.Functions {
		if fn.Name == "сумма" {
			found = true
			if !fn.Exported {
				t.Fatal("ожидалось Exported == true в IR")
			}
		}
	}
	if !found {
		t.Fatal("не найдена функция \"сумма\" в IR")
	}
}

func TestAnalyzeExportedFunctionWithExportedEntity(t *testing.T) {
	_, err := AnalyzeSource(t, `
экспортировать сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
экспортировать функция новая_точка(x: Целое): Точка
{
    вернуть новая Точка(x: x)
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeExportedFunctionWithNonExportedEntityRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
экспортировать функция новая_точка(x: Целое): Точка
{
    вернуть новая Точка(x: x)
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "не помечена") {
		t.Fatalf("ожидалась ошибка про сущность без \"экспортировать\", получено: %v", err)
	}
}

func TestAnalyzeExportedStructRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
экспортировать структура Точка
{
    имеет:
        x: Целое
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "структура") {
		t.Fatalf("ожидалась ошибка про \"структура\", получено: %v", err)
	}
}

func TestAnalyzeExportedFunctionWithUnexportableTypeRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
экспортировать функция ф(список: Список<Целое>)
{
}
функция главная { вывести "ок" }
`)
	if err == nil {
		t.Fatal("ожидалась ошибка про недопустимый тип generic-коллекции в экспортируемой сигнатуре")
	}
}

// TestAnalyzeExportedMethodOnExportedEntity - метод/конструктор
// экспортированной сущности, оба помечены "экспортировать" - допустимо.
func TestAnalyzeExportedMethodOnExportedEntity(t *testing.T) {
	prog, err := AnalyzeSource(t, `
экспортировать сущность Точка
{
    имеет:
        x: Целое

    экспортировать конструктор(x: Целое)
    {
        этот.x = x
    }

    экспортировать координата(): Целое
    {
        вернуть этот.x
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if !ent.Ctor.Exported {
		t.Fatal("ожидалось Exported == true у конструктора в IR")
	}
	if !ent.Methods[0].Exported {
		t.Fatal("ожидалось Exported == true у метода в IR")
	}
}

// TestAnalyzeExportedMethodOnNonExportedEntityRejected - метод помечен
// "экспортировать", а сама сущность - нет: неявная граница ABI (docs/
// SPEC.md).
func TestAnalyzeExportedMethodOnNonExportedEntityRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    экспортировать координата(): Целое
    {
        вернуть этот.x
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "сама сущность - нет") {
		t.Fatalf("ожидалась ошибка про сущность без \"экспортировать\", получено: %v", err)
	}
}

// TestAnalyzeExportedConstructorOnNonExportedEntityRejected - та же
// проверка, что и для метода, но для конструктора.
func TestAnalyzeExportedConstructorOnNonExportedEntityRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    экспортировать конструктор(x: Целое)
    {
        этот.x = x
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "сама сущность - нет") {
		t.Fatalf("ожидалась ошибка про сущность без \"экспортировать\", получено: %v", err)
	}
}

// TestAnalyzeExportedMethodWithUnexportableTypeRejected - тот же принцип
// проверки типов, что и у экспортируемой функции (checkExportableType),
// теперь и у метода.
func TestAnalyzeExportedMethodWithUnexportableTypeRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
экспортировать сущность Точка
{
    имеет:
        x: Целое

    экспортировать метод(список: Список<Целое>)
    {
    }
}
функция главная { вывести "ок" }
`)
	if err == nil {
		t.Fatal("ожидалась ошибка про недопустимый тип generic-коллекции в сигнатуре экспортируемого метода")
	}
}
