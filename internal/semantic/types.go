package semantic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
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
	"БД":             ir.TypeDB,
	"Дробное":        ir.TypeFloat32,
	"Символ":         ir.TypeChar,
	"Байт":           ir.TypeByte,
	"Целое8":         ir.TypeInt8,
	"Целое16":        ir.TypeInt16,
	"Целое32":        ir.TypeInt32,
	"Целое64":        ir.TypeInt64,
	"Натуральное":    ir.TypeUint,
	"Натуральное8":   ir.TypeUint8,
	"Натуральное16":  ir.TypeUint16,
	"Натуральное32":  ir.TypeUint32,
	"Натуральное64":  ir.TypeUint64,
	"Тип":            ir.TypeTypeInfo,
	"Значение":       ir.TypeAny,
}

// resolveTypeName - единственное место, где имя типа Rech (строка из AST)
// маппится на ir.Type: встроенные примитивы, уже собранная сущность
// (entities - сигнатуры, собранные раньше по файлу, см. collectEntities;
// сущность, объявленная позже в файле, чем место, где её имя встретилось
// как тип - та же граница, что и в cbackend.resolveType) или производный
// тип состояния (aliases - "АвторизованныйПользователь" -> "Пользователь",
// см. collectEntities) - резолвится в тип БАЗОВОЙ сущности, не в
// собственный тип: производный тип - псевдоним без своего
// представления, как и в cbackend (genCtx.stateAliases), - или встроенная
// generic-коллекция (resolveGenericTypeName/resolveDictTypeName).
func resolveTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, name string) (ir.Type, error) {
	if strings.HasPrefix(name, "?") {
		return resolveNullableTypeName(entities, methods, aliases, interfaces, name)
	}
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
	case "ДатаВремя":
		registerHTTPEntity(entities, "ДатаВремя", dateTimeFields)
		return ir.EntityType("ДатаВремя"), nil
	case mutexEntityName:
		registerMutexEntity(entities, methods)
		return ir.EntityType(mutexEntityName), nil
	}
	if _, ok := interfaces[name]; ok {
		return ir.InterfaceType(name), nil
	}
	if sig, ok := entities[name]; ok {
		if sig.IsExternOpaque {
			return ir.ExternEntityType(name), nil
		}
		if sig.IsPointer {
			// "Указатель<T>", уже зарегистрированный resolvePointerTypeName
			// при первой встрече - в отличие от Список/Стопка/Очередь/
			// Словарь/пользовательских generic-типов (у которых
			// инстанциация ДЕЙСТВИТЕЛЬНО KindEntity), у "Указатель<T>"
			// собственный Kind (KindPointer) - без этой проверки повторное
			// упоминание уже встреченного "Указатель<T>" (например,
			// "Указатель<Указатель<T>>" при резолвинге внешнего слоя)
			// молча вернулось бы как KindEntity.
			return ir.Type{Kind: ir.KindPointer, Entity: name}, nil
		}
		return ir.EntityType(name), nil
	}
	if base, ok := aliases[name]; ok {
		return ir.EntityType(base), nil
	}
	if strings.HasPrefix(name, ir.CallableTypeName+"<") {
		params, result, err := resolveCallableTypeName(entities, methods, aliases, interfaces, name)
		if err != nil {
			return ir.TypeUnknown, err
		}
		return ir.CallableType(params, result), nil
	}
	return resolveGenericTypeName(entities, methods, aliases, interfaces, name)
}

// httpRequestFields/httpResponseFields - фиксированные поля встроенных
// сущностей "Запрос"/"Ответ" - в этом порядке
// (позиционный конструктор "новый Запрос(...)" следует ему) и без
// падежного согласования (обычные имена полей). Заголовки - одна `Строка`
// (каждый заголовок на своей строке, разделитель "\r\n"), не `Словарь<К,
// В>` - заводить отдельный тип представления только для заголовков было бы
// преждевременно, раз formatting/парсинг уже завязаны на одну строку.
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
	sig := entitySig{IsStruct: true}
	for _, f := range fields {
		sig.FieldNames = append(sig.FieldNames, f.Name)
		sig.FieldTypes = append(sig.FieldTypes, f.Type)
		sig.FieldVisibility = append(sig.FieldVisibility, ast.Public)
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
// (Список/Стопка/Очередь - у "Словарь" своя функция, resolveDictTypeName,
// два типа-аргумента вместо одного) для конкретного типа элемента elem -
// курируемый список, как и
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

// isBuiltinCollectionType - t сам является инстанциацией встроенной
// generic-коллекции (Список/Стопка/Очередь/Словарь), а не пользовательской
// сущностью - у ЛЮБОЙ из четырёх ElemType всегда заполнен (entitySig.
// ElemType), у обычной пользовательской сущности - никогда (нулевое
// значение, ir.TypeUnknown) - этого достаточно, не нужен отдельный
// маркер-флаг вида "IsGenericCollection" (IsList/IsDict сами по себе не
// покрывают Стопка/Очередь - IsList true только у Список, ради проверки
// цикла "для x в списке").
func isBuiltinCollectionType(entities map[string]entitySig, t ir.Type) bool {
	if t.Kind != ir.KindEntity {
		return false
	}
	return entities[t.Entity].ElemType != ir.TypeUnknown
}

// collectionRealEntityElem - t сам является встроенной generic-коллекцией
// (isBuiltinCollectionType) с элементом (у "Словарь" - значением, тот же
// ElemType) - ПОЛЬЗОВАТЕЛЬСКОЙ сущностью/структурой (не примитивом и не
// вложенной другой встроенной коллекцией) - возвращает эту сущность и true.
// Используется ТОЛЬКО для запрета такой коллекции ПОЛЕМ сущности/структуры
// (см. её вызов в collectEntities) - самим Список<T>/Словарь<К, В>
// сущность элементом разрешена (resolveGenericTypeName), но C-typedef
// такой коллекции (cbackend) должен идти строго ПОСЛЕ typedef самой
// сущности-элемента (анонимные typedef struct - без forward-declaration в
// C), а entity/structure-typedef этой же сущности местами (её поле -
// generic-коллекция ПО ЗНАЧЕНИЮ) должен идти РАНЬШЕ typedef коллекции,
// которую несёт - при сущности-элементе одновременно оба требования
// решаются полным топологическим порядком typedef'ов, которого сейчас нет
// (только фиксированный порядок "коллекции -> сущности", плюс отдельные
// группы под "Вызываемый"/nullable-обёртки, cbackend/ir_generator.go) -
// вместо него простой запрет: сущность-элемент коллекции не может быть
// полем (только локальная переменная/параметр/результат функции, где такой
// проблемы порядка нет - typedef коллекции синтезируется в отдельный блок
// ПОСЛЕ всех typedef сущностей, см. genericsCodeAfterEntities).
func collectionRealEntityElem(entities map[string]entitySig, t ir.Type) (ir.Type, bool) {
	if !isBuiltinCollectionType(entities, t) {
		return ir.TypeUnknown, false
	}
	elem := entities[t.Entity].ElemType
	if elem.Kind != ir.KindEntity || isBuiltinCollectionType(entities, elem) {
		return ir.TypeUnknown, false
	}
	return elem, true
}

// topoSortGenericNames упорядочивает generic-инстанциации (names) так,
// чтобы инстанциация, использованная как ЭЛЕМЕНТ другой (Список<Целое>
// внутри Список<Список<Целое>>), шла РАНЬШЕ той, что её содержит -
// backend'у (internal/backend/cbackend) нужен уже готовый C-typedef
// внутреннего типа, чтобы объявить typedef внешнего (то же ограничение,
// что раньше решалось простым "просто до/после typedefs сущностей" для
// generic-коллекций как таковых - вложенность коллекций друг в друга
// требует настоящего топологического порядка, не одной пары "до/после").
// DFS-обход с сортировкой по имени как детерминированным тай-брейком между
// независимыми ветками (сохраняет прежний порядок там, где вложенности
// нет).
func topoSortGenericNames(names []string, entities map[string]entitySig) []string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	visited := make(map[string]bool, len(sorted))
	order := make([]string, 0, len(sorted))
	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		if elem := entities[name].ElemType; isBuiltinCollectionType(entities, elem) {
			visit(elem.Entity)
		}
		order = append(order, name)
	}
	for _, name := range sorted {
		visit(name)
	}
	return order
}

// resolveGenericTypeName - при первой встрече конкретной инстанциации
// builtin-коллекции ("Список<Целое>") синтезирует её сигнатуру (entitySig/
// funcSig) и регистрирует в entities/methods (мемоизация - синтез только
// один раз, дальше просто найдётся в карте) - дальше остальной
// internal/semantic обращается с ней как с обычной сущностью, не зная,
// пользовательская она или встроенная.
func resolveGenericTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, name string) (ir.Type, error) {
	template, argName, ok := parseGenericTypeName(name)
	if !ok {
		return ir.TypeUnknown, fmt.Errorf("неизвестный тип %q", name)
	}
	if template == "Словарь" {
		return resolveDictTypeName(entities, methods, aliases, interfaces, name)
	}
	if template == "Указатель" {
		return resolvePointerTypeName(entities, methods, aliases, interfaces, name, argName)
	}
	elem, err := resolveTypeName(entities, methods, aliases, interfaces, argName)
	if err != nil {
		return ir.TypeUnknown, fmt.Errorf("тип элемента %q в %q: %w", argName, name, err)
	}
	if elem.Kind == ir.KindNullable {
		return ir.TypeUnknown, fmt.Errorf("%q: элементы встроенных коллекций пока не поддерживают nullable-типы", name)
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

// dictKeyKinds - допустимые типы ключа "Словарь<К, В>" - те, у которых есть
// естественное, однозначное равенство и хеш (dictCollectionCCode,
// internal/backend/cbackend): целочисленные примитивы и Строка.
// Действительное/Дробное осознанно исключены - сравнение чисел с плавающей
// точкой на равенство ненадёжно (типичная ловушка в любом языке с
// хеш-картами), проще запретить, чем документировать предостережение.
var dictKeyKinds = map[ir.TypeKind]bool{
	ir.KindInt: true, ir.KindString: true, ir.KindByte: true, ir.KindChar: true, ir.KindBool: true,
}

// resolveDictTypeName - resolveGenericTypeName для "Словарь<К, В>" -
// отдельная функция, не ветка внутри resolveGenericTypeName, потому что
// у "Словарь" два аргумента типа (parseGenericTypeNameMulti, не
// parseGenericTypeName) и своя, более узкая проверка типа ключа
// (dictKeyKinds) вдобавок к общей "значения - только примитивы", как у
// Список/Стопка/Очередь.
func resolveDictTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, name string) (ir.Type, error) {
	_, args, ok := parseGenericTypeNameMulti(name)
	if !ok || len(args) != 2 {
		return ir.TypeUnknown, fmt.Errorf("%q: \"Словарь\" ожидает ровно два аргумента типа - ключ и значение (\"Словарь<Строка, Целое>\")", name)
	}
	key, err := resolveTypeName(entities, methods, aliases, interfaces, args[0])
	if err != nil {
		return ir.TypeUnknown, fmt.Errorf("тип ключа %q в %q: %w", args[0], name, err)
	}
	if !dictKeyKinds[key.Kind] {
		return ir.TypeUnknown, fmt.Errorf("%q: тип ключа %s не поддерживается - допустимы только Целое/Байт/Символ/Логическое/Строка", name, key)
	}
	val, err := resolveTypeName(entities, methods, aliases, interfaces, args[1])
	if err != nil {
		return ir.TypeUnknown, fmt.Errorf("тип значения %q в %q: %w", args[1], name, err)
	}
	if val.Kind == ir.KindNullable {
		return ir.TypeUnknown, fmt.Errorf("%q: значения встроенного словаря пока не поддерживают nullable-типы", name)
	}
	if _, exists := entities[name]; !exists {
		entities[name] = entitySig{IsDict: true, KeyType: key, ElemType: val}
		methods[name] = map[string]funcSig{
			"установить": {Params: []ir.Type{key, val}},
			"получить":   {Params: []ir.Type{key}, ReturnType: val, HasReturn: true},
			"содержит":   {Params: []ir.Type{key}, ReturnType: ir.TypeBool, HasReturn: true},
			"удалить":    {Params: []ir.Type{key}},
			"длина":      {ReturnType: ir.TypeInt, HasReturn: true},
			"пусто":      {ReturnType: ir.TypeBool, HasReturn: true},
		}
	}
	return ir.EntityType(name), nil
}

// isFFIScalar - t допустим в сигнатуре "внешняя функция" и в поле "внешняя
// структура" (FFI, докс/SPEC.md, раздел 12): примитив со стабильным,
// задокументированным C-представлением - Целое (long long), Действительное
// (double), Дробное (float), Строка (const char *), Логическое (bool),
// Байт (unsigned char), Символ (int32_t, тот же C-тип, что и обычный C
// "int"/"int32_t" - подходит и для 4-байтного целочисленного поля реальной
// C-структуры, не только для кодовых точек Unicode). Сущность (кроме уже
// экспортированной/внешней) и generic-коллекция не имеют стабильного
// C-представления, на которое мог бы полагаться сторонний C-код.
func isFFIScalar(t ir.Type) bool {
	switch t.Kind {
	case ir.KindInt, ir.KindFloat, ir.KindFloat32, ir.KindString, ir.KindBool, ir.KindByte, ir.KindChar:
		return true
	default:
		return false
	}
}

// isFFIType - t допустим в сигнатуре "внешняя функция"/поле "внешняя
// структура": isFFIScalar (примитивы), KindExternEntity - непрозрачный
// указатель, объявленный "внешняя сущность" (стабильное C-представление -
// void *, см. cbackend/valuetype.go, kindExternEntity), или сама
// "внешняя структура" (entitySig.IsExternStruct) - у неё уже проверены все
// поля этой же функцией на этапе собственного объявления, значит она сама
// по значению - тоже стабильный C-тип, годный для вложения в другую
// внешнюю структуру или для передачи в другую внешнюю функцию. Обычная
// (не внешняя) "структура" не допускается - её поля не гарантированно
// FFI-безопасны (могли бы содержать Список/сущность/интерфейс).
func isFFIType(entities map[string]entitySig, t ir.Type) bool {
	if isFFIScalar(t) || t.Kind == ir.KindExternEntity {
		return true
	}
	if t.Kind == ir.KindEntity {
		return entities[t.Entity].IsExternStruct
	}
	return false
}
