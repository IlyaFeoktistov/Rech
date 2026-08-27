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
)

// entityType возвращает valueType, соответствующий значению сущности name.
func entityType(name string) valueType {
	return valueType{kind: kindEntity, entity: name}
}

// primitiveCTypes - valueType (не kindEntity/kindUnknown) -> C-тип.
// kindChar - int32_t ("Символ" хранит кодовую точку Unicode, не байт, как C
// char - язык кириллический по духу, 'я' должен помещаться в один Символ).
var primitiveCTypes = map[valueType]string{
	typeByte:    "unsigned char",
	typeInt:     "long long",
	typeFloat32: "float",
	typeFloat:   "double",
	typeChar:    "int32_t",
	typeString:  "const char *",
	typeBool:    "bool",
	typeFile:    "rech_file_t",
	typeSocket:  "rech_socket_t",
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
	if c, ok := primitiveCTypes[t]; ok {
		return c
	}
	return "void *"
}
