// Package semantic: рефлексия ("тип <Имя>", "значение как <Имя>", методы
// встроенных "Значение"/"Тип") - см. docs/SPEC.md. "Значение" (KindAny) -
// боксирование значения любого типа, "Тип" (KindTypeInfo) - дескриптор
// типа (имя, поля, методы). Оба - не KindEntity: элементы встроенных
// generic-коллекций ограничены примитивами ("не для сущностей", см.
// resolveGenericTypeName), а Список<Значение> обязан работать (аргументы
// вызова метода/конструктора по рефлексии) - будь Значение сущностью,
// этот же запрет отверг бы Список<Значение>.
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// listOfAnyType - "Список<Значение>", тип аргументов "вызвать"/"создать" -
// строковый ключ ir.Type.Entity собирается тем же способом, что и у
// resolveGenericTypeName ("Шаблон<Аргумент>"), поэтому совпадает с тем,
// что получает реальный "новый Список<Значение>()" в коде пользователя -
// сравнение типов через "==" остаётся простым равенством строки.
var listOfAnyType = ir.EntityType("Список<Значение>")

// anyMethodSigs - методы "Значение" (боксированное значение любого типа) -
// фиксированный, не generic набор (в отличие от методов Список<T>/
// Стопка<T>, которые строит genericMethodSigs под конкретный ElemType).
var anyMethodSigs = map[string]funcSig{
	// Не "тип" - оно уже зарезервировано лексером под "тип <Имя>"
	// (KwType), после "." требуется обычный IDENT.
	"тип_значения":    {Params: nil, ReturnType: ir.TypeTypeInfo, HasReturn: true},
	"получить_поле":   {Params: []ir.Type{ir.TypeString}, ReturnType: ir.TypeAny, HasReturn: true},
	"установить_поле": {Params: []ir.Type{ir.TypeString, ir.TypeAny}, HasReturn: false},
	"вызвать":         {Params: []ir.Type{ir.TypeString, listOfAnyType}, ReturnType: ir.TypeAny, HasReturn: true},
}

// typeInfoMethodSigs - методы "Тип" (дескриптор типа - имя, поля, методы).
// Поля/методы - по индексу (число_полей/имя_поля/тип_поля и аналоги для
// методов), не как Список<ПолеИнфо> - сущности как элементы встроенных
// коллекций не поддерживаются (см. package doc), а заводить под одну эту
// фичу исключение из общего правила - лишняя связанность.
var typeInfoMethodSigs = map[string]funcSig{
	"имя":         {Params: nil, ReturnType: ir.TypeString, HasReturn: true},
	"число_полей": {Params: nil, ReturnType: ir.TypeInt, HasReturn: true},
	"имя_поля":    {Params: []ir.Type{ir.TypeInt}, ReturnType: ir.TypeString, HasReturn: true},
	"тип_поля":    {Params: []ir.Type{ir.TypeInt}, ReturnType: ir.TypeTypeInfo, HasReturn: true},
	// "тег_поля" - значение ключа тега (`json:name, sql:user_name`, см.
	// internal/structtag) у поля по индексу - "" нет тега или в нём нет
	// такого ключа. Нужен библиотекам вроде ORM/сериализаторов, написанным
	// на самом Rech: они не могут прочитать ir.Field.Tag напрямую (это
	// компилятор), но могут пройти по полям через рефлексию и спросить свой
	// собственный ключ тега у каждого.
	"тег_поля": {Params: []ir.Type{ir.TypeInt, ir.TypeString}, ReturnType: ir.TypeString, HasReturn: true},
	// "тег_поля_текст" - тот же тег поля по индексу, но СЫРЫМ ТЕКСТОМ целиком
	// (без разбора по ключу) - docs/ROADMAP.md: "тег_поля(i, ключ)" уже
	// требует ЗНАТЬ ключ заранее, библиотеке, которая хочет пройтись по
	// ВСЕМ ключам тега (не только своему одному) или разобрать его
	// собственным форматом, нужен доступ к исходной строке - тем же
	// принципом, что и у "тег_метода_текст"/"тег_текст" ниже.
	"тег_поля_текст":          {Params: []ir.Type{ir.TypeInt}, ReturnType: ir.TypeString, HasReturn: true},
	"число_методов":           {Params: nil, ReturnType: ir.TypeInt, HasReturn: true},
	"имя_метода":              {Params: []ir.Type{ir.TypeInt}, ReturnType: ir.TypeString, HasReturn: true},
	"число_параметров_метода": {Params: []ir.Type{ir.TypeInt}, ReturnType: ir.TypeInt, HasReturn: true},
	"тип_параметра_метода":    {Params: []ir.Type{ir.TypeInt, ir.TypeInt}, ReturnType: ir.TypeTypeInfo, HasReturn: true},
	"тип_результата_метода":   {Params: []ir.Type{ir.TypeInt}, ReturnType: ir.TypeTypeInfo, HasReturn: true},
	// "тег_метода"/"тег_метода_текст" - тот же тег, что уже был у поля, но
	// метода (docs/ROADMAP.md, паритет с C#-атрибутами на методе - у
	// метода тег в языке уже существует синтаксически, internal/parser,
	// служит и служебным ключам компилятора вроде "противоположность", но
	// не был виден рефлексии до этого шага).
	"тег_метода":       {Params: []ir.Type{ir.TypeInt, ir.TypeString}, ReturnType: ir.TypeString, HasReturn: true},
	"тег_метода_текст": {Params: []ir.Type{ir.TypeInt}, ReturnType: ir.TypeString, HasReturn: true},
	"создать":          {Params: []ir.Type{listOfAnyType}, ReturnType: ir.TypeAny, HasReturn: true},
	// "тег"/"тег_текст" - тег самой сущности/структуры целиком (не поля/
	// метода) - docs/ROADMAP.md, паритет с C#-атрибутами на классе:
	// "сущность Имя `ключ:значение`" - новый синтаксис (ast.EntityDecl.Tag),
	// у интерфейса/примитива тега нет - "" везде.
	"тег":       {Params: []ir.Type{ir.TypeString}, ReturnType: ir.TypeString, HasReturn: true},
	"тег_текст": {Params: nil, ReturnType: ir.TypeString, HasReturn: true},
	// "базовый_тип"/"наследует_ли"/"число_интерфейсов"/"получить_интерфейс"/
	// "реализует_ли" - docs/ROADMAP.md, паритет с C# Type.BaseType/
	// IsSubclassOf/GetInterfaces/IsAssignableFrom - тем же принципом
	// раздельных отношений, что уже есть у самого языка ("наследует" у
	// сущности, "реализует" у интерфейса), не единый общий
	// IsAssignableFrom. "_ли" - "наследует"/"реализует" САМИ ПО СЕБЕ уже
	// зарезервированы лексером (KwInherits/KwImplements, объявление
	// сущности/интерфейса) - "после '.' ожидался IDENT" без суффикса.
	// Действуют ТОЛЬКО на сущность/структуру - у примитива и у интерфейса
	// (сам интерфейс не наследует сущность и не реализует другой интерфейс
	// тем же словом - "наследует" у интерфейса расширяет контракт, не эта
	// категория) базовый_тип - "Пусто", число_интерфейсов - 0, наследует_
	// ли/реализует_ли - всегда "ложь".
	"базовый_тип":        {Params: nil, ReturnType: ir.TypeTypeInfo, HasReturn: true},
	"наследует_ли":       {Params: []ir.Type{ir.TypeTypeInfo}, ReturnType: ir.TypeBool, HasReturn: true},
	"число_интерфейсов":  {Params: nil, ReturnType: ir.TypeInt, HasReturn: true},
	"получить_интерфейс": {Params: []ir.Type{ir.TypeInt}, ReturnType: ir.TypeTypeInfo, HasReturn: true},
	"реализует_ли":       {Params: []ir.Type{ir.TypeTypeInfo}, ReturnType: ir.TypeBool, HasReturn: true},
}

// analyzeTypeOfExpr разбирает "тип <Имя>" - дескриптор типа Имя, известного
// уже во время компиляции (аналог typeof(T)). Имя резолвится тем же
// resolveTypeName, что и любое другое упоминание типа - сущность/структура
// ИЛИ интерфейс (docs/ROADMAP.md - находка "тип ИмяИнтерфейса не
// существует") регистрируется в Program.ReflectTargets (backend генерирует
// под неё дескриптор), примитив - нет (его дескриптор эмитится всегда).
func analyzeTypeOfExpr(sc *ctx, e *ast.TypeOfExpr) (ir.Expr, error) {
	t, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, e.TypeName)
	if err != nil {
		return nil, fmt.Errorf("строка %d: \"тип %s\": %w", e.Line, e.TypeName, err)
	}
	sc.a.markUsesReflection()
	if t.Kind == ir.KindEntity || t.Kind == ir.KindInterface {
		sc.a.registerReflectTarget(t)
	}
	return &ir.TypeOfExpr{Type: t}, nil
}

// analyzeAsExpr разбирает "<значение> как <Имя>" - обычно разбоксирование
// "Значение" обратно к конкретному типу Имя, с проверкой в рантайме
// (несовпадение реального типа - паника): Value обязан быть типа
// "Значение" - "как" не общий оператор приведения (для этого уже есть
// "(Тип) значение"), только парный к боксирующему касту. ОТДЕЛЬНОЕ
// исключение - Value типа dbResultType ("Список<Словарь<Строка,
// Значение>>", результат "запросить"/"выполнить", db.go): "запросить(...)
// как Список<Тип>" - маппинг строк результата в список сущностей, см.
// db_rows.go - проверяется и обрабатывается раньше общей ветки выше
// (dbResultType - не "Значение", KindEntity).
func analyzeAsExpr(sc *ctx, e *ast.AsExpr) (ir.Expr, error) {
	value, err := analyzeExpr(sc, e.Value)
	if err != nil {
		return nil, err
	}
	if valType := ir.TypeOf(value); valType.Kind == ir.KindEntity && valType.Entity == dbResultType {
		to, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, e.TypeName)
		if err != nil {
			return nil, fmt.Errorf("строка %d: \"как %s\": %w", e.Line, e.TypeName, err)
		}
		sig, ok := sc.entities[to.Entity]
		if to.Kind != ir.KindEntity || !ok || !sig.IsList {
			return nil, fmt.Errorf("строка %d: результат \"запросить\"/\"выполнить\" можно привести только к \"Список<Тип>\" (маппинг строк по полям), получено %s", e.Line, to)
		}
		return analyzeDBRowsAsExpr(sc, value, to, sig.ElemType, e.Line)
	}
	if ir.TypeOf(value).Kind != ir.KindAny {
		return nil, fmt.Errorf("строка %d: \"как\" применим только к \"Значение\", получено %s", e.Line, ir.TypeOf(value))
	}
	to, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, e.TypeName)
	if err != nil {
		return nil, fmt.Errorf("строка %d: \"как %s\": %w", e.Line, e.TypeName, err)
	}
	sc.a.markUsesReflection()
	// KindInterface (docs/ROADMAP.md - находка "значение как ИмяИнтерфейса
	// не распаковывает боксированную реализацию") - регистрируется тем же
	// путём, что и сущность: backend'у нужен дескриптор САМОГО интерфейса
	// (тип_значения()/тип_параметра_метода(...) метода, встреченного через
	// него) и, отдельно, Program.InterfaceImplementors (собран в buildIR,
	// ниже по файлу analyzer.go) - список сущностей, которые он реально
	// принимает при разбоксировании.
	if to.Kind == ir.KindEntity || to.Kind == ir.KindInterface {
		sc.a.registerReflectTarget(to)
	}
	return &ir.AsExpr{Value: value, To: to}, nil
}
