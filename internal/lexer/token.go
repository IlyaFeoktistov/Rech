package lexer

// TokenType перечисляет виды токенов, которые умеет распознавать лексер Rech v0.1.
type TokenType int

const (
	EOF TokenType = iota
	NEWLINE

	IDENT  // идентификатор, например: главная, пользователь
	STRING // строковый литерал в двойных кавычках
	CHAR   // символьный литерал в одинарных кавычках: 'a', 'я'
	INT    // целочисленный литерал: 42
	FLOAT  // литерал с плавающей точкой: 3.14

	// DOC_COMMENT - "/** ... */" перед объявлением (Value - текст между
	// "/**" и "*/", как есть, без обрезки отступов построчно - парсер сам
	// решает, что с ним делать). Обычный "// ..." и обычный "/* ... */" не
	// становятся токеном вообще - лексер съедает их как пробел
	// (skipInsignificantWhitespaceAndComments), см. docs/SPEC.md,
	// "Комментарии".
	DOC_COMMENT

	LBRACE    // {
	RBRACE    // }
	LPAREN    // (
	RPAREN    // )
	SEMICOLON // ;
	COMMA     // ,
	COLON     // :
	DBLCOLON  // :: (доступ к статическому члену - Тип::имя)
	DOT       // .

	ASSIGN  // =
	PLUS    // +
	MINUS   // -
	STAR    // *
	SLASH   // /
	PERCENT // % (остаток от деления, только Целое)
	EQ      // ==
	NEQ     // !=
	BANG    // ! (префикс - "!x", синоним "x == Ничто", только для nullable - docs/SPEC.md)
	LT      // <
	GT      // >
	LE      // <=
	GE      // >=

	INCR // ++
	DECR // --

	PLUS_ASSIGN    // += (а = а + б)
	MINUS_ASSIGN   // -= (а = а - б)
	STAR_ASSIGN    // *= (а = а * б)
	SLASH_ASSIGN   // /= (а = а / б)
	PERCENT_ASSIGN // %= (а = а % б)

	BITAND // &  (побитовое И, только Целое)
	BITOR  // |  (побитовое ИЛИ, только Целое)
	BITXOR // ^  (побитовое исключающее ИЛИ, только Целое)
	BITNOT // ~  (побитовое НЕ, унарное, только Целое)
	SHL    // << (сдвиг влево, только Целое)
	SHR    // >> (сдвиг вправо, только Целое)

	// HASH - "#" (условная компиляция по платформе - "#если ОС.<Член>
	// { ... }", docs/SPEC.md) - решается на этапе компиляции (internal/
	// module, ResolvePlatformBlocks), не то же самое, что рантайм "если"/
	// "иначе" (оба - уже существующие KwIf/KwElse, отдельного ключевого
	// слова для условия НЕ заведено - парсер сам различает по HASH перед
	// KwIf/KwElse).
	HASH // #

	KwFunction    // функция
	KwPrint       // вывести
	KwShow        // показать (отладочный вывод значения вместе с типом)
	KwVariable    // переменная
	KwAnd         // и
	KwOr          // или
	KwNot         // не
	KwTrue        // истина
	KwFalse       // ложь
	KwIf          // если
	KwElse        // иначе
	KwWhile       // пока
	KwFor         // для
	KwFrom        // от
	KwTo          // до
	KwReturn      // вернуть
	KwWith        // с (предлог творительного падежа в естественном вызове)
	KwIn          // в (предлог предложного падежа в естественном вызове)
	KwEntity      // сущность
	KwClass       // класс (синоним "сущность")
	KwStruct      // структура (тип, передаваемый по значению - в отличие от сущности/класса)
	KwHas         // имеет
	KwNew         // новый / новая / новое / новые
	KwThis        // этот
	KwState       // состояние (секция объявления состояний сущности)
	KwBecomes     // становится (переход в другое состояние)
	KwAssert      // утверждать (проверка условия, аварийно завершает при лжи)
	KwPrivate     // приватный / приватная / приватное / приватные
	KwPublic      // публичный / публичная / публичное / публичные
	KwProtected   // защищённый / защищённая / защищённое / защищённые
	KwInternal    // внутренний / внутренняя / внутреннее (видимость верхнего уровня файла - виден всему .rechproj-проекту, но не потребителю через "использовать ... из" зависимости)
	KwStatic      // статический / статическая / статическое / статические
	KwUse         // использовать (импорт)
	KwOf          // из (предлог перед путём импорта)
	KwAs          // как (переименование импортируемого имени)
	KwExternal    // внешняя (внешняя функция, FFI)
	KwConst       // константа (неизменяемое значение)
	KwEnum        // перечисление (именованный набор целочисленных значений)
	KwSynonym     // синоним (другое имя уже существующего типа)
	KwInterface   // интерфейс (контракт на набор методов без реализации)
	KwInherits    // наследует (наследование сущности/класса)
	KwImplements  // реализует (соответствие интерфейсу)
	KwConstructor // конструктор (явное тело конструктора сущности/класса)
	KwParent      // родитель (вызов конструктора базовой сущности)
	KwTry         // попытаться
	KwCatch       // поймать
	KwFinally     // окончательно (блок "попытаться", исполняемый всегда последним)
	KwThrow       // выбросить
	KwRelates     // относится (проверка "относится к <Тип>")
	KwToward      // к (предлог дательного падежа после "относится")
	KwLaunch      // запустить (запуск функции в отдельном потоке ОС)
	KwAwait       // дождаться (ожидание завершения задачи)
	KwAsync       // асинхронный / асинхронная / асинхронное / асинхронные (модификатор "функция" - тело можно дождаться "дождаться <вызов>(...)" без явного "запустить")
	KwCoroLaunch  // поручить (фоновый запуск "асинхронная функция" на планировщике корутин, без немедленного ожидания - результат "Задача", как у "запустить", дожидается тем же "дождаться")
	KwExport      // экспортировать (функция/сущность/класс/структура, видимые извне библиотеки)
	KwParse       // разобрать (разобрать JSON <выражение> как <Тип>)
	KwSerialize   // сериализовать (сериализовать JSON <выражение>)
	KwJSON        // JSON (формат-маркер после "разобрать"/"сериализовать", не склоняется)
	KwXML         // XML (формат-маркер после "разобрать"/"сериализовать", не склоняется)
	KwYAML        // YAML (формат-маркер после "разобрать"/"сериализовать", не склоняется)
	KwPROTOBUF    // PROTOBUF (формат-маркер после "разобрать"/"сериализовать", не склоняется; источник/результат - Список<Байт>, не Строка)
	KwType        // тип (тип <Имя> - дескриптор типа без экземпляра, аналог typeof)
	KwNull        // Ничто (отсутствие значения у nullable-типа "?Тип")
	KwAbstract    // абстрактный / абстрактная / абстрактное / абстрактные (перед "сущность"/"класс")
	KwSwitch      // выбрать (множественный выбор по значению/типу - "выбрать <выражение> { когда ... иначе ... }")
	KwWhen        // когда (ветка внутри "выбрать")
	KwBreak       // прервать (немедленный выход из ближайшего охватывающего цикла)
	KwContinue    // продолжить (пропустить остаток текущей итерации ближайшего охватывающего цикла)
	KwOperator    // оператор (перегрузка бинарного оператора для сущности/структуры - "статический оператор +(...)")

	QUESTION         // ? (префикс nullable-типа: "?Тип")
	QUESTIONQUESTION // ?? (значение слева, если оно не Ничто, иначе - справа)
	NULL_ASSIGN      // ?= (присвоить справа, только если слева сейчас Ничто)

	LBRACKET // [
	RBRACKET // ]

	// TAG - произвольный текст в обратных кавычках "`...`" - ОДНА и та же
	// лексическая форма для двух разных ролей, различаемых парсером по
	// ПОЗИЦИИ, не лексером: тег поля/метода ("`json:...`" после объявления
	// поля/метода - как struct tag в Go, без кавычек вокруг значения, см.
	// internal/structtag) и шаблонная строка в позиции выражения
	// ("`Привет, ${имя}!`" - см. internal/parser/template.go). Content -
	// уже с обработанными escape-последовательностями (\n/\t/\r/\`/\\/\$) и
	// реальными переносами строк внутри (в отличие от обычной строки в
	// двойных кавычках, где перенос строки - ошибка).
	TAG
)

// keywords - таблица ключевых слов v0.1. Список сознательно узкий:
// остальные слова из финальной идеи (может)
// добавляются по мере реализации соответствующих этапов - см.
// .
var keywords = map[string]TokenType{
	"функция":        KwFunction,
	"вывести":        KwPrint,
	"показать":       KwShow,
	"переменная":     KwVariable,
	"и":              KwAnd,
	"или":            KwOr,
	"не":             KwNot,
	"истина":         KwTrue,
	"ложь":           KwFalse,
	"если":           KwIf,
	"иначе":          KwElse,
	"пока":           KwWhile,
	"для":            KwFor,
	"от":             KwFrom,
	"до":             KwTo,
	"вернуть":        KwReturn,
	"с":              KwWith,
	"в":              KwIn,
	"сущность":       KwEntity,
	"класс":          KwClass,
	"структура":      KwStruct,
	"имеет":          KwHas,
	"новый":          KwNew,
	"новая":          KwNew,
	"новое":          KwNew,
	"новые":          KwNew,
	"этот":           KwThis,
	"состояние":      KwState,
	"становится":     KwBecomes,
	"утверждать":     KwAssert,
	"приватный":      KwPrivate,
	"приватная":      KwPrivate,
	"приватное":      KwPrivate,
	"приватные":      KwPrivate,
	"публичный":      KwPublic,
	"публичная":      KwPublic,
	"публичное":      KwPublic,
	"публичные":      KwPublic,
	"защищённый":     KwProtected,
	"защищённая":     KwProtected,
	"защищённое":     KwProtected,
	"защищённые":     KwProtected,
	"внутренний":     KwInternal,
	"внутренняя":     KwInternal,
	"внутреннее":     KwInternal,
	"статический":    KwStatic,
	"статическая":    KwStatic,
	"статическое":    KwStatic,
	"статические":    KwStatic,
	"использовать":   KwUse,
	"из":             KwOf,
	"как":            KwAs,
	"внешняя":        KwExternal,
	"константа":      KwConst,
	"перечисление":   KwEnum,
	"синоним":        KwSynonym,
	"интерфейс":      KwInterface,
	"наследует":      KwInherits,
	"реализует":      KwImplements,
	"конструктор":    KwConstructor,
	"родитель":       KwParent,
	"попытаться":     KwTry,
	"поймать":        KwCatch,
	"окончательно":   KwFinally,
	"выбросить":      KwThrow,
	"относится":      KwRelates,
	"к":              KwToward,
	"запустить":      KwLaunch,
	"дождаться":      KwAwait,
	"асинхронный":    KwAsync,
	"асинхронная":    KwAsync,
	"асинхронное":    KwAsync,
	"асинхронные":    KwAsync,
	"поручить":       KwCoroLaunch,
	"экспортировать": KwExport,
	"разобрать":      KwParse,
	"сериализовать":  KwSerialize,
	"JSON":           KwJSON,
	"XML":            KwXML,
	"YAML":           KwYAML,
	"PROTOBUF":       KwPROTOBUF,
	"тип":            KwType,
	"Ничто":          KwNull,
	"абстрактный":    KwAbstract,
	"абстрактная":    KwAbstract,
	"абстрактное":    KwAbstract,
	"абстрактные":    KwAbstract,
	"выбрать":        KwSwitch,
	"когда":          KwWhen,
	"прервать":       KwBreak,
	"продолжить":     KwContinue,
	"оператор":       KwOperator,
}

// KeywordTokenTypes - различные TokenType из keywords, без повторов (у
// нескольких словоформ - "новый"/"новая"/"новое"/"новые",
// "приватный"/"приватная"/... - один и тот же TokenType) - экспортирован
// ради internal/lsp (проверка полноты keywordDocs, тем же принципом, что
// builtins.All/ReservedFuncNames экспортированы ради проверки полноты
// hoverBuiltinFunc).
func KeywordTokenTypes() []TokenType {
	seen := make(map[TokenType]bool, len(keywords))
	var out []TokenType
	for _, tt := range keywords {
		if !seen[tt] {
			seen[tt] = true
			out = append(out, tt)
		}
	}
	return out
}

// Token - одна лексема с позицией в исходном файле (для сообщений об ошибках).
type Token struct {
	Type  TokenType
	Value string
	Line  int
	Col   int

	// SpaceBefore - перед токеном в исходнике стоял пробел/комментарий
	// (не NEWLINE - он сам токен). Нужен там, где пробел различает два
	// прочтения: "Тип[]" (список) и "Тип []" (пустой список захвата
	// лямбды, parseTypeName).
	SpaceBefore bool
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
	case CHAR:
		return "CHAR"
	case INT:
		return "INT"
	case FLOAT:
		return "FLOAT"
	case DOC_COMMENT:
		return "DOC_COMMENT"
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
	case DBLCOLON:
		return "::"
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
	case PERCENT:
		return "%"
	case EQ:
		return "=="
	case NEQ:
		return "!="
	case BANG:
		return "!"
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
	case PLUS_ASSIGN:
		return "+="
	case MINUS_ASSIGN:
		return "-="
	case STAR_ASSIGN:
		return "*="
	case SLASH_ASSIGN:
		return "/="
	case PERCENT_ASSIGN:
		return "%="
	case QUESTION:
		return "?"
	case QUESTIONQUESTION:
		return "??"
	case NULL_ASSIGN:
		return "?="
	case LBRACKET:
		return "["
	case RBRACKET:
		return "]"
	case TAG:
		return "TAG"
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
	case HASH:
		return "#"
	case KwFunction:
		return "функция"
	case KwPrint:
		return "вывести"
	case KwShow:
		return "показать"
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
	case KwStruct:
		return "структура"
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
	case KwProtected:
		return "защищённый"
	case KwInternal:
		return "внутренний"
	case KwStatic:
		return "статический"
	case KwUse:
		return "использовать"
	case KwOf:
		return "из"
	case KwAs:
		return "как"
	case KwExternal:
		return "внешняя"
	case KwConst:
		return "константа"
	case KwEnum:
		return "перечисление"
	case KwSynonym:
		return "синоним"
	case KwInterface:
		return "интерфейс"
	case KwInherits:
		return "наследует"
	case KwImplements:
		return "реализует"
	case KwConstructor:
		return "конструктор"
	case KwParent:
		return "родитель"
	case KwTry:
		return "попытаться"
	case KwCatch:
		return "поймать"
	case KwFinally:
		return "окончательно"
	case KwThrow:
		return "выбросить"
	case KwRelates:
		return "относится"
	case KwToward:
		return "к"
	case KwLaunch:
		return "запустить"
	case KwAwait:
		return "дождаться"
	case KwAsync:
		return "асинхронная"
	case KwCoroLaunch:
		return "поручить"
	case KwExport:
		return "экспортировать"
	case KwParse:
		return "разобрать"
	case KwSerialize:
		return "сериализовать"
	case KwJSON:
		return "JSON"
	case KwXML:
		return "XML"
	case KwYAML:
		return "YAML"
	case KwPROTOBUF:
		return "PROTOBUF"
	case KwType:
		return "тип"
	case KwNull:
		return "Ничто"
	case KwAbstract:
		return "абстрактный"
	case KwSwitch:
		return "выбрать"
	case KwWhen:
		return "когда"
	case KwBreak:
		return "прервать"
	case KwContinue:
		return "продолжить"
	case KwOperator:
		return "оператор"
	default:
		return "UNKNOWN"
	}
}
