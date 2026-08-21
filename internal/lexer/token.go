package lexer

// TokenType перечисляет виды токенов, которые умеет распознавать лексер Rech v0.1.
type TokenType int

const (
	EOF TokenType = iota
	NEWLINE

	IDENT  // идентификатор, например: главная, пользователь
	STRING // строковый литерал в двойных кавычках

	LBRACE // {
	RBRACE // }

	KwFunction // функция
	KwPrint    // вывести
)

// keywords - таблица ключевых слов v0.1. Список сознательно узкий:
// остальные слова из финальной идеи (сущность, имеет, может, переменная,
// если, иначе, пока, для, вернуть, создать, вызвать) добавляются по мере
// реализации соответствующих этапов - см. docs/ROADMAP.md.
var keywords = map[string]TokenType{
	"функция": KwFunction,
	"вывести": KwPrint,
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
	case LBRACE:
		return "{"
	case RBRACE:
		return "}"
	case KwFunction:
		return "функция"
	case KwPrint:
		return "вывести"
	default:
		return "UNKNOWN"
	}
}
