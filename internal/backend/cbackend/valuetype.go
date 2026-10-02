package cbackend

// valueTypeKind - "форма" значения: один из встроенных примитивов или
// kindEntity (конкретная сущность, имя которой несёт само значение
// valueType.entity - иначе структуре пришлось бы жить снаружи, в отдельном
// реестре, доступном отовсюду, где сравниваются типы).
type valueTypeKind int

const (
	kindUnknown valueTypeKind = iota
	kindByte
	kindInt
	kindFloat32
	kindFloat
	kindChar
	kindString
	kindBool
	kindEntity
	// kindFile - непрозрачный дескриптор открытого файла - представление в C
	// разное по платформе (int/HANDLE), см. valueType.cType и преамбулу
	// typedef rech_file_t в fileHelpers.
	kindFile
	// kindSocket - непрозрачный дескриптор сетевого сокета - тот же принцип,
	// что и kindFile, см. typedef rech_socket_t в socketHelpers.
	kindSocket
	// kindDB - непрозрачный дескриптор соединения с БД - представление в C
	// одно и то же для SQLite/PostgreSQL/MySQL (указатель на собственную
	// структуру-обёртку, несущую и настоящий handle драйвера, и тег того,
	// какой это драйвер), см. typedef rech_db_t в internal/backend/cbackend/
	// db.go.
	kindDB
	// kindHTTP2Session - непрозрачный дескриптор активной HTTP/2-сессии
	// (см. ir.KindHTTP2Session) - представление в C - указатель на
	// собственную структуру-обёртку над nghttp2-сессией, см. typedef
	// rech_http2_session_t в internal/backend/cbackend/http2.go.
	kindHTTP2Session
	// kindTask - значение-дескриптор задачи - entity несёт сигнатуру
	// запущенного Вызываемого (см. package doc concurrency.go) - всегда
	// указатель на C-структуру задачи этой сигнатуры.
	kindTask
	// kindCallable - значение типа "Вызываемый" (&имя/лямбда) -
	// entity несёт каноническую строку сигнатуры (та же строка, что и
	// ir.Type.Entity/ir.CallableSig.Name, см. internal/backend/cbackend/
	// callable.go) - значение (не указатель): двухсловная структура
	// {указатель на функцию, указатель на окружение замыкания} копируется
	// как есть, тем же принципом, что и Go func-значения.
	kindCallable
	// kindExternEntity - непрозрачный внешний тип ("внешняя сущность", FFI) -
	// entity несёт Rech-facing псевдоним, но само значение представлено
	// одинаково для ЛЮБОГО такого типа: "void *" (см. cType) - реальное
	// устройство чужой C-структуры Rech не знает и не описывает, только
	// передаёт указатель дальше между внешними/обычными функциями.
	kindExternEntity
	// kindInt8/16/32/64, kindUint/8/16/32/64 - Целое8/16/32/64,
	// Натуральное/8/16/32/64 - C stdint.h-типы напрямую (int8_t/uint32_t/
	// ...), <stdint.h> уже безусловно включён в каждый сгенерированный
	// файл (см. generateSource).
	kindInt8
	kindInt16
	kindInt32
	kindUint
	kindUint16
	kindUint32
	// kindAny - "Значение", боксированное значение любого типа (рефлексия,
	// internal/backend/cbackend/reflect.go) - C-представление rech_any_t
	// (тег типа + union), по значению, не по указателю - см. cType.
	kindAny
	// kindTypeInfo - "Тип", дескриптор типа (имя, поля, методы) - C-
	// представление "const rech_type_meta_t *" - указатель на статический
	// дескриптор, свой на каждый примитив/встретившуюся сущность (см.
	// internal/backend/cbackend/reflect.go).
	kindTypeInfo
	// kindNullable - "?Тип" - entity несёт полное каноническое имя (с "?",
	// тот же формат, что ir.Type.Entity у KindNullable) - C-представление
	// свой typedef-struct (тег занятости + значение) на каждую встретившуюся
	// инстанциацию, см. internal/backend/cbackend/nullable.go.
	kindNullable
	// kindPointer - "Указатель<T>" - представление в C буквально "T *"
	// (см. cType) - entity несёт каноническое имя ("Указатель<Целое>"),
	// тем же принципом, что и kindNullable/kindEntity.
	kindPointer
	// kindArray - "T[N]" - C-массив внутри своей структуры (arrayTypedefCode):
	// значение, entity несёт каноническое имя ("Целое[3]").
	kindArray
	// kindDecimal - "Десятичное", struct rech_dec (decimal_runtime.go).
	kindDecimal
)

// valueType - тип значения выражения C-кодогенерации. Структура (не голое
// число), потому что типов больше, чем фиксированный список примитивов - у
// сущностей есть имя, и оно должно сохраняться в самом значении valueType,
// чтобы кодогенерация полей/конструктора могла сравнивать "тип этого
// значения" с "тип поля Х" без внешнего контекста. Сравнение (==, !=),
// использование как ключа map - всё продолжает работать, потому что
// структура с сравнимыми полями сравнима сама.
type valueType struct {
	kind   valueTypeKind
	entity string // валиден только при kind == kindEntity - оригинальное имя сущности
}

var (
	typeUnknown      = valueType{kind: kindUnknown}
	typeByte         = valueType{kind: kindByte}
	typeDecimal      = valueType{kind: kindDecimal}
	typeInt          = valueType{kind: kindInt}
	typeFloat32      = valueType{kind: kindFloat32}
	typeFloat        = valueType{kind: kindFloat}
	typeChar         = valueType{kind: kindChar}
	typeString       = valueType{kind: kindString}
	typeBool         = valueType{kind: kindBool}
	typeFile         = valueType{kind: kindFile}
	typeSocket       = valueType{kind: kindSocket}
	typeDB           = valueType{kind: kindDB}
	typeHTTP2Session = valueType{kind: kindHTTP2Session}

	typeInt8   = valueType{kind: kindInt8}
	typeInt16  = valueType{kind: kindInt16}
	typeInt32  = valueType{kind: kindInt32}
	typeUint   = valueType{kind: kindUint}
	typeUint16 = valueType{kind: kindUint16}
	typeUint32 = valueType{kind: kindUint32}

	typeAny      = valueType{kind: kindAny}
	typeTypeInfo = valueType{kind: kindTypeInfo}
)

// entityType возвращает valueType, соответствующий значению сущности name.
func entityType(name string) valueType {
	return valueType{kind: kindEntity, entity: name}
}

// taskType возвращает valueType задачи сигнатуры sig (см. package doc
// kindTask).
func taskType(sig string) valueType {
	return valueType{kind: kindTask, entity: sig}
}

// callableType возвращает valueType, соответствующий значению типа
// "Вызываемый" с канонической сигнатурой name.
func callableType(name string) valueType {
	return valueType{kind: kindCallable, entity: name}
}

// externEntityType возвращает valueType, соответствующий значению
// непрозрачного внешнего типа alias (см. package doc kindExternEntity).
func externEntityType(alias string) valueType {
	return valueType{kind: kindExternEntity, entity: alias}
}

// nullableType возвращает valueType, соответствующий значению "?Тип" -
// name несёт полное каноническое имя (с "?").
func nullableType(name string) valueType {
	return valueType{kind: kindNullable, entity: name}
}

// pointerType возвращает valueType, соответствующий значению
// "Указатель<T>" - name несёт полное каноническое имя ("Указатель<Целое>").
func pointerType(name string) valueType {
	return valueType{kind: kindPointer, entity: name}
}

// arrayType - valueType массива "T[N]" по каноническому имени.
func arrayType(name string) valueType {
	return valueType{kind: kindArray, entity: name}
}

// primitiveCTypes - valueType (не kindEntity/kindUnknown) -> C-тип.
// kindChar - int32_t ("Символ" хранит кодовую точку Unicode, не байт, как C
// char - язык кириллический по духу, 'я' должен помещаться в один Символ).
var primitiveCTypes = map[valueType]string{
	typeByte:         "unsigned char",
	typeInt:          "long long",
	typeFloat32:      "float",
	typeFloat:        "double",
	typeDecimal:      "rech_dec",
	typeChar:         "int32_t",
	typeString:       "const char *",
	typeBool:         "bool",
	typeFile:         "rech_file_t",
	typeSocket:       "rech_socket_t",
	typeDB:           "rech_db_t *",
	typeHTTP2Session: "rech_http2_session_t *",
	typeInt8:         "int8_t",
	typeInt16:        "int16_t",
	typeInt32:        "int32_t",
	typeUint:         "uint64_t",
	typeUint16:       "uint16_t",
	typeUint32:       "uint32_t",
	typeAny:          "rech_any_t",
	typeTypeInfo:     "const rech_type_meta_t *",
}

// cType - C-тип, соответствующий t. Для kindEntity зависит от того,
// "сущность"/"класс" ли name (ссылка - указатель, GC_MALLOC) или
// "структура" (значение - сама структура, как раньше единственно возможное
// представление любой сущности) - см. ctx.entityIsRef.
func (t valueType) cType(ctx *irGenCtx) string {
	if t.kind == kindDecimal {
		ctx.usesDecimal = true
	}
	if t.kind == kindEntity {
		if ctx.entityIsRef(t.entity) {
			return entityCName(ctx, t.entity) + " *"
		}
		return entityCName(ctx, t.entity)
	}
	if t.kind == kindTask {
		// entity == "" - абстрактная "Задача" (isAssignable ослаблен под
		// значение ЛЮБОЙ конкретной "Задача<...>", см. package doc
		// isAssignable, internal/semantic/inheritance.go): конкретные
		// "rech_TASK_<сигнатура>" - разные, несовместимые C-структуры, общего
		// C-типа-предка у них нет - opaque "void *" достаточно для
		// хранения/передачи, "дождаться" на абстрактной "Задача" запрещено
		// на уровне semantic (см. analyzeAwaitExpr), значит разыменование
		// этого указателя сюда никогда не доходит.
		if t.entity == "" {
			return "void *"
		}
		return mangleTaskType(t.entity) + " *"
	}
	if t.kind == kindCallable {
		return mangleCallableType(t.entity)
	}
	if t.kind == kindExternEntity {
		return "void *"
	}
	if t.kind == kindNullable {
		return mangleType(t.entity)
	}
	if t.kind == kindPointer {
		inner := fromIRType(ctx.pointerInner[t.entity])
		return inner.cType(ctx) + " *"
	}
	if t.kind == kindArray {
		return mangleType(t.entity)
	}
	if c, ok := primitiveCTypes[t]; ok {
		return c
	}
	return "void *"
}
