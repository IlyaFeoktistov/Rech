package ir

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
	// KindInterface - значение интерфейсного типа - контракт на
	// набор методов, которому соответствует конкретная сущность за
	// значением (хранит её имя в Entity, тем же полем, что и KindEntity -
	// два вида различает только Kind, оба несут ровно "имя типа" и больше
	// ничего). У интерфейсных значений нет полей (только методы) - в
	// отличие от KindEntity, доступ через "." к полю - ошибка компиляции.
	KindInterface
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
	TypeUnknown = Type{Kind: KindUnknown}
	TypeByte    = Type{Kind: KindByte}
	TypeInt     = Type{Kind: KindInt}
	TypeFloat32 = Type{Kind: KindFloat32}
	TypeFloat   = Type{Kind: KindFloat}
	TypeChar    = Type{Kind: KindChar}
	TypeString  = Type{Kind: KindString}
	TypeBool    = Type{Kind: KindBool}
	TypeFile    = Type{Kind: KindFile}
	TypeSocket  = Type{Kind: KindSocket}
)

// EntityType возвращает Type, соответствующий значению сущности name.
func EntityType(name string) Type {
	return Type{Kind: KindEntity, Entity: name}
}

// InterfaceType возвращает Type, соответствующий значению интерфейсного
// типа name.
func InterfaceType(name string) Type {
	return Type{Kind: KindInterface, Entity: name}
}

func (t Type) String() string {
	if t.Kind == KindEntity || t.Kind == KindInterface {
		return t.Entity
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
	default:
		return "неизвестный тип"
	}
}

// Expr - общий маркерный интерфейс выражений IR (как ast.Expr).
