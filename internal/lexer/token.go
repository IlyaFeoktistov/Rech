package lexer

// TokenType перечисляет виды токенов, которые умеет распознавать лексер Rech v0.1.
type TokenType int

const (
	EOF TokenType = iota
	NEWLINE

	IDENT  // идентификатор, например: главная, пользователь
	STRING // строковый литерал в двойных кавычках
	INT    // целочисленный литерал: 42
	FLOAT  // литерал с плавающей точкой: 3.14

	LBRACE    // {
	RBRACE    // }
	LPAREN    // (
	RPAREN    // )
	SEMICOLON // ;
	COMMA     // ,
	COLON     // :
	DOT       // .

	ASSIGN // =
	PLUS   // +
	MINUS  // -
	STAR   // *
	SLASH  // /
	EQ     // ==
	NEQ    // !=
	LT     // <
	GT     // >
	LE     // <=
	GE     // >=

	INCR // ++
	DECR // --

	BITAND // &  (побитовое И, только Целое)
	BITOR  // |  (побитовое ИЛИ, только Целое)
	BITXOR // ^  (побитовое исключающее ИЛИ, только Целое)
	BITNOT // ~  (побитовое НЕ, унарное, только Целое)
	SHL    // << (сдвиг влево, только Целое)
	SHR    // >> (сдвиг вправо, только Целое)

	KwFunction // функция
	KwPrint    // вывести
	KwVariable // переменная
	KwAnd      // и
	KwOr       // или
	KwNot      // не
	KwTrue     // истина
	KwFalse    // ложь
	KwIf       // если
	KwElse     // иначе
	KwWhile    // пока
	KwFor      // для
	KwFrom     // от
	KwTo       // до
	KwReturn   // вернуть
	KwWith     // с (предлог творительного падежа в естественном вызове)
	KwIn       // в (предлог предложного падежа в естественном вызове)
	KwEntity   // сущность
	KwClass    // класс (синоним "сущность")
	KwHas      // имеет
	KwNew      // новый / новая / новое / новые
	KwThis     // этот
	KwState    // состояние (секция объявления состояний сущности, шаг 4.1)
	KwBecomes  // становится (переход в другое состояние, шаг 4.1)
	KwAssert   // утверждать (проверка условия, аварийно завершает при лжи)
	KwPrivate  // приватный / приватная / приватное / приватные (шаг 4.3)
	KwPublic   // публичный / публичная / публичное / публичные (шаг 4.3)
	KwUse      // использовать (импорт, шаг 11)
	KwOf       // из (предлог перед путём импорта, шаг 11)
	KwAs       // как (переименование импортируемого имени, шаг 11)
)

// keywords - таблица ключевых слов v0.1. Список сознательно узкий:
// остальные слова из финальной идеи (может)
// добавляются по мере реализации соответствующих этапов - см.
// docs/ROADMAP.md.
var keywords = map[string]TokenType{
	"функция":      KwFunction,
	"вывести":      KwPrint,
	"переменная":   KwVariable,
	"и":            KwAnd,
	"или":          KwOr,
	"не":           KwNot,
	"истина":       KwTrue,
	"ложь":         KwFalse,
	"если":         KwIf,
	"иначе":        KwElse,
	"пока":         KwWhile,
	"для":          KwFor,
	"от":           KwFrom,
	"до":           KwTo,
	"вернуть":      KwReturn,
	"с":            KwWith,
	"в":            KwIn,
	"сущность":     KwEntity,
	"класс":        KwClass,
	"имеет":        KwHas,
	"новый":        KwNew,
	"новая":        KwNew,
	"новое":        KwNew,
	"новые":        KwNew,
	"этот":         KwThis,
	"состояние":    KwState,
	"становится":   KwBecomes,
	"утверждать":   KwAssert,
	"приватный":    KwPrivate,
	"приватная":    KwPrivate,
	"приватное":    KwPrivate,
	"приватные":    KwPrivate,
	"публичный":    KwPublic,
	"публичная":    KwPublic,
	"публичное":    KwPublic,
	"публичные":    KwPublic,
	"использовать": KwUse,
	"из":           KwOf,
	"как":          KwAs,
}

// Token - одна лексема с позицией в исходном файле (для сообщений об ошибках).
type Token struct {
	Type  TokenType
	Value string
	Line  int
	Col   int
}

func (t TokenType) String() string {
	switch t {
	case EOF:
		return "EOF"
	case NEWLINE:
		return "NEWLINE"
	case IDENT:
		return "IDENT"
	case STRING:
		return "STRING"
	case INT:
		return "INT"
	case FLOAT:
		return "FLOAT"
	case LBRACE:
		return "{"
	case RBRACE:
		return "}"
	case LPAREN:
		return "("
	case RPAREN:
		return ")"
	case SEMICOLON:
		return ";"
	case COMMA:
		return ","
	case COLON:
		return ":"
	case DOT:
		return "."
	case ASSIGN:
		return "="
	case PLUS:
		return "+"
	case MINUS:
		return "-"
	case STAR:
		return "*"
	case SLASH:
		return "/"
	case EQ:
		return "=="
	case NEQ:
		return "!="
	case LT:
		return "<"
	case GT:
		return ">"
	case LE:
		return "<="
	case GE:
		return ">="
	case INCR:
		return "++"
	case DECR:
		return "--"
	case BITAND:
		return "&"
	case BITOR:
		return "|"
	case BITXOR:
		return "^"
	case BITNOT:
		return "~"
	case SHL:
		return "<<"
	case SHR:
		return ">>"
	case KwFunction:
		return "функция"
	case KwPrint:
		return "вывести"
	case KwVariable:
		return "переменная"
	case KwAnd:
		return "и"
	case KwOr:
		return "или"
	case KwNot:
		return "не"
	case KwTrue:
		return "истина"
	case KwFalse:
		return "ложь"
	case KwIf:
		return "если"
	case KwElse:
		return "иначе"
	case KwWhile:
		return "пока"
	case KwFor:
		return "для"
	case KwFrom:
		return "от"
	case KwTo:
		return "до"
	case KwReturn:
		return "вернуть"
	case KwWith:
		return "с"
	case KwIn:
		return "в"
	case KwEntity:
		return "сущность"
	case KwClass:
		return "класс"
	case KwHas:
		return "имеет"
	case KwNew:
		return "новый"
	case KwThis:
		return "этот"
	case KwState:
		return "состояние"
	case KwBecomes:
		return "становится"
	case KwAssert:
		return "утверждать"
	case KwPrivate:
		return "приватный"
	case KwPublic:
		return "публичный"
	case KwUse:
		return "использовать"
	case KwOf:
		return "из"
	case KwAs:
		return "как"
	default:
		return "UNKNOWN"
	}
}
