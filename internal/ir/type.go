package ir

type TypeKind int

const (
	KindUnknown TypeKind = iota
	KindString
	KindInt
	KindFloat
	KindBool
	KindEntity
	// KindFile - непрозрачный дескриптор открытого файла (шаг 8, файловый
	// ввод-вывод) - без операторов, как и KindString.
	KindFile
	// KindSocket - непрозрачный дескриптор сетевого сокета (шаг 8, сетевые
	// примитивы) - как и KindFile, без операторов, платформенно-разное
	// представление (POSIX fd/Windows SOCKET) решает каждый потребитель сам.
	KindSocket
)

// Type - тип значения выражения IR, уже разрешённый семантическим анализом
// (шаг 5.b) - в отличие от AST, где тип выводится заново на каждом
// обращении (cbackend.genExpr), IR хранит готовый результат прямо в узле
// (Identifier.Type, BinaryExpr.Type, UnaryExpr.Type и т. п.) - у литералов
// тип известен по самому Go-типу узла, см. TypeOf.
type Type struct {
	Kind   TypeKind
	Entity string // валиден только при Kind == KindEntity
}

var (
	TypeUnknown = Type{Kind: KindUnknown}
	TypeString  = Type{Kind: KindString}
	TypeInt     = Type{Kind: KindInt}
	TypeFloat   = Type{Kind: KindFloat}
	TypeBool    = Type{Kind: KindBool}
	TypeFile    = Type{Kind: KindFile}
	TypeSocket  = Type{Kind: KindSocket}
)

// EntityType возвращает Type, соответствующий значению сущности name.
func EntityType(name string) Type {
	return Type{Kind: KindEntity, Entity: name}
}

func (t Type) String() string {
	if t.Kind == KindEntity {
		return t.Entity
	}
	switch t.Kind {
	case KindString:
		return "Строка"
	case KindInt:
		return "Целое"
	case KindFloat:
		return "Число"
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
