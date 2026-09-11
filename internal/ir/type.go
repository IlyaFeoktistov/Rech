package ir

import "strings"

type TypeKind int

const (
	KindUnknown TypeKind = iota
	// KindByte - "Байт", 8 бит без знака ([0, 255]), C unsigned char.
	KindByte
	// KindInt - "Целое", 64-битное знаковое, C long long.
	KindInt
	// KindFloat32 - "Дробное", число с плавающей точкой одинарной точности
	// (C float) - в отличие от KindFloat ("Действительное", двойная
	// точность). Отдельный вид, не просто "более узкий KindFloat" - смешанная
	// арифметика Дробное/Действительное явно приводит Дробное к
	// Действительное (расширение), как и Целое->Действительное.
	KindFloat32
	// KindFloat - "Действительное", двойная точность, C double.
	KindFloat
	// KindChar - "Символ", один кодовый пункт Unicode (не байт, как C char -
	// язык кириллический по духу, 'я' должен помещаться в один Символ). В C -
	// int32_t, не char.
	KindChar
	KindString
	KindBool
	KindEntity
	// KindFile - непрозрачный дескриптор открытого файла - без операторов,
	// как и KindString.
	KindFile
	// KindSocket - непрозрачный дескриптор сетевого сокета - как и KindFile,
	// без операторов, платформенно-разное представление (POSIX fd/Windows
	// SOCKET) решает каждый потребитель сам.
	KindSocket
	// KindDB - непрозрачный дескриптор соединения с БД (SQLite/PostgreSQL/
	// MySQL под одним и тем же именем "БД", различаются по схеме строки
	// подключения в рантайме, не по Kind) - как и KindFile/KindSocket, без
	// операторов, представление решает каждый потребитель сам.
	KindDB
	// KindHTTP2Session - непрозрачный дескриптор активной HTTP/2-сессии
	// (клиентской или серверной, поверх уже принятого/установленного
	// "Сокет") - как и KindFile/KindSocket/KindDB, без операторов,
	// представление (nghttp2-сессия в cbackend, golang.org/x/net/http2 в
	// interp) решает каждый потребитель сам. Один RPC/запрос внутри такой
	// сессии - НЕ отдельный Kind, а обычная сущность "HTTP2Поток" (поля
	// "сессия"/"ид" - см. internal/semantic/http2.go) - у HTTP/2 одна
	// сессия мультиплексирует много потоков, а не наоборот, поэтому именно
	// сессия непрозрачна, а поток - обычные данные (номер потока), с
	// которыми работают свободные функции.
	KindHTTP2Session
	// KindInterface - значение интерфейсного типа - контракт на
	// набор методов, которому соответствует конкретная сущность за
	// значением (хранит её имя в Entity, тем же полем, что и KindEntity -
	// два вида различает только Kind, оба несут ровно "имя типа" и больше
	// ничего). У интерфейсных значений нет полей (только методы) - в
	// отличие от KindEntity, доступ через "." к полю - ошибка компиляции.
	KindInterface
	// KindTask - значение-дескриптор задачи, запущенной "запустить
	// <имя>(...)" в отдельном потоке ОС (шаг 15) - Entity несёт имя
	// обёрнутой функции (тем же полем, что KindEntity/KindInterface несут
	// имя сущности/интерфейса), не тип результата: у каждой когда-либо
	// запущенной функции - собственная C-обёртка (аргументы разной формы у
	// разных функций делают представление "по типу результата" негодным,
	// см. package doc internal/backend/cbackend/concurrency.go).
	KindTask
	// KindCoroTask - значение-дескриптор задачи, запущенной ФОНОВО на
	// лёгком планировщике корутин ("поручить <асинхронная функция>(...)") -
	// тот же смысл, что и KindTask (Entity -
	// имя обёрнутой функции, дескриптор потом дожидается обычным
	// "дождаться"), отдельный Kind ТОЛЬКО потому, что "дождаться"
	// (generateIRAwaitExpr, internal/backend/cbackend/concurrency.go)
	// обязан различить, какую пару C-обёрток звать - поточную
	// (rech_await_<имя>, concurrency.go) или корутинную
	// (rech_coro_join_<имя>, coroutine_task.go) - различие живёт в самом
	// ТИПЕ значения (переживает присваивание переменной, в отличие от
	// проверки формы AST-узла), не только в Coroutine-флаге LaunchExpr,
	// который эту переменную когда-то породил. На уровне Rech-кода
	// НЕВИДИМО - оба вида дают одно и то же имя типа "Задача<...>"
	// (Type.String()), программист не видит разницы.
	KindCoroTask
	// KindCallable - значение "функции как значения" ("Вызываемый",
	// "функция как значение") - функция/метод, взятая по ссылке (&имя), или лямбда,
	// присвоенная переменной/переданная аргументом. Entity несёт
	// каноническую строку сигнатуры вида "Вызываемый<Целое, Строка>Целое"
	// (параметры через запятую внутри "<...>", тип результата сразу после
	// закрывающей ">" без разделителя, пусто - без результата) - тем же
	// принципом, что KindTask несёт имя функции: сама структура сигнатуры
	// (список типов параметров, тип результата) не помещается в Type без
	// потери сравнимости через "==" (slice - несравнимый тип в Go), поэтому
	// разобранная форма живёт отдельно, в Program.Callables, под тем же
	// ключом-строкой, см. CallableSig.
	KindCallable
	// KindExternEntity - непрозрачный внешний тип, объявленный "внешняя
	// сущность <ИмяC> [как <Псевдоним>]" (FFI, docs/SPEC.md, раздел 12) -
	// указатель на структуру чужой C-библиотеки (например, CURL* из
	// libcurl), чьё реальное устройство Rech не описывает и не может -
	// значение всегда получено извне (от внешней функции) и передаётся
	// дальше как есть, без полей/методов/"новый". Entity несёт Rech-facing
	// псевдоним (тем же полем, что и KindEntity/KindInterface/KindTask).
	KindExternEntity
	// KindInt8/16/32/64 - "Целое8/16/32/64", знаковые целые фиксированной
	// ширины (явные аналоги C int8_t/int16_t/int32_t/int64_t) - в отличие
	// от KindInt ("Целое", уже 64-битное), участвуют в арифметике (+ - * /)
	// СТРОГО между операндами одного и того же Kind, без неявной промоции
	// (даже KindInt64 к KindInt, хотя оба 64-битные).
	KindInt8
	KindInt16
	KindInt32
	KindInt64
	// KindUint/8/16/32/64 - "Натуральное"/"Натуральное8/16/32/64", целые без
	// знака ([0, 2^N-1], не "Целое с проверкой на отрицательность") - тот же
	// принцип строгой (без промоции) арифметики, что и у KindInt8..64.
	// Приведение между знаковым и беззнаковым - только через явный каст,
	// никогда неявно.
	KindUint
	KindUint8
	KindUint16
	KindUint32
	KindUint64
	// KindAny - "Значение", непрозрачный контейнер значения ЛЮБОГО типа
	// (боксирование для рефлексии) - НЕ KindEntity: элементы встроенных
	// generic-коллекций ограничены примитивами ("не для сущностей", см.
	// internal/semantic/types.go), а Список<Значение> обязан работать
	// (аргументы вызова метода/конструктора по рефлексии) - представление
	// в cbackend несёт собственный тег типа внутри себя (rech_any_t), а не
	// полагается на статический Type снаружи, как у остальных Kind.
	KindAny
	// KindTypeInfo - "Тип", непрозрачный дескриптор типа (имя, поля,
	// методы) - результат "тип <Имя>" и "значение.тип()". Как и KindAny,
	// не KindEntity - у Тип нет полей/методов пользовательской сущности,
	// это сам дескриптор, а не значение описываемого им типа.
	KindTypeInfo
	// KindNullable - "?Тип" (значение либо самого Тип, либо Ничто) -
	// Entity несёт полное каноническое имя, включая "?" ("?Целое",
	// "?Пользователь") - тем же принципом, что Entity у KindTask/
	// KindCallable несёт составную строку, не голое имя. Внутренний тип
	// (Тип из "?Тип") не хранится в самом Type (иначе пришлось бы менять
	// Type на структуру с указателем, теряя сравнимость через "==") -
	// достаётся отдельно, через entitySig.ElemType по этому же
	// каноническому имени (см. internal/semantic/nullable.go), тем же
	// приёмом, что и элемент Список<T>/значение Словарь<К, В>. Отдельный
	// Kind, не KindEntity - у nullable-обёртки нет ни полей/методов, ни
	// vtable/JSON/рефлексии обычной сущности, путать с ними не нужно.
	KindNullable
	// KindNull - тип синглтон-значения "Ничто" САМОГО ПО СЕБЕ, не как часть
	// "?Тип" (KindNullable) - нужен, чтобы боксированное "Значение" могло
	// сообщить о своём типе "Ничто" через уже существующий общий путь
	// (typeInfoValue/reflectMetaVarFor читают Type.String()) - конкретно,
	// SQL NULL в результате "запросить" боксируется значением этого Kind
	// (internal/backend/cbackend/db.go, internal/interp/db.go), других
	// источников пока нет.
	KindNull
	// KindPointer - "Указатель<T>" (адрес значения T - переменной/параметра,
	// поля сущности, элемента списка) - Entity несёт полное каноническое
	// имя ("Указатель<Целое>"), тем же принципом, что и у KindNullable;
	// внутренний тип - через PointerInstantiation по этому имени (см. её
	// package doc), не в самом Type. Отдельный Kind, не generic-коллекция
	// (Список/Стопка/Очередь/Словарь) - представление тривиально (один
	// адрес, не растущий буфер), не нуждается в их инфраструктуре.
	KindPointer
)

// Type - тип значения выражения IR, уже разрешённый семантическим анализом
// (шаг 5.b) - в отличие от AST, где тип выводится заново на каждом
// обращении (cbackend.genExpr), IR хранит готовый результат прямо в узле
// (Identifier.Type, BinaryExpr.Type, UnaryExpr.Type и т. п.) - у литералов
// тип известен по самому Go-типу узла, см. TypeOf.
type Type struct {
	Kind TypeKind
	// Entity - имя сущности (Kind == KindEntity) или интерфейса
	// (Kind == KindInterface) - для остальных Kind не валидно.
	Entity string
}

var (
	TypeUnknown      = Type{Kind: KindUnknown}
	TypeByte         = Type{Kind: KindByte}
	TypeInt          = Type{Kind: KindInt}
	TypeFloat32      = Type{Kind: KindFloat32}
	TypeFloat        = Type{Kind: KindFloat}
	TypeChar         = Type{Kind: KindChar}
	TypeString       = Type{Kind: KindString}
	TypeBool         = Type{Kind: KindBool}
	TypeFile         = Type{Kind: KindFile}
	TypeSocket       = Type{Kind: KindSocket}
	TypeDB           = Type{Kind: KindDB}
	TypeHTTP2Session = Type{Kind: KindHTTP2Session}

	TypeInt8  = Type{Kind: KindInt8}
	TypeInt16 = Type{Kind: KindInt16}
	TypeInt32 = Type{Kind: KindInt32}
	TypeInt64 = Type{Kind: KindInt64}

	TypeUint   = Type{Kind: KindUint}
	TypeUint8  = Type{Kind: KindUint8}
	TypeUint16 = Type{Kind: KindUint16}
	TypeUint32 = Type{Kind: KindUint32}
	TypeUint64 = Type{Kind: KindUint64}

	TypeAny       = Type{Kind: KindAny}
	TypeTypeInfo  = Type{Kind: KindTypeInfo}
	TypeNullValue = Type{Kind: KindNull}
)

// IntWidthSigned возвращает битовую ширину и знаковость числового Kind,
// участвующего в строгой (без неявной промоции) целочисленной арифметике -
// KindInt ("Целое") и девять новых Kind фиксированной ширины/знаковости
// (KindInt8..64, KindUint..64). ok=false для любого другого Kind, в т. ч.
// KindByte/KindChar (у них своя, отдельная семантика - "код/индекс, не
// число для вычислений", арифметика над ними не поддерживается вовсе, см.
// internal/semantic/cast.go). Единая точка истины про ширину/знаковость -
// используется семантикой (арифметика/касты), cbackend (C-тип) и interp
// (усечение при переполнении/сравнение без знака), чтобы не разойтись.
func (k TypeKind) IntWidthSigned() (bits int, signed bool, ok bool) {
	switch k {
	case KindInt:
		return 64, true, true
	case KindInt8:
		return 8, true, true
	case KindInt16:
		return 16, true, true
	case KindInt32:
		return 32, true, true
	case KindInt64:
		return 64, true, true
	case KindUint:
		return 64, false, true
	case KindUint8:
		return 8, false, true
	case KindUint16:
		return 16, false, true
	case KindUint32:
		return 32, false, true
	case KindUint64:
		return 64, false, true
	default:
		return 0, false, false
	}
}

// EntityType возвращает Type, соответствующий значению сущности name.
func EntityType(name string) Type {
	return Type{Kind: KindEntity, Entity: name}
}

// InterfaceType возвращает Type, соответствующий значению интерфейсного
// типа name.
func InterfaceType(name string) Type {
	return Type{Kind: KindInterface, Entity: name}
}

// ExternEntityType возвращает Type, соответствующий значению непрозрачного
// внешнего типа alias (см. package doc KindExternEntity).
func ExternEntityType(alias string) Type {
	return Type{Kind: KindExternEntity, Entity: alias}
}

// TaskType возвращает Type, соответствующий значению-дескриптору задачи,
// запущенной "запустить funcName(...)".
func TaskType(funcName string) Type {
	return Type{Kind: KindTask, Entity: funcName}
}

// CoroTaskType возвращает Type, соответствующий значению-дескриптору
// задачи, запущенной ФОНОВО на планировщике корутин - "поручить
// funcName(...)" (см. package doc KindCoroTask).
func CoroTaskType(funcName string) Type {
	return Type{Kind: KindCoroTask, Entity: funcName}
}

// NullableType возвращает Type, соответствующий "?<inner>" - Entity несёт
// полное каноническое имя ("?" + inner.String()), см. package doc
// KindNullable. inner.Kind == KindNullable (повторная обёртка "??Тип") -
// вызывающая сторона (internal/semantic) отвергает это раньше, сюда такое
// не должно доходить.
func NullableType(inner Type) Type {
	return Type{Kind: KindNullable, Entity: "?" + inner.String()}
}

// CallableTypeName - зарезервированное имя типа "функция как значение".
const CallableTypeName = "Вызываемый"

// CallableType возвращает Type "функции как значения" с указанной
// сигнатурой - Entity несёт каноническую строку (см. KindCallable),
// построенную рекурсивным String() каждого параметра/результата, поэтому
// два семантически одинаковых значения-типа (например, оба построены из
// "Целое") дают текстуально одинаковую строку и поэтому равны через "=="
// как и любой другой Type. result == nil - без типа результата.
func CallableType(params []Type, result *Type) Type {
	var b strings.Builder
	b.WriteString(CallableTypeName)
	b.WriteByte('<')
	for i, p := range params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.String())
	}
	b.WriteByte('>')
	if result != nil {
		b.WriteString(result.String())
	}
	return Type{Kind: KindCallable, Entity: b.String()}
}

func (t Type) String() string {
	if t.Kind == KindEntity || t.Kind == KindInterface || t.Kind == KindCallable || t.Kind == KindExternEntity || t.Kind == KindNullable || t.Kind == KindPointer {
		return t.Entity
	}
	if t.Kind == KindTask || t.Kind == KindCoroTask {
		return "Задача<" + t.Entity + ">"
	}
	switch t.Kind {
	case KindByte:
		return "Байт"
	case KindInt:
		return "Целое"
	case KindFloat32:
		return "Дробное"
	case KindFloat:
		return "Действительное"
	case KindChar:
		return "Символ"
	case KindString:
		return "Строка"
	case KindBool:
		return "Логическое"
	case KindFile:
		return "Файл"
	case KindSocket:
		return "Сокет"
	case KindDB:
		return "БД"
	case KindHTTP2Session:
		return "HTTP2Сессия"
	case KindNull:
		return "Ничто"
	case KindInt8:
		return "Целое8"
	case KindInt16:
		return "Целое16"
	case KindInt32:
		return "Целое32"
	case KindInt64:
		return "Целое64"
	case KindUint:
		return "Натуральное"
	case KindUint8:
		return "Натуральное8"
	case KindUint16:
		return "Натуральное16"
	case KindUint32:
		return "Натуральное32"
	case KindUint64:
		return "Натуральное64"
	case KindAny:
		return "Значение"
	case KindTypeInfo:
		return "Тип"
	default:
		return "неизвестный тип"
	}
}

// Expr - общий маркерный интерфейс выражений IR (как ast.Expr).
