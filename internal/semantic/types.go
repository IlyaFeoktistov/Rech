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
	"HTTP2Сессия":    ir.TypeHTTP2Session,
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
// generic-коллекция (resolveGenericTypeName/resolveDictTypeName) - или
// имя перечисления (enums, см. package doc ctx.enums/buildEnumTable):
// перечисление не заводит собственное IR-представление, тип аннотации
// резолвится прямо в ir.TypeInt (та же Целое-семантика, что и у
// "Тип.Член" в analyzeExpr).
func resolveTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, name string) (ir.Type, error) {
	if strings.HasPrefix(name, "?") {
		return resolveNullableTypeName(entities, methods, aliases, interfaces, enums, name)
	}
	if t, ok := primitiveTypes[name]; ok {
		return t, nil
	}
	if _, ok := enums[name]; ok {
		return ir.TypeInt, nil
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
	case semaphoreEntityName:
		registerSemaphoreEntity(entities, methods)
		return ir.EntityType(semaphoreEntityName), nil
	case transactionEntityName:
		registerTransactionEntity(entities, methods)
		return ir.EntityType(transactionEntityName), nil
	case httpBytesRequestType:
		if _, err := resolveTypeName(entities, methods, aliases, interfaces, enums, byteListType); err != nil {
			return ir.TypeUnknown, err
		}
		registerHTTPEntity(entities, httpBytesRequestType, httpBytesRequestFields)
		return ir.EntityType(httpBytesRequestType), nil
	case httpBytesResponseType:
		if _, err := resolveTypeName(entities, methods, aliases, interfaces, enums, byteListType); err != nil {
			return ir.TypeUnknown, err
		}
		registerHTTPEntity(entities, httpBytesResponseType, httpBytesResponseFields)
		return ir.EntityType(httpBytesResponseType), nil
	case formPartType:
		if _, err := resolveTypeName(entities, methods, aliases, interfaces, enums, byteListType); err != nil {
			return ir.TypeUnknown, err
		}
		registerHTTPEntity(entities, formPartType, formPartFields)
		return ir.EntityType(formPartType), nil
	case http2StreamType:
		registerHTTPEntity(entities, http2StreamType, HTTP2StreamFields)
		return ir.EntityType(http2StreamType), nil
	case imageCodecEntityName:
		if _, err := resolveTypeName(entities, methods, aliases, interfaces, enums, byteListType); err != nil {
			return ir.TypeUnknown, err
		}
		registerHTTPEntity(entities, imageCodecEntityName, ImageCodecFields)
		return ir.EntityType(imageCodecEntityName), nil
	case "Совпадение":
		// "группы" (Список<Строка>) - зарегистрировать ДО самой "Совпадение",
		// иначе метод вроде "м.группы.длина()" не найдёт methods["Список<
		// Строка>"], если ничто в программе больше не заставило её
		// зарегистрироваться (например, "разбить" ни разу не вызвана).
		if _, err := resolveTypeName(entities, methods, aliases, interfaces, enums, regexGroupsType); err != nil {
			return ir.TypeUnknown, err
		}
		registerHTTPEntity(entities, "Совпадение", MatchFields)
		return ir.EntityType("Совпадение"), nil
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
		params, result, err := resolveCallableTypeName(entities, methods, aliases, interfaces, enums, name)
		if err != nil {
			return ir.TypeUnknown, err
		}
		return ir.CallableType(params, result), nil
	}
	return resolveGenericTypeName(entities, methods, aliases, interfaces, enums, name)
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

// StandardEnums - встроенные перечисления - доступны в любой программе без
// объявления, тем же принципом, что и Файл/Сокет/Запрос/Ответ
// (пользователь не объявляет их сам, но может объявить СВОЁ перечисление с
// любым другим именем обычным "перечисление" - collectEnums). Курируемый
// список, не механизм пользовательских generic-типов - обычные
// перечисления того же вида, что пользователь мог бы написать сам, просто
// уже готовые для типичных, заранее известных наборов значений.
// Экспортирован ради internal/lsp (hover - показать член "HTTPКод.ОК" и
// т. п. без объявления в открытом документе, см. internal/lsp/enumhover.go) -
// единственный потребитель за пределами этого пакета.
var StandardEnums = map[string]map[string]int64{
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
	// СигналОС - для "получен_сигнал(СигналОС.SIGTERM)" (docs/ROADMAP.md) -
	// имена дословно POSIX, не переведены на русский - узнаваемость важнее
	// единообразия остального языка (искать "SIGTERM" в документации/
	// вопросах на форумах намного проще, чем переведённое имя). Значения -
	// НЕ настоящие номера сигналов ОС (те платформо-зависимы - SIGUSR1 не
	// одно и то же число на Linux/BSD/macOS) - это просто индекс в
	// внутреннюю таблицу cbackend/interp (см. package doc
	// internal/backend/cbackend/signals.go), настоящее имя C-макроса
	// подставляется генератором кода по этому индексу, не самим числом.
	// SIGKILL/SIGSTOP включены ради полноты списка имён - "получен_сигнал" с
	// ними паникует (rtmsg.SignalUncatchable) - POSIX не даёт перехватить ни
	// один из двух в принципе.
	//
	// Вторая группа (ordinal 15-30, добавлена вместе с "отправить_сигнал") -
	// остальные именованные POSIX-сигналы (signal(7)) до полного списка 1-31
	// (32+ - real-time сигналы SIGRTMIN..SIGRTMAX, у них нет фиксированных
	// имён - не заведены). Среди них SIGILL/SIGTRAP/SIGBUS/SIGFPE/SIGSEGV/
	// SIGSYS - сигналы АППАРАТНЫХ ошибок программы (обращение к чужой памяти,
	// деление на ноль и т. п.), не события "снаружи" - "получен_сигнал" их
	// технически регистрирует тем же sigaction, но возврат из обработчика
	// такого сигнала возобновляет выполнение ПРЯМО С ОШИБОЧНОЙ инструкции,
	// то есть исполнение обычно тут же падает в тот же сигнал повторно
	// (единственное надёжное применение - отловить сигнал, ПОСЛАННЫЙ другим
	// процессом через "отправить_сигнал"/kill, не настоящую аппаратную
	// ошибку). SIGSTKFLT/SIGPWR - специфичны для Linux (на других POSIX-
	// системах не определены вовсе) - компилятор уже нацелен только на
	// Linux/Windows (см. package doc cbackend/signals.go), не на macOS/BSD.
	"СигналОС": {
		"SIGHUP": 0, "SIGINT": 1, "SIGQUIT": 2, "SIGABRT": 3, "SIGKILL": 4,
		"SIGUSR1": 5, "SIGUSR2": 6, "SIGPIPE": 7, "SIGALRM": 8, "SIGTERM": 9,
		"SIGCHLD": 10, "SIGCONT": 11, "SIGSTOP": 12, "SIGTSTP": 13, "SIGWINCH": 14,
		"SIGILL": 15, "SIGTRAP": 16, "SIGBUS": 17, "SIGFPE": 18, "SIGSEGV": 19,
		"SIGSTKFLT": 20, "SIGTTIN": 21, "SIGTTOU": 22, "SIGURG": 23,
		"SIGXCPU": 24, "SIGXFSZ": 25, "SIGVTALRM": 26, "SIGPROF": 27,
		"SIGIO": 28, "SIGPWR": 29, "SIGSYS": 30,
	},
	// СетевойПротокол - для "сетевой_узел(СетевойПротокол.TCP, хост, порт)"
	// (docs/ROADMAP.md - найденный по ходу examples/проекты/чат_сервер
	// недочёт: голая строка "tcp"/"udp" - опечатка ("тcp"/"Tcp") молча
	// давала бы рантайм-панику вместо ошибки компиляции, ни один
	// инструмент (LSP/автодополнение) не мог подсказать допустимые
	// значения) - тот же принцип, что у АлгоритмХеша/АлгоритмПодписи
	// ниже: builtin принимает Целое (индекс), а не Строка, само сравнение
	// значения - внутри диспетчера (rech_network_node/networkNode), не
	// строковым strcmp.
	"СетевойПротокол": {
		"TCP": 0, "UDP": 1,
	},
	// АлгоритмХеша - для "хеш(АлгоритмХеша.SHA256, данные)" - один builtin
	// на семейство хеш-функций вместо отдельного имени на каждый алгоритм
	// (хеш_sha256/хеш_sha1/хеш_md5/... - как было в исходной версии этого
	// шага) - тот же принцип, что и у СигналОС/получен_сигнал выше:
	// открытый для расширения курируемый список, не нужно трогать
	// сигнатуру "хеш" при добавлении следующего алгоритма (SHA-3 и т. п.).
	// SHA1/MD5 - криптографически сломаны (известны коллизии), не для
	// новых паролей/подписей (для этого - SHA256/SHA512), только для
	// совместимости с форматами, которые их требуют "как есть".
	"АлгоритмХеша": {
		"SHA256": 0, "SHA1": 1, "MD5": 2, "SHA512": 3,
		// SHA3_256 - Keccak-based SHA-3 (FIPS 202), 256-битный вариант -
		// первый добавленный после самого механизма "хеш"/АлгоритмХеша
		// алгоритм, подтверждает, что расширение действительно дёшево
		// (только новый член здесь + новая ветка диспетчера в interp/
		// cbackend, без изменения сигнатуры "хеш"). Не сломан (в отличие
		// от SHA1/MD5) - годится для новых паролей/подписей, как SHA256/
		// SHA512 - добавлен не как замена им, а как самостоятельный,
		// принципиально другой (не Меркла-Дамгора, а sponge-конструкция)
		// алгоритм для тех, кому нужен именно SHA-3.
		"SHA3_256": 4,
	},
	// АлгоритмПодписи - для "сгенерировать_приватный_ключ(АлгоритмПодписи.
	// Ed25519)" - тот же принцип, что и АлгоритмХеша выше: builtin
	// расширяется новым членом, не новой сигнатурой. RSA - широко
	// поддерживается (X.509/TLS-сертификаты, старые SSH-ключи), Ed25519 -
	// современный стандарт (умолчание в OpenSSH с 2014) с более простым
	// API (фиксированный размер ключа/подписи, без выбора padding-схемы,
	// как у RSA).
	"АлгоритмПодписи": {
		"RSA": 0, "Ed25519": 1,
	},
	// ФорматИзображения - для "декодировать_изображение(путь,
	// ФорматИзображения.PNG)"/"закодировать_изображение(...)" - тот же
	// принцип, что и АлгоритмХеша/АлгоритмПодписи выше: один builtin на
	// семейство форматов вместо отдельного имени функции на каждый
	// (png_декодировать_файл/jpeg_декодировать_файл - как было в исходной
	// версии этого среза). internal/backend/cbackend/imagecodec.go -
	// rech_decode_image_file/rech_encode_image_file - ИНДЕКСЫ ДОЛЖНЫ
	// БУКВАЛЬНО СОВПАДАТЬ с их switch (0=PNG, 1=JPEG).
	"ФорматИзображения": {
		"PNG": 0, "JPEG": 1,
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
			// "удалить" - НАЙДЕННЫЙ по ходу examples/проекты/чат_сервер
			// недочёт (docs/ROADMAP.md): у Список не было способа удалить
			// элемент вовсе (только у Словарь) - сдвигает хвост на одну
			// позицию влево (та же семантика, что list.pop(i) в Python/
			// ArrayList.remove(int) в Java), возвращает удалённый элемент -
			// тем же принципом, что "снять"/"извлечь" у Стопка/Очередь.
			"удалить": {Params: []ir.Type{ir.TypeInt}, ReturnType: elem, HasReturn: true},
			"длина":   {ReturnType: ir.TypeInt, HasReturn: true},
			"пусто":   {ReturnType: ir.TypeBool, HasReturn: true},
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
func resolveGenericTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, name string) (ir.Type, error) {
	template, argName, ok := parseGenericTypeName(name)
	if !ok {
		return ir.TypeUnknown, fmt.Errorf("неизвестный тип %q", name)
	}
	if template == "Словарь" {
		return resolveDictTypeName(entities, methods, aliases, interfaces, enums, name)
	}
	if template == "Указатель" {
		return resolvePointerTypeName(entities, methods, aliases, interfaces, enums, name, argName)
	}
	elem, err := resolveTypeName(entities, methods, aliases, interfaces, enums, argName)
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
func resolveDictTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, name string) (ir.Type, error) {
	_, args, ok := parseGenericTypeNameMulti(name)
	if !ok || len(args) != 2 {
		return ir.TypeUnknown, fmt.Errorf("%q: \"Словарь\" ожидает ровно два аргумента типа - ключ и значение (\"Словарь<Строка, Целое>\")", name)
	}
	key, err := resolveTypeName(entities, methods, aliases, interfaces, enums, args[0])
	if err != nil {
		return ir.TypeUnknown, fmt.Errorf("тип ключа %q в %q: %w", args[0], name, err)
	}
	if !dictKeyKinds[key.Kind] {
		return ir.TypeUnknown, fmt.Errorf("%q: тип ключа %s не поддерживается - допустимы только Целое/Байт/Символ/Логическое/Строка", name, key)
	}
	val, err := resolveTypeName(entities, methods, aliases, interfaces, enums, args[1])
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
// void *, см. cbackend/valuetype.go, kindExternEntity), сама
// "внешняя структура" (entitySig.IsExternStruct) - у неё уже проверены все
// поля этой же функцией на этапе собственного объявления, значит она сама
// по значению - тоже стабильный C-тип, годный для вложения в другую
// внешнюю структуру или для передачи в другую внешнюю функцию, ИЛИ
// "Указатель<T>" (docs/ROADMAP.md, "FFI: указатели..." - изначально не
// пускалась вовсе, простая недоделка: появилась ПОЗЖЕ раздела FFI, границу
// просто не пересмотрели заодно) - открывает out-параметры C API вроде
// `sqlite3_open` без единой строчки нового рантайма, cbackend уже
// представляет её ровно как обычный C-указатель "T *" (internal/ir/
// pointer.go). Внутренний тип T рекурсивно обязан сам быть FFI-безопасным -
// "Указатель<СвояСущность>" (обычная, не внешняя сущность) по-прежнему
// недопустим: C-представление обычной сущности (GC-управляемая структура с
// vtable) не то, что ожидает реальный сторонний C API. Обычная (не внешняя)
// "структура" - тоже не допускается по значению - её поля не гарантированно
// FFI-безопасны (могли бы содержать Список/сущность/интерфейс).
//
// "Вызываемый(...)" (docs/ROADMAP.md, "FFI: маршалинг колбэков") - тоже
// допустима, если РЕКУРСИВНО FFI-безопасны и её параметры, и её результат -
// значение такого типа, переданное аргументом внешней функции/полем внешней
// структуры, не остаётся парой {fn, env}: cbackend маршалит его через
// общий на сигнатуру глобальный слот + сгенерированный C-trampoline с ЕЁ
// РЕАЛЬНОЙ (без скрытого env) сигнатурой (см. internal/backend/cbackend/
// ffi_callback.go) - C-сторона в результате получает голый указатель на
// функцию, ожидаемого вида. methods/aliases/interfaces нужны только для
// этого случая - resolveCallableTypeName резолвит собственные параметры/
// результат сигнатуры t.Entity.
func isFFIType(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, t ir.Type) bool {
	if isFFIScalar(t) || t.Kind == ir.KindExternEntity {
		return true
	}
	if t.Kind == ir.KindEntity {
		return entities[t.Entity].IsExternStruct
	}
	if t.Kind == ir.KindPointer {
		return isFFIType(entities, methods, aliases, interfaces, enums, entities[t.Entity].ElemType)
	}
	if t.Kind == ir.KindCallable {
		params, result, err := resolveCallableTypeName(entities, methods, aliases, interfaces, enums, t.Entity)
		if err != nil {
			return false
		}
		for _, p := range params {
			if !isFFIType(entities, methods, aliases, interfaces, enums, p) {
				return false
			}
		}
		return result == nil || isFFIType(entities, methods, aliases, interfaces, enums, *result)
	}
	return false
}
