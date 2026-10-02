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
        Целое x

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
        Целое x
        Целое y

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

// TestAnalyzeFieldByTagMethodsOK - "индекс_поля_по_тегу"/
// "получить_поле_по_тегу" (обратное направление к
// тег_поля/получить_поле) - "?Целое"/"?Значение", доказанная непустота
// ("если... != Ничто") разворачивает "?Значение" даже для "как" (см.
// analyzeAsExpr, найденный этой же фичей живой пробел - "как" раньше не
// звал maybeUnwrapNarrowed вовсе).
func TestAnalyzeFieldByTagMethodsOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        Целое x `+"`"+`sql:x`+"`"+`
}
функция главная
{
    переменная т = тип Точка
    переменная idx = т.индекс_поля_по_тегу("sql", "x")
    если idx != Ничто {
        вывести т.имя_поля(idx)
    }

    переменная точка = новая Точка()
    переменная бокс = (Значение) точка
    переменная поле = бокс.получить_поле_по_тегу("sql", "x")
    если поле != Ничто {
        вывести поле как Целое
    }
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
// (находка "тип ИмяИнтерфейса не существует") теперь
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
// ИмяИнтерфейса" (находка "значение как ИмяИнтерфейса не
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

// TestAnalyzeBaseTypeAndInterfaceReflectionMethodsOK - "базовый_тип"/
// "наследует_ли"/"число_интерфейсов"/"получить_интерфейс"/"реализует_ли"
// (паритет с C# Type.BaseType/IsSubclassOf/GetInterfaces/
// IsAssignableFrom) успешно анализируются - все пять принимают/возвращают
// ожидаемые типы (Тип/Целое/Логическое).
func TestAnalyzeBaseTypeAndInterfaceReflectionMethodsOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Животное
{
    имеет:
        Строка имя
}
сущность Кот наследует Животное реализует Звучащий
{
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция главная
{
    переменная т = тип Кот
    переменная база = т.базовый_тип()
    вывести база.имя()
    вывести т.наследует_ли(тип Животное)
    вывести т.число_интерфейсов()
    переменная интерфейс1 = т.получить_интерфейс(0)
    вывести интерфейс1.имя()
    вывести т.реализует_ли(тип Звучащий)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeEntityInterfacesTransitiveThroughInterfaceInheritance -
// Program.EntityInterfaces (обратный индекс к InterfaceImplementors)
// вычисляется транзитивно тем же entitySatisfies - сущность, реализующая
// только ПРОИЗВОДНЫЙ интерфейс ("Звучащий наследует Именуемый", сущность
// "реализует" только "Звучащий"), всё равно засчитывается как реализующая
// и БАЗОВЫЙ интерфейс "Именуемый" тоже.
func TestAnalyzeEntityInterfacesTransitiveThroughInterfaceInheritance(t *testing.T) {
	prog, err := AnalyzeSource(t, `
интерфейс Именуемый
{
    имя_объекта(): Строка
}
интерфейс Звучащий наследует Именуемый
{
    издать_звук(): Строка
}
сущность Кот реализует Звучащий
{
    имя_объекта(): Строка
    {
        вернуть "Мурка"
    }
    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция главная
{
    переменная т = тип Кот
    вывести т.число_интерфейсов()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ifaces := prog.EntityInterfaces["Кот"]
	if len(ifaces) != 2 {
		t.Fatalf("ожидалось 2 интерфейса (Звучащий + унаследованный Именуемый) у \"Кот\", получено: %+v", ifaces)
	}
	var hasNamed, hasSounding bool
	for _, name := range ifaces {
		hasNamed = hasNamed || name == "Именуемый"
		hasSounding = hasSounding || name == "Звучащий"
	}
	if !hasNamed || !hasSounding {
		t.Fatalf("ожидались \"Именуемый\" и \"Звучащий\" среди EntityInterfaces[\"Кот\"], получено: %+v", ifaces)
	}
}
