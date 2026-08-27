package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestAnalyzeSelfReturnOwnMethodResolvesToOwnEntity - "имя(...): этот" у
// метода сущности X резолвится в EntityType(X) для самой X.
func TestAnalyzeSelfReturnOwnMethodResolvesToOwnEntity(t *testing.T) {
	irProg := parseProgram(t, `
сущность Точка
{
    задать(): этот
    {
        вернуть этот
    }
}
функция главная
{
    переменная точка = новая Точка()
    переменная снова = точка.задать()
}
`)
	var точка *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Точка" {
			точка = ent
		}
	}
	if точка == nil || len(точка.Methods) != 1 {
		t.Fatalf("сущность Точка не найдена")
	}
	m := точка.Methods[0]
	if !m.HasReturn || m.ReturnType != ir.EntityType("Точка") {
		t.Fatalf("ожидался тип результата EntityType(Точка), получено HasReturn=%v ReturnType=%v", m.HasReturn, m.ReturnType)
	}
}

// TestAnalyzeSelfReturnInheritedMethodResolvesToDerivedEntityAtCallSite -
// сценарий из явного запроса пользователя: наследник вызывает
// НЕПЕРЕОПРЕДЕЛЁННЫЙ метод базовой сущности с "этот" как типом результата -
// результат типизируется КОНКРЕТНЫМ (производным) типом получателя, а не
// базовым, что позволяет вызвать следующим метод, объявленный только у
// производной сущности (это и есть цель фичи - "цепочки" через
// наследование).
func TestAnalyzeSelfReturnInheritedMethodResolvesToDerivedEntityAtCallSite(t *testing.T) {
	irProg := parseProgram(t, `
сущность СтроительА
{
    имеет:
        имя_поле: Строка

    задать_имя(значение: Строка): этот
    {
        этот.имя_поле = значение
        вернуть этот
    }
}
сущность СтроительБ наследует СтроительА
{
    имеет:
        доп_поле: Целое

    задать_доп(значение: Целое): этот
    {
        этот.доп_поле = значение
        вернуть этот
    }
}
функция главная
{
    переменная б = новый СтроительБ("", 0)
    переменная б2 = б.задать_имя("х")
    б2.задать_доп(5)
}
`)
	_ = irProg
}

// TestAnalyzeSelfReturnMismatchedOverrideRejected - переопределение метода
// с "этот" как типом результата конкретным именем типа (даже своим
// собственным) - ошибка сигнатуры, не эквивалент.
func TestAnalyzeSelfReturnMismatchedOverrideRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность СтроительА
{
    задать(): этот
    {
        вернуть этот
    }
}
сущность СтроительБ наследует СтроительА
{
    задать(): СтроительБ
    {
        вернуть этот
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "не совпадает по сигнатуре") {
		t.Fatalf("ожидалась ошибка несовпадения сигнатуры (\"этот\" vs конкретный тип), получено: %v", err)
	}
}

// TestAnalyzeSelfReturnInterfaceMethodResolvesToInterfaceType - у метода
// интерфейса "этот" резолвится в сам интерфейс (самое узкое, что можно
// статически утверждать через интерфейсное значение - конкретный тип
// реализации неизвестен на этом уровне).
func TestAnalyzeSelfReturnInterfaceMethodResolvesToInterfaceType(t *testing.T) {
	irProg := parseProgram(t, `
интерфейс Именуемый
{
    задать_имя(значение: Строка): этот
}
сущность Штука реализует Именуемый
{
    имеет:
        имя_поле: Строка

    задать_имя(значение: Строка): этот
    {
        этот.имя_поле = значение
        вернуть этот
    }
}
функция настроить(х: Именуемый): Именуемый
{
    вернуть х.задать_имя("х")
}
функция главная
{
    переменная ш = новая Штука("")
    переменная результат = настроить(ш)
}
`)
	var iface *ir.Interface
	for _, i := range irProg.Interfaces {
		if i.Name == "Именуемый" {
			iface = i
		}
	}
	if iface == nil || len(iface.Methods) != 1 {
		t.Fatalf("интерфейс Именуемый не найден")
	}
	m := iface.Methods[0]
	if !m.HasReturn || m.ReturnType != ir.InterfaceType("Именуемый") {
		t.Fatalf("ожидался тип результата InterfaceType(Именуемый), получено HasReturn=%v ReturnType=%v", m.HasReturn, m.ReturnType)
	}
}

// TestAnalyzeSelfReturnEntitySatisfiesInterfaceWithSelfReturn - реализация
// с "этот" удовлетворяет контракту интерфейса с "этот" (сравниваются по
// факту self-возврата, не по конкретному резолвленному типу - см. package
// doc sigsMatch).
func TestAnalyzeSelfReturnEntitySatisfiesInterfaceWithSelfReturn(t *testing.T) {
	_ = parseProgram(t, `
интерфейс Именуемый
{
    задать_имя(значение: Строка): этот
}
сущность Штука реализует Именуемый
{
    задать_имя(значение: Строка): этот
    {
        вернуть этот
    }
}
функция главная { вывести "ок" }
`)
}
