package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeTypeOfExprOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная т = тип Целое
    вывести т.имя()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeTypeOfUnknownTypeRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная т = тип НетТакогоТипа
}
`)
	if err == nil {
		t.Fatal("ожидалась ошибка - неизвестный тип")
	}
}

func TestAnalyzeBoxCastAnyValueOK(t *testing.T) {
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
функция главная
{
    переменная а = (Значение) 5
    переменная б = (Значение) "текст"
    переменная точка = новая Точка(1)
    переменная г = (Значение) точка
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeAsExprRequiresAnyReceiver(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = 5
    переменная б = а как Целое
}
`)
	if err == nil || !strings.Contains(err.Error(), "\"как\" применим только к \"Значение\"") {
		t.Fatalf("ожидалась ошибка про \"как\" не на Значение, получено: %v", err)
	}
}

func TestAnalyzeAsExprRoundTripOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная коробка = (Значение) 5
    переменная обратно = коробка как Целое
    вывести обратно
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeAnyMethodsOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная коробка = (Значение) 5
    переменная т = коробка.тип_значения()
    вывести т.имя()
    переменная аргументы = новый Список<Значение>()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeAnyUnknownMethodRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная коробка = (Значение) 5
    коробка.несуществующий_метод()
}
`)
	if err == nil || !strings.Contains(err.Error(), "у \"Значение\" нет метода") {
		t.Fatalf("ожидалась ошибка про неизвестный метод Значение, получено: %v", err)
	}
}

func TestAnalyzeTypeInfoMethodsOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое
        y: Целое

    длина(): Целое
    {
        вернуть этот.x
    }
}
функция главная
{
    переменная т = тип Точка
    вывести т.число_полей()
    вывести т.имя_поля(0)
    переменная типПоля = т.тип_поля(0)
    вывести типПоля.имя()
    вывести т.тег_поля(0, "sql")
    вывести т.число_методов()
    вывести т.имя_метода(0)
    вывести т.число_параметров_метода(0)
    переменная типРезультата = т.тип_результата_метода(0)
    вывести типРезультата.имя()
    переменная аргументы = новый Список<Значение>()
    переменная экземпляр = т.создать(аргументы)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeTypeInfoUnknownMethodRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная т = тип Целое
    т.несуществующий_метод()
}
`)
	if err == nil || !strings.Contains(err.Error(), "у \"Тип\" нет метода") {
		t.Fatalf("ожидалась ошибка про неизвестный метод Тип, получено: %v", err)
	}
}

// TestAnalyzeReflectAnyValueUsableAsListElement - Список<Значение> обязан
// компилироваться (аргументы "вызвать"/"создать") - Значение не KindEntity,
// поэтому не натыкается на запрет "элементы коллекций - только примитивы,
// не сущности" (см. package doc internal/semantic/reflect.go).
func TestAnalyzeReflectAnyValueUsableAsListElement(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная список = новый Список<Значение>()
    список.добавить((Значение) 5)
    вывести список.длина()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeTypeOfInterfaceRegistersReflectTarget - "тип ИмяИнтерфейса"
// (docs/ROADMAP.md - находка "тип ИмяИнтерфейса не существует") теперь
// разрешён и регистрирует KindInterface в ReflectTargets (backend'у нужен
// дескриптор), не только KindEntity, как было раньше.
func TestAnalyzeTypeOfInterfaceRegistersReflectTarget(t *testing.T) {
	prog, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
функция главная
{
    переменная т = тип Звучащий
    вывести т.имя()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	found := false
	for _, target := range prog.ReflectTargets {
		if target == ir.InterfaceType("Звучащий") {
			found = true
		}
	}
	if !found {
		t.Fatalf("ожидался \"Звучащий\" в ReflectTargets, получено: %+v", prog.ReflectTargets)
	}
}

// TestAnalyzeAsInterfaceOKAndComputesImplementors - "значение как
// ИмяИнтерфейса" (docs/ROADMAP.md - находка "значение как ИмяИнтерфейса не
// распаковывает боксированную реализацию") теперь разрешён для боксированной
// КОНКРЕТНОЙ реализации, и Program.InterfaceImplementors вычисляется
// транзитивно (entitySatisfies) для каждого объявленного интерфейса.
func TestAnalyzeAsInterfaceOKAndComputesImplementors(t *testing.T) {
	prog, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Кот реализует Звучащий
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция главная
{
    переменная коробка = (Значение) новый Кот()
    переменная звучащий = коробка как Звучащий
    вывести звучащий.издать_звук()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	impls := prog.InterfaceImplementors["Звучащий"]
	if len(impls) != 1 || impls[0] != "Кот" {
		t.Fatalf("ожидался implementors [\"Кот\"] у \"Звучащий\", получено: %+v", impls)
	}
}

// TestAnalyzeBoxInterfaceTypedValueRejected - боксирование "(Значение) x",
// где x статически типизирован ИНТЕРФЕЙСОМ, отклоняется явной ошибкой
// компиляции (не тихий неверный код) - см. package doc analyzeCastExpr,
// внутри cast.go, "ИСКЛЮЧЕНИЕ - from.Kind == ir.KindInterface".
func TestAnalyzeBoxInterfaceTypedValueRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Кот реализует Звучащий
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция обработать(з: Звучащий)
{
    переменная коробка = (Значение) з
}
функция главная
{
    обработать(новый Кот())
}
`)
	if err == nil || !strings.Contains(err.Error(), "интерфейсно-типизированное") {
		t.Fatalf("ожидалась ошибка про боксирование интерфейсного значения, получено: %v", err)
	}
}
