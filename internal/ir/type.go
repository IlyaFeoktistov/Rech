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
	if t.Kind == KindEntity || t.Kind == KindInterface || t.Kind == KindCallable || t.Kind == KindExternEntity {
		return t.Entity
	}
	if t.Kind == KindTask {
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
	default:
		return "неизвестный тип"
	}
}

// Expr - общий маркерный интерфейс выражений IR (как ast.Expr).
