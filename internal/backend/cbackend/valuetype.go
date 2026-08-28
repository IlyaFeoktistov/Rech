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
	// kindTask - значение-дескриптор задачи ("запустить <имя>(...)") -
	// entity несёт имя ОБЁРНУТОЙ ФУНКЦИИ, не тип результата (см.
	// package doc internal/backend/cbackend/concurrency.go) - всегда
	// указатель на собственную C-структуру этой функции.
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
	kindInt64
	kindUint
	kindUint8
	kindUint16
	kindUint32
	kindUint64
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
	typeUnknown = valueType{kind: kindUnknown}
	typeByte    = valueType{kind: kindByte}
	typeInt     = valueType{kind: kindInt}
	typeFloat32 = valueType{kind: kindFloat32}
	typeFloat   = valueType{kind: kindFloat}
	typeChar    = valueType{kind: kindChar}
	typeString  = valueType{kind: kindString}
	typeBool    = valueType{kind: kindBool}
	typeFile    = valueType{kind: kindFile}
	typeSocket  = valueType{kind: kindSocket}
	typeDB      = valueType{kind: kindDB}

	typeInt8   = valueType{kind: kindInt8}
	typeInt16  = valueType{kind: kindInt16}
	typeInt32  = valueType{kind: kindInt32}
	typeInt64  = valueType{kind: kindInt64}
	typeUint   = valueType{kind: kindUint}
	typeUint8  = valueType{kind: kindUint8}
	typeUint16 = valueType{kind: kindUint16}
	typeUint32 = valueType{kind: kindUint32}
	typeUint64 = valueType{kind: kindUint64}

	typeAny      = valueType{kind: kindAny}
	typeTypeInfo = valueType{kind: kindTypeInfo}
)

// entityType возвращает valueType, соответствующий значению сущности name.
func entityType(name string) valueType {
	return valueType{kind: kindEntity, entity: name}
}

// taskType возвращает valueType, соответствующий значению-дескриптору
// задачи, обёртывающей функцию funcName.
func taskType(funcName string) valueType {
	return valueType{kind: kindTask, entity: funcName}
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

// primitiveCTypes - valueType (не kindEntity/kindUnknown) -> C-тип.
// kindChar - int32_t ("Символ" хранит кодовую точку Unicode, не байт, как C
// char - язык кириллический по духу, 'я' должен помещаться в один Символ).
var primitiveCTypes = map[valueType]string{
	typeByte:     "unsigned char",
	typeInt:      "long long",
	typeFloat32:  "float",
	typeFloat:    "double",
	typeChar:     "int32_t",
	typeString:   "const char *",
	typeBool:     "bool",
	typeFile:     "rech_file_t",
	typeSocket:   "rech_socket_t",
	typeDB:       "rech_db_t *",
	typeInt8:     "int8_t",
	typeInt16:    "int16_t",
	typeInt32:    "int32_t",
	typeInt64:    "int64_t",
	typeUint:     "uint64_t",
	typeUint8:    "uint8_t",
	typeUint16:   "uint16_t",
	typeUint32:   "uint32_t",
	typeUint64:   "uint64_t",
	typeAny:      "rech_any_t",
	typeTypeInfo: "const rech_type_meta_t *",
}

// cType - C-тип, соответствующий t. Для kindEntity зависит от того,
// "сущность"/"класс" ли name (ссылка - указатель, GC_MALLOC) или
// "структура" (значение - сама структура, как раньше единственно возможное
// представление любой сущности) - см. ctx.entityIsRef.
func (t valueType) cType(ctx *irGenCtx) string {
	if t.kind == kindEntity {
		if ctx.entityIsRef(t.entity) {
			return mangleType(t.entity) + " *"
		}
		return mangleType(t.entity)
	}
	if t.kind == kindTask {
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
	if c, ok := primitiveCTypes[t]; ok {
		return c
	}
	return "void *"
}
