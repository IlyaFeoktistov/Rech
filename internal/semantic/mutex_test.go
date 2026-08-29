package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestAnalyzeMutexFieldTypeChecksAndAppearsFirstInIR - "Блокировка" как тип
// поля пользовательской сущности - должна пройти проверку типов
// (захватить/освободить без аргументов и без результата) и появиться в IR
// ПЕРВОЙ (раньше сущности, у которой она - тип поля) - см. package doc
// internal/semantic/analyzer.go, buildIR, про то, почему порядок важен для
// cbackend (struct-typedef, ссылающийся на неё как на тип поля).
func TestAnalyzeMutexFieldTypeChecksAndAppearsFirstInIR(t *testing.T) {
	irProg := parseProgram(t, `
сущность Счёт
{
    имеет:
        баланс: Целое
        блокировка: Блокировка

    списать(сумма: Целое)
    {
        этот.блокировка.захватить()
        этот.баланс = этот.баланс - сумма
        этот.блокировка.освободить()
    }
}
функция главная
{
    переменная счёт = новый Счёт(0, новая Блокировка())
    счёт.списать(10)
}
`)
	if len(irProg.Entities) < 2 || irProg.Entities[0].Name != "Блокировка" {
		t.Fatalf("ожидалась \"Блокировка\" первой сущностью IR, получено %+v", irProg.Entities)
	}
	if !irProg.Entities[0].IsMutex {
		t.Fatalf("ожидался IsMutex == true, получено %+v", irProg.Entities[0])
	}
}

// TestAnalyzeMutexUsedOnlyInFunctionBodyStillRegistersInIR - регрессионный
// тест на находку сессии: "Блокировка" может встретиться впервые не в поле
// сущности (это зарегистрировало бы её раньше, в collectEntities), а
// внутри тела функции - тоже обязана попасть в IR.
func TestAnalyzeMutexUsedOnlyInFunctionBodyStillRegistersInIR(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная м = новая Блокировка()
    м.захватить()
    м.освободить()
}
`)
	var found *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Блокировка" {
			found = ent
		}
	}
	if found == nil || !found.IsMutex {
		t.Fatalf("ожидалась зарегистрированная \"Блокировка\" в IR, получено %+v", irProg.Entities)
	}
}

// TestAnalyzeMutexUnknownMethodFails - у "Блокировки" только два метода.
func TestAnalyzeMutexUnknownMethodFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная м = новая Блокировка()
    м.освободитьвсё()
}
`)
	if err == nil || !strings.Contains(err.Error(), "нет метода") {
		t.Fatalf("ожидалась ошибка - у \"Блокировки\" нет такого метода, получено: %v", err)
	}
}

// TestAnalyzeMutexMethodsRejectArguments - "захватить"/"освободить" не
// принимают аргументов.
func TestAnalyzeMutexMethodsRejectArguments(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная м = новая Блокировка()
    м.захватить(1)
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка - \"захватить\" не принимает аргументов")
	}
}

// TestAnalyzeUserEntityCannotBeNamedBlokirovka - "Блокировка" зарезервирована
// (internal/builtins.ReservedEntityNames), как и Запрос/Ответ/ДатаВремя -
// нельзя объявить свою сущность с этим именем (иначе она молча подменила бы
// собой системную регистрацию, см. package doc internal/semantic/mutex.go).
func TestAnalyzeUserEntityCannotBeNamedBlokirovka(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Блокировка
{
    имеет:
        х: Целое
}
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя") {
		t.Fatalf("ожидалась ошибка - \"Блокировка\" зарезервированное имя, получено: %v", err)
	}
}
