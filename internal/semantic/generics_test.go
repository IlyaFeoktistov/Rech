package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeGenericEntityInstantiation(t *testing.T) {
	prog := parseProgram(t, `
сущность Коробка<T>
{
    имеет:
        содержимое: T

    получить(): T
    {
        вернуть этот.содержимое
    }
}
функция главная
{
    переменная коробка = новая Коробка<Целое>(5)
    вывести коробка.получить()
}
`)
	var inst *ir.Entity
	for _, ent := range prog.Entities {
		if ent.Name == "Коробка<Целое>" {
			inst = ent
		}
	}
	if inst == nil {
		t.Fatalf("ожидалась инстанциация \"Коробка<Целое>\", получено %+v", prog.Entities)
	}
	if len(inst.Fields) != 1 || inst.Fields[0].Type != ir.TypeInt {
		t.Fatalf("ожидалось поле содержимое:Целое, получено %+v", inst.Fields)
	}
}

func TestAnalyzeGenericFunctionInstantiation(t *testing.T) {
	prog := parseProgram(t, `
функция первый<T>(список: Список<T>): T
{
    вернуть список.получить(0)
}
функция главная
{
    переменная сп = новый Список<Строка>()
    сп.добавить("привет")
    вывести первый<Строка>(сп)
}
`)
	var inst *ir.Function
	for _, fn := range prog.Functions {
		if fn.Name == "первый<Строка>" {
			inst = fn
		}
	}
	if inst == nil {
		t.Fatalf("ожидалась инстанциация \"первый<Строка>\", получено %+v", prog.Functions)
	}
	if inst.ReturnType != ir.TypeString || !inst.HasReturn {
		t.Fatalf("ожидался тип результата Строка, получено %s", inst.ReturnType)
	}
}

func TestAnalyzeGenericMultipleParamsAndNesting(t *testing.T) {
	prog := parseProgram(t, `
сущность Пара<A, Б>
{
    имеет:
        первый: A
        второй: Б
}
функция главная
{
    переменная внутрь = новая Пара<Целое, Целое>(1, 2)
    переменная снаружи = новая Пара<Целое, Пара<Целое, Целое>>(0, внутрь)
    вывести снаружи.второй.первый
}
`)
	names := map[string]bool{}
	for _, ent := range prog.Entities {
		names[ent.Name] = true
	}
	if !names["Пара<Целое, Целое>"] || !names["Пара<Целое, Пара<Целое, Целое>>"] {
		t.Fatalf("ожидались обе инстанциации, получено %+v", names)
	}
}

func TestAnalyzeGenericWrongTypeArgCountFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Коробка<T>
{
    имеет:
        содержимое: T
}
функция главная
{
    переменная коробка = новая Коробка<Целое, Строка>(1, "х")
}
`)
	if err == nil || !strings.Contains(err.Error(), "ожидает параметров типа") {
		t.Fatalf("ожидалась ошибка числа параметров типа, получено: %v", err)
	}
}

func TestAnalyzeGenericConstraintRejectsMismatch(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция первый<T как Целое|Строка>(x: T): T
{
    вернуть x
}
функция главная
{
    переменная х = первый<Логическое>(истина)
}
`)
	if err == nil || !strings.Contains(err.Error(), "не удовлетворяет ограничению") {
		t.Fatalf("ожидалась ошибка ограничения типа, получено: %v", err)
	}
}

func TestAnalyzeGenericConstraintAcceptsPrimitiveMatch(t *testing.T) {
	parseProgram(t, `
функция первый<T как Целое|Строка>(x: T): T
{
    вернуть x
}
функция главная
{
    переменная х = первый<Строка>("привет")
    вывести х
}
`)
}

func TestAnalyzeGenericInterfaceConstraintAcceptsImplementer(t *testing.T) {
	parseProgram(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Кошка реализует Звучащий
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция крикнуть<T как Звучащий>(x: T): Строка
{
    вернуть x.издать_звук()
}
функция главная
{
    переменная кот = новая Кошка()
    вывести крикнуть<Кошка>(кот)
}
`)
}

func TestAnalyzeGenericInterfaceConstraintRejectsNonImplementer(t *testing.T) {
	_, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Камень
{
    имеет:
        вес: Целое
}
функция крикнуть<T как Звучащий>(x: T): Строка
{
    вернуть x.издать_звук()
}
функция главная
{
    переменная камень = новый Камень(1)
    вывести крикнуть<Камень>(камень)
}
`)
	if err == nil || !strings.Contains(err.Error(), "не удовлетворяет ограничению") {
		t.Fatalf("ожидалась ошибка ограничения типа, получено: %v", err)
	}
}

func TestAnalyzeGenericFieldTypeMismatchStillCaught(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Коробка<T>
{
    имеет:
        содержимое: T
}
функция главная
{
    переменная коробка = новая Коробка<Целое>("не число")
}
`)
	if err == nil || !strings.Contains(err.Error(), "должен быть типа") {
		t.Fatalf("ожидалась обычная ошибка несовпадения типа поля, получено: %v", err)
	}
}
