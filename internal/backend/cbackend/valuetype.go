package cbackend

// valueTypeKind - "форма" значения: один из встроенных примитивов или
// kindEntity (конкретная сущность, имя которой несёт само значение
// valueType.entity - иначе структуре пришлось бы жить снаружи, в отдельном
// реестре, доступном отовсюду, где сравниваются типы).
type valueTypeKind int

const (
	kindUnknown valueTypeKind = iota
	kindString
	kindInt
	kindFloat
	kindBool
	kindEntity
	// kindFile - непрозрачный дескриптор открытого файла (шаг 8, файловый
	// ввод-вывод) - представление в C разное по платформе (int/HANDLE), см.
	// valueType.cType и преамбулу typedef rech_file_t в fileHelpers.
	kindFile
	// kindSocket - непрозрачный дескриптор сетевого сокета (шаг 8, сетевые
	// примитивы) - тот же принцип, что и kindFile, см. typedef
	// rech_socket_t в socketHelpers.
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
	typeString  = valueType{kind: kindString}
	typeInt     = valueType{kind: kindInt}
	typeFloat   = valueType{kind: kindFloat}
	typeBool    = valueType{kind: kindBool}
	typeFile    = valueType{kind: kindFile}
	typeSocket  = valueType{kind: kindSocket}
)

// entityType возвращает valueType, соответствующий значению сущности name.
func entityType(name string) valueType {
	return valueType{kind: kindEntity, entity: name}
}

// typeNames - единственное место, где ВСТРОЕННЫЕ типы Rech связаны с их
// написанием в исходнике - valueType.String() читает эту таблицу для
// текста ошибок.
var typeNames = map[valueType]string{
	typeString: "Строка",
	typeInt:    "Целое",
	typeFloat:  "Число",
	typeBool:   "Логическое",
	typeFile:   "Файл",
	typeSocket: "Сокет",
}

func (t valueType) String() string {
	if t.kind == kindEntity {
		return t.entity
	}
	if name, ok := typeNames[t]; ok {
		return name
	}
	return "неизвестный тип"
}

func (t valueType) cType() string {
	if t.kind == kindEntity {
		return mangleType(t.entity)
	}
	switch t {
	case typeString:
		return "const char *"
	case typeInt:
		return "long long"
	case typeFloat:
		return "double"
	case typeBool:
		return "bool"
	case typeFile:
		return "rech_file_t"
	case typeSocket:
		return "rech_socket_t"
	default:
		return "void *"
	}
}
