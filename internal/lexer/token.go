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
)

// keywords - таблица ключевых слов v0.1. Список сознательно узкий:
// остальные слова из финальной идеи (может, состояние)
// добавляются по мере реализации соответствующих этапов - см.
// docs/ROADMAP.md.
var keywords = map[string]TokenType{
	"функция":    KwFunction,
	"вывести":    KwPrint,
	"переменная": KwVariable,
	"и":          KwAnd,
	"или":        KwOr,
	"не":         KwNot,
	"истина":     KwTrue,
	"ложь":       KwFalse,
	"если":       KwIf,
	"иначе":      KwElse,
	"пока":       KwWhile,
	"для":        KwFor,
	"от":         KwFrom,
	"до":         KwTo,
	"вернуть":    KwReturn,
	"с":          KwWith,
	"в":          KwIn,
	"сущность":   KwEntity,
	"класс":      KwClass,
	"имеет":      KwHas,
	"новый":      KwNew,
	"новая":      KwNew,
	"новое":      KwNew,
	"новые":      KwNew,
	"этот":       KwThis,
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
	default:
		return "UNKNOWN"
	}
}
