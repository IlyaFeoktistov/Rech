package semantic

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

var builtinKindTypes = map[builtins.Kind]ir.Type{
	builtins.KindInt:    ir.TypeInt,
	builtins.KindFloat:  ir.TypeFloat,
	builtins.KindString: ir.TypeString,
	builtins.KindBool:   ir.TypeBool,
	builtins.KindFile:   ir.TypeFile,
	builtins.KindSocket: ir.TypeSocket,
}

func builtinIRType(k builtins.Kind) ir.Type {
	return builtinKindTypes[k]
}

// registerBuiltinFuncs заполняет funcs встроенными функциями шага 8
// (internal/builtins) - тот же список и тот же принцип регистрации, что и
// cbackend.registerBuiltins (AST-путь) - до разбора пользовательских
// функций программы.
func registerBuiltinFuncs(funcs map[string]funcSig) {
	for _, f := range builtins.All {
		params := make([]ir.Type, len(f.Params))
		for i, k := range f.Params {
			params[i] = builtinIRType(k)
		}
		funcs[f.Name] = funcSig{Params: params, ReturnType: builtinIRType(f.ReturnKind), HasReturn: f.HasReturn}
	}
}

// primitiveTypes - имена встроенных примитивов Rech -> ir.Type.
var primitiveTypes = map[string]ir.Type{
	"Строка":         ir.TypeString,
	"Целое":          ir.TypeInt,
	"Действительное": ir.TypeFloat,
	"Логическое":     ir.TypeBool,
	"Файл":           ir.TypeFile,
	"Сокет":          ir.TypeSocket,
	"Дробное":        ir.TypeFloat32,
	"Символ":         ir.TypeChar,
	"Байт":           ir.TypeByte,
}

// resolveTypeName - единственное место, где имя типа Rech (строка из AST)
// маппится на ir.Type: встроенные примитивы, уже собранная сущность
// (entities - сигнатуры, собранные раньше по файлу, см. collectEntities;
// сущность, объявленная позже в файле, чем место, где её имя встретилось
// как тип - та же граница, что и в cbackend.resolveType) или производный
// тип состояния (aliases - "АвторизованныйПользователь" -> "Пользователь",
// шаг 4.1, см. collectEntities) - резолвится в тип БАЗОВОЙ сущности, не в
// собственный тип: производный тип - псевдоним без своего
// представления, как и в cbackend (genCtx.stateAliases), - или встроенная
// generic-коллекция (шаг 4.2, resolveGenericTypeName).
func resolveTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, name string) (ir.Type, error) {
	if t, ok := primitiveTypes[name]; ok {
		return t, nil
	}
	switch name {
	case "Запрос":
		registerHTTPEntity(entities, "Запрос", httpRequestFields)
		return ir.EntityType("Запрос"), nil
	case "Ответ":
		registerHTTPEntity(entities, "Ответ", httpResponseFields)
		return ir.EntityType("Ответ"), nil
	}
	if _, ok := entities[name]; ok {
		return ir.EntityType(name), nil
	}
	if base, ok := aliases[name]; ok {
		return ir.EntityType(base), nil
	}
	return resolveGenericTypeName(entities, methods, aliases, name)
}

// httpRequestFields/httpResponseFields - фиксированные поля встроенных
// сущностей "Запрос"/"Ответ" - в этом порядке
// (позиционный конструктор "новый Запрос(...)" следует ему) и без
// падежного согласования (обычные имена полей). Заголовки - одна `Строка`
// (каждый заголовок на своей строке, разделитель "\r\n"), не список пар
// ключ/значение - в языке пока нет словаря/хеш-карты (docs/ROADMAP.md), а
// заводить отдельный тип только для заголовков было бы преждевременно.
var httpRequestFields = []ir.Field{
	{Name: "метод", Type: ir.TypeString},
	{Name: "адрес", Type: ir.TypeString},
	{Name: "порт", Type: ir.TypeInt},
	{Name: "путь", Type: ir.TypeString},
	{Name: "заголовки", Type: ir.TypeString},
	{Name: "тело", Type: ir.TypeString},
}

var httpResponseFields = []ir.Field{
	{Name: "код", Type: ir.TypeInt},
	{Name: "заголовки", Type: ir.TypeString},
	{Name: "тело", Type: ir.TypeString},
}

// registerHTTPEntity - лениво синтезирует entitySig встроенной сущности
// (мемоизация, как resolveGenericTypeName у generic-коллекций) - вызывается
// при первой встрече имени "Запрос"/"Ответ" где угодно (параметр, поле,
// тип результата, "новый Запрос(...)"), не только при вызове самой
// "запрос" - пользователь может объявить свою функцию, принимающую/
// возвращающую эти сущности, без единого вызова встроенной "запрос".
func registerHTTPEntity(entities map[string]entitySig, name string, fields []ir.Field) {
	if _, exists := entities[name]; exists {
		return
	}
	sig := entitySig{}
	for _, f := range fields {
		sig.FieldNames = append(sig.FieldNames, f.Name)
		sig.FieldTypes = append(sig.FieldTypes, f.Type)
		sig.FieldPrivate = append(sig.FieldPrivate, false)
	}
	entities[name] = sig
}

// standardEnums - встроенные перечисления -
// доступны в любой программе без объявления, тем же принципом, что и
// Файл/Сокет/Запрос/Ответ (пользователь не объявляет их сам, но может
// объявить СВОЁ перечисление с любым другим именем обычным
// "перечисление" - collectEnums). Курируемый список, не механизм
// пользовательских generic-типов - обычные перечисления того же вида,
// что пользователь мог бы написать сам, просто уже готовые для типичных,
// заранее известных наборов значений.
var standardEnums = map[string]map[string]int64{
	// HTTPКод - для сравнения с "запрос(...).код" (шаг 9, HTTP-клиент) без
	// голых магических чисел в исходнике программы - только самые частые
	// коды, не полный список IANA (курируемый минимум, не исчерпывающий).
	"HTTPКод": {
		"ОК":                 200,
		"Создано":            201,
		"Принято":            202,
		"БезСодержимого":     204,
		"ПеремещёнПостоянно": 301,
		"НайденВременно":     302,
		"НеИзменялось":       304,
		"НеправильныйЗапрос": 400,
		"НеАвторизован":      401,
		"Запрещено":          403,
		"НеНайдено":          404,
		"МетодНеДопустим":    405,
		"Конфликт":           409,
		"ОшибкаСервера":      500,
		"НеРеализовано":      501,
		"СервисНедоступен":   503,
	},
	// ДеньНедели/Месяц - нет ни одной функции даты/времени в языке пока
	// (docs/ROADMAP.md), но сами имена дней/месяцев не изменятся, когда она
	// появится - разумно завести сейчас, раз запрошены "заранее известные"
	// перечисления. Понедельник == 1 - ISO 8601, не "воскресенье == 0",
	// как в некоторых языках.
	"ДеньНедели": {
		"Понедельник": 1, "Вторник": 2, "Среда": 3, "Четверг": 4,
		"Пятница": 5, "Суббота": 6, "Воскресенье": 7,
	},
	"Месяц": {
		"Январь": 1, "Февраль": 2, "Март": 3, "Апрель": 4,
		"Май": 5, "Июнь": 6, "Июль": 7, "Август": 8,
		"Сентябрь": 9, "Октябрь": 10, "Ноябрь": 11, "Декабрь": 12,
	},
}

// parseGenericTypeName разбирает закодированную парсером строку вида
// "Список<Целое>" на (шаблон, аргумент) - тот же формат, что и
// cbackend.parseGenericTypeName (AST-путь) - IR не должен знать про
// cbackend, поэтому не переиспользует ту функцию напрямую, но формат
// строки - общий инвариант всего компилятора (parser.parseTypeName).
func parseGenericTypeName(name string) (template, arg string, ok bool) {
	i := strings.IndexByte(name, '<')
	if i < 0 || !strings.HasSuffix(name, ">") {
		return "", "", false
	}
	return name[:i], name[i+1 : len(name)-1], true
}

// genericMethodSigs - сигнатуры методов встроенного generic-шаблона
// (шаг 4.2) для конкретного типа элемента elem - курируемый список, как и
// genericTemplates в cbackend (AST-путь), только здесь сигнатуры, а не
// C-код (C-код синтезирует cbackend.GenerateFromIR по Program.Generics,
// переиспользуя cbackend.listCollectionCCode/stackCollectionCCode/
// queueCollectionCCode - ту же реализацию, что и у AST-пути).
func genericMethodSigs(template string, elem ir.Type) (map[string]funcSig, bool) {
	switch template {
	case "Список":
		return map[string]funcSig{
			"добавить":   {Params: []ir.Type{elem}},
			"получить":   {Params: []ir.Type{ir.TypeInt}, ReturnType: elem, HasReturn: true},
			"установить": {Params: []ir.Type{ir.TypeInt, elem}},
			"длина":      {ReturnType: ir.TypeInt, HasReturn: true},
			"пусто":      {ReturnType: ir.TypeBool, HasReturn: true},
		}, true
	case "Стопка":
		return map[string]funcSig{
			"положить": {Params: []ir.Type{elem}},
			"снять":    {ReturnType: elem, HasReturn: true},
			"верх":     {ReturnType: elem, HasReturn: true},
			"длина":    {ReturnType: ir.TypeInt, HasReturn: true},
			"пусто":    {ReturnType: ir.TypeBool, HasReturn: true},
		}, true
	case "Очередь":
		return map[string]funcSig{
			"добавить": {Params: []ir.Type{elem}},
			"извлечь":  {ReturnType: elem, HasReturn: true},
			"первый":   {ReturnType: elem, HasReturn: true},
			"длина":    {ReturnType: ir.TypeInt, HasReturn: true},
			"пусто":    {ReturnType: ir.TypeBool, HasReturn: true},
		}, true
	default:
		return nil, false
	}
}

// resolveGenericTypeName - при первой встрече конкретной инстанциации
// builtin-коллекции ("Список<Целое>") синтезирует её сигнатуру (entitySig/
// funcSig) и регистрирует в entities/methods (мемоизация - синтез только
// один раз, дальше просто найдётся в карте) - дальше остальной
// internal/semantic обращается с ней как с обычной сущностью, не зная,
// пользовательская она или встроенная.
func resolveGenericTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, name string) (ir.Type, error) {
	template, argName, ok := parseGenericTypeName(name)
	if !ok {
		return ir.TypeUnknown, fmt.Errorf("неизвестный тип %q", name)
	}
	elem, err := resolveTypeName(entities, methods, aliases, argName)
	if err != nil {
		return ir.TypeUnknown, fmt.Errorf("тип элемента %q в %q: %w", argName, name, err)
	}
	if elem.Kind == ir.KindEntity {
		return ir.TypeUnknown, fmt.Errorf("%q: элементы встроенных коллекций поддерживаются только для примитивов, не для сущностей", name)
	}
	sigs, ok := genericMethodSigs(template, elem)
	if !ok {
		return ir.TypeUnknown, fmt.Errorf("неизвестный generic-тип %q", template)
	}
	if _, exists := entities[name]; !exists {
		entities[name] = entitySig{IsList: template == "Список", ElemType: elem}
		methods[name] = sigs
	}
	return ir.EntityType(name), nil
}

// Analyze производит IR из AST. Покрытие на данный момент - шаги 0-4
// (`вывести`, переменные/арифметика, если/пока/для, функции с параметрами
// и типом результата, вызов, сущности/поля/конструктор/методы/"этот"),
// шаг 4.1 (состояния, упрощённая версия) и шаг 4.2 (встроенные generic-
// коллекции) - см. internal/ir, package doc. Расширяется вместе с

// (шаг 7, FFI) - тот же принцип, что и cbackend.isFFIScalar (AST-путь):
// ни сущности, ни generic-коллекции не имеют стабильного, задокументированного
// C-представления, на которое мог бы полагаться сторонний C-код.
func isFFIScalar(t ir.Type) bool {
	switch t.Kind {
	case ir.KindInt, ir.KindFloat, ir.KindString, ir.KindBool:
		return true
	default:
		return false
	}
}

// endsWithReturnValue - true, если последняя инструкция тела (верхнего
// уровня, не внутри вложенных если/пока/для) - "вернуть <значение>". Как и
// в AST-пути (cbackend.endsWithReturnValue) - простая синтаксическая
// проверка, не flow-sensitive анализ всех путей выполнения (это шаг 5,
// когда появится сам flow-sensitive слой - см. docs/ROADMAP.md).
