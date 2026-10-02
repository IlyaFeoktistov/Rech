package lexer

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
)

// Lexer превращает исходный текст на Rech в поток токенов.
// Идентификаторы и ключевые слова опираются на unicode.IsLetter,
// поэтому кириллица обрабатывается так же естественно, как латиница.
type Lexer struct {
	src  []rune
	pos  int
	line int
	col  int

	// skipped - next() пропустил пробелы/комментарии перед последним
	// токеном (Token.SpaceBefore).
	skipped bool
}

// New строит Lexer над src. Ведущий UTF-8 BOM (U+FEFF - типичный результат
// сохранения файла как "UTF-8 with BOM" на Windows, в т.ч. по умолчанию у
// PowerShell Out-File/некоторых редакторов) обрезается - иначе он попадал
// бы в поток токенов как обычный символ и ронял разбор на первой строке.
func New(src string) *Lexer {
	src = strings.TrimPrefix(src, "\uFEFF")
	return &Lexer{src: []rune(src), pos: 0, line: 1, col: 1}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekAt(offset int) rune {
	if l.pos+offset >= len(l.src) {
		return 0
	}
	return l.src[l.pos+offset]
}

func (l *Lexer) advance() rune {
	r := l.src[l.pos]
	l.pos++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

// Tokenize прогоняет весь исходник и возвращает список токенов,
// завершающийся токеном EOF.
func (l *Lexer) Tokenize() ([]Token, error) {
	var tokens []Token
	for {
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		tok.SpaceBefore = l.skipped
		tokens = append(tokens, tok)
		if tok.Type == EOF {
			break
		}
	}
	return joinLeadingDotComma(tokens), nil
}

func (l *Lexer) next() (Token, error) {
	start := l.pos
	if err := l.skipInsignificantWhitespaceAndComments(); err != nil {
		return Token{}, err
	}
	l.skipped = l.pos > start

	line, col := l.line, l.col

	if l.pos >= len(l.src) {
		return Token{Type: EOF, Line: line, Col: col}, nil
	}

	r := l.peek()

	switch {
	case r == '\n':
		l.advance()
		return Token{Type: NEWLINE, Value: "\n", Line: line, Col: col}, nil
	case r == '{':
		l.advance()
		return Token{Type: LBRACE, Value: "{", Line: line, Col: col}, nil
	case r == '}':
		l.advance()
		return Token{Type: RBRACE, Value: "}", Line: line, Col: col}, nil
	case r == '(':
		l.advance()
		return Token{Type: LPAREN, Value: "(", Line: line, Col: col}, nil
	case r == ')':
		l.advance()
		return Token{Type: RPAREN, Value: ")", Line: line, Col: col}, nil
	case r == ';':
		l.advance()
		return Token{Type: SEMICOLON, Value: ";", Line: line, Col: col}, nil
	case r == ',':
		l.advance()
		return Token{Type: COMMA, Value: ",", Line: line, Col: col}, nil
	case r == ':':
		l.advance()
		if l.peek() == ':' {
			l.advance()
			return Token{Type: DBLCOLON, Value: "::", Line: line, Col: col}, nil
		}
		return Token{Type: COLON, Value: ":", Line: line, Col: col}, nil
	case r == '.':
		l.advance()
		if l.peek() == '.' && l.peekAt(1) == '.' {
			l.advance()
			l.advance()
			return Token{Type: ELLIPSIS, Value: "...", Line: line, Col: col}, nil
		}
		return Token{Type: DOT, Value: ".", Line: line, Col: col}, nil
	case r == '+':
		l.advance()
		if l.peek() == '+' {
			l.advance()
			return Token{Type: INCR, Value: "++", Line: line, Col: col}, nil
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Type: PLUS_ASSIGN, Value: "+=", Line: line, Col: col}, nil
		}
		return Token{Type: PLUS, Value: "+", Line: line, Col: col}, nil
	case r == '-':
		l.advance()
		if l.peek() == '-' {
			l.advance()
			return Token{Type: DECR, Value: "--", Line: line, Col: col}, nil
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Type: MINUS_ASSIGN, Value: "-=", Line: line, Col: col}, nil
		}
		return Token{Type: MINUS, Value: "-", Line: line, Col: col}, nil
	case r == '*':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: STAR_ASSIGN, Value: "*=", Line: line, Col: col}, nil
		}
		return Token{Type: STAR, Value: "*", Line: line, Col: col}, nil
	case r == '%':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: PERCENT_ASSIGN, Value: "%=", Line: line, Col: col}, nil
		}
		return Token{Type: PERCENT, Value: "%", Line: line, Col: col}, nil
	case r == '/' && l.peekAt(1) == '*':
		return l.readDocComment(line, col)
	case r == '/':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: SLASH_ASSIGN, Value: "/=", Line: line, Col: col}, nil
		}
		return Token{Type: SLASH, Value: "/", Line: line, Col: col}, nil
	case r == '=':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: EQ, Value: "==", Line: line, Col: col}, nil
		}
		return Token{Type: ASSIGN, Value: "=", Line: line, Col: col}, nil
	case r == '!':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: NEQ, Value: "!=", Line: line, Col: col}, nil
		}
		// Одиночный "!" - ПРЕФИКС: у nullable - синоним "x == Ничто", у
		// Логическое - отрицание, как "не" (docs/SPEC.md) - парсер
		// (parseUnary) десахаривает "!x" в BinaryExpr("==", x, Ничто) с
		// Bang, различает по типу операнда семантика (analyzeBang).
		return Token{Type: BANG, Value: "!", Line: line, Col: col}, nil
	case r == '<':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: LE, Value: "<=", Line: line, Col: col}, nil
		}
		if l.peek() == '<' {
			l.advance()
			return Token{Type: SHL, Value: "<<", Line: line, Col: col}, nil
		}
		return Token{Type: LT, Value: "<", Line: line, Col: col}, nil
	case r == '>':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: GE, Value: ">=", Line: line, Col: col}, nil
		}
		if l.peek() == '>' {
			l.advance()
			return Token{Type: SHR, Value: ">>", Line: line, Col: col}, nil
		}
		return Token{Type: GT, Value: ">", Line: line, Col: col}, nil
	case r == '&':
		l.advance()
		if l.peek() == '&' {
			l.advance()
			return Token{Type: KwAnd, Value: "&&", Line: line, Col: col}, nil
		}
		return Token{Type: BITAND, Value: "&", Line: line, Col: col}, nil
	case r == '|':
		l.advance()
		if l.peek() == '|' {
			l.advance()
			return Token{Type: KwOr, Value: "||", Line: line, Col: col}, nil
		}
		return Token{Type: BITOR, Value: "|", Line: line, Col: col}, nil
	case r == '^':
		l.advance()
		return Token{Type: BITXOR, Value: "^", Line: line, Col: col}, nil
	case r == '~':
		l.advance()
		return Token{Type: BITNOT, Value: "~", Line: line, Col: col}, nil
	case r == '#':
		l.advance()
		return Token{Type: HASH, Value: "#", Line: line, Col: col}, nil
	case r == '?':
		l.advance()
		if l.peek() == '?' {
			l.advance()
			return Token{Type: QUESTIONQUESTION, Value: "??", Line: line, Col: col}, nil
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Type: NULL_ASSIGN, Value: "?=", Line: line, Col: col}, nil
		}
		return Token{Type: QUESTION, Value: "?", Line: line, Col: col}, nil
	case r == '[':
		l.advance()
		return Token{Type: LBRACKET, Value: "[", Line: line, Col: col}, nil
	case r == ']':
		l.advance()
		return Token{Type: RBRACKET, Value: "]", Line: line, Col: col}, nil
	case r == '"':
		return l.readString(line, col)
	case r == '\'':
		return l.readChar(line, col)
	case r == '`':
		return l.readTag(line, col)
	case unicode.IsDigit(r):
		return l.readNumber(line, col), nil
	case unicode.IsLetter(r) || r == '_':
		return l.readIdentOrKeyword(line, col), nil
	default:
		return Token{}, errAt(CodeUnexpectedChar, line, col, "неожиданный символ %q", r)
	}
}

// skipInsignificantWhitespaceAndComments съедает пробелы, табуляции,
// комментарии вида "// ..." и ОБЫЧНЫЕ блочные комментарии "/* ... */"
// (перенос строки внутри такого комментария не превращается в NEWLINE -
// комментарий целиком поглощается, как пробел). Doc-комментарий "/** ... */"
// - особый случай: он должен стать собственным токеном (DOC_COMMENT, см.
// readDocComment), а не быть съеден здесь - "/**" НЕ ЗАКРЫТОЕ сразу же (не
// "/**/", это по-прежнему обычный пустой блочный комментарий) распознаётся
// по третьему символу, цикл в этом случае просто возвращается, оставляя
// "/**..." нетронутым для next().
func (l *Lexer) skipInsignificantWhitespaceAndComments() error {
	for l.pos < len(l.src) {
		r := l.peek()
		switch {
		case r == ' ' || r == '\t' || r == '\r':
			l.advance()
		case r == '/' && l.peekAt(1) == '/':
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
		case r == '/' && l.peekAt(1) == '*' && isDocCommentStart(l):
			return nil
		case r == '/' && l.peekAt(1) == '*':
			line, col := l.line, l.col
			l.advance()
			l.advance()
			closed := false
			for l.pos < len(l.src) {
				if l.peek() == '*' && l.peekAt(1) == '/' {
					l.advance()
					l.advance()
					closed = true
					break
				}
				l.advance()
			}
			if !closed {
				return errAt(CodeUnclosedComment, line, col, "комментарий не закрыт (нет \"*/\")")
			}
		default:
			return nil
		}
	}
	return nil
}

// isDocCommentStart - true, если позиция лексера стоит ровно на "/**", за
// которым не следует немедленно "/" (иначе это "/**/" - обычный пустой
// блочный комментарий, не doc-комментарий).
func isDocCommentStart(l *Lexer) bool {
	return l.peekAt(2) == '*' && l.peekAt(3) != '/'
}

// readDocComment разбирает "/** ... */" в токен DOC_COMMENT - Value несёт
// текст между "/**" и "*/" как есть (без обрезки отступов построчно).
// Недостающее "*/" до конца файла - ошибка компиляции (как и незакрытая
// строка), а не тихо поглощённый остаток файла.
func (l *Lexer) readDocComment(line, col int) (Token, error) {
	l.advance() // '/'
	l.advance() // '*'
	l.advance() // '*'
	start := l.pos
	for {
		if l.pos >= len(l.src) {
			return Token{}, errAt(CodeUnclosedComment, line, col, "doc-комментарий не закрыт (нет \"*/\")")
		}
		if l.peek() == '*' && l.peekAt(1) == '/' {
			text := string(l.src[start:l.pos])
			l.advance()
			l.advance()
			return Token{Type: DOC_COMMENT, Value: text, Line: line, Col: col}, nil
		}
		l.advance()
	}
}

func (l *Lexer) readIdentOrKeyword(line, col int) Token {
	start := l.pos
	for l.pos < len(l.src) && (unicode.IsLetter(l.peek()) || unicode.IsDigit(l.peek()) || l.peek() == '_') {
		l.advance()
	}
	value := string(l.src[start:l.pos])
	if kw, ok := keywords[value]; ok {
		return Token{Type: kw, Value: value, Line: line, Col: col}
	}
	return Token{Type: IDENT, Value: value, Line: line, Col: col}
}

// hexDigit/octalDigit/binaryDigit - наборы цифр у "0x"/"0o"/"0b"-литералов
// (readNumber ниже) - только ASCII a-f/A-F для hex (в отличие от
// unicode.IsDigit, который берёт ЛЮБую desятичную цифру Unicode - здесь
// это неуместно, шестнадцатеричная цифра - строго ASCII).
func hexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func octalDigit(r rune) bool {
	return r >= '0' && r <= '7'
}

func binaryDigit(r rune) bool {
	return r == '0' || r == '1'
}

func (l *Lexer) readNumber(line, col int) Token {
	start := l.pos
	// "0x"/"0o"/"0b" (шестнадцатеричный/восьмеричный/двоичный литерал) -
	// распознаётся ТОЛЬКО по явному префиксу сразу после ведущего "0", не
	// через "угадывание базы по содержимому" - десятичный литерал с
	// ведущим нулём ("007") остаётся десятичным, как и раньше (см. её же
	// разбор в parser/expr.go, parseIntTokenValue - тем же принципом
	// объяснено там). Дробная часть/экспонента у этих трёх форм не
	// поддержаны (только целые - мотивация была адреса памяти/битовые
	// маски, не научная нотация).
	if l.peek() == '0' {
		switch l.peekAt(1) {
		case 'x', 'X':
			l.advance()
			l.advance()
			for l.pos < len(l.src) && hexDigit(l.peek()) {
				l.advance()
			}
			return Token{Type: INT, Value: string(l.src[start:l.pos]), Line: line, Col: col}
		case 'o', 'O':
			l.advance()
			l.advance()
			for l.pos < len(l.src) && octalDigit(l.peek()) {
				l.advance()
			}
			return Token{Type: INT, Value: string(l.src[start:l.pos]), Line: line, Col: col}
		case 'b', 'B':
			l.advance()
			l.advance()
			for l.pos < len(l.src) && binaryDigit(l.peek()) {
				l.advance()
			}
			return Token{Type: INT, Value: string(l.src[start:l.pos]), Line: line, Col: col}
		}
	}
	for l.pos < len(l.src) && unicode.IsDigit(l.peek()) {
		l.advance()
	}
	isFloat := false
	if l.peek() == '.' && unicode.IsDigit(l.peekAt(1)) {
		isFloat = true
		l.advance() // .
		for l.pos < len(l.src) && unicode.IsDigit(l.peek()) {
			l.advance()
		}
	}
	// Экспонента ("1e10", "2.5e-3", "1.5E+10") - "e"/"E" без хотя бы одной
	// цифры сразу за ним (пропустив необязательный знак) - НЕ экспонента
	// ("1e" сама по себе, без цифр - число "1", "e" остаётся отдельным
	// токеном дальше, тем же принципом, что и одинокая "." без цифры после
	// неё выше не делает число дробным). Экспонента всегда делает литерал
	// FLOAT, даже без десятичной точки в мантиссе ("1e10" - Действительное,
	// не Целое) - та же семантика, что в большинстве языков со scientific
	// notation.
	if r := l.peek(); r == 'e' || r == 'E' {
		digitsAt := 1
		if next := l.peekAt(1); next == '+' || next == '-' {
			digitsAt = 2
		}
		if unicode.IsDigit(l.peekAt(digitsAt)) {
			isFloat = true
			l.advance() // e/E
			if sign := l.peek(); sign == '+' || sign == '-' {
				l.advance()
			}
			for l.pos < len(l.src) && unicode.IsDigit(l.peek()) {
				l.advance()
			}
		}
	}
	value := string(l.src[start:l.pos])
	if isFloat {
		return Token{Type: FLOAT, Value: value, Line: line, Col: col}
	}
	return Token{Type: INT, Value: value, Line: line, Col: col}
}

// readString разбирает строковый литерал "..." - полностью "как написано",
// без единого исключения: "\" - всегда буквальный символ, даже перед "
// или перед другим "\" ("C:\rech" и "https:\\site.ru" печатаются ровно
// так, как набраны). Прямое следствие - вставить символ " внутрь "..."
// невозможно ни в каком виде; для этого нужен backtick-литерал (см.
// readTag), который единственный поддерживает экранирование.
func (l *Lexer) readString(line, col int) (Token, error) {
	l.advance() // открывающая "
	var sb []rune
	for {
		if l.pos >= len(l.src) {
			return Token{}, errAt(CodeBadLiteral, line, col, "строка не закрыта кавычкой")
		}
		r := l.peek()
		if r == '"' {
			l.advance()
			break
		}
		if r == '\n' {
			return Token{}, errAt(CodeBadLiteral, line, col, "перенос строки внутри строкового литерала не поддерживается в v0.1")
		}
		sb = append(sb, r)
		l.advance()
	}
	return Token{Type: STRING, Value: string(sb), Line: line, Col: col}, nil
}

// readChar разбирает символьный литерал ('a', 'я', эскейп-последовательность)
// в одинарных кавычках - ровно один кодовый пункт Unicode внутри, не байт,
// как C char (Rech кириллический по духу, 'я' должен помещаться в один
// Символ). Value токена - сам символ как строка (одна руна) - парсер
// декодирует её обратно в rune для ast.CharLiteral.
func (l *Lexer) readChar(line, col int) (Token, error) {
	l.advance() // открывающая '
	if l.pos >= len(l.src) {
		return Token{}, errAt(CodeBadLiteral, line, col, "символьный литерал не закрыт кавычкой")
	}
	var value rune
	switch r := l.peek(); r {
	case '\'':
		return Token{}, errAt(CodeBadLiteral, line, col, "пустой символьный литерал")
	case '\n':
		return Token{}, errAt(CodeBadLiteral, line, col, "перенос строки внутри символьного литерала не поддерживается")
	case '\\':
		l.advance()
		esc := l.peek()
		switch esc {
		case 'n':
			value = '\n'
		case 't':
			value = '\t'
		case 'r':
			value = '\r'
		case '\'':
			value = '\''
		case '\\':
			value = '\\'
		default:
			return Token{}, errAt(CodeBadEscape, l.line, l.col, "неизвестная последовательность экранирования \\%c", esc)
		}
		l.advance()
	default:
		value = r
		l.advance()
	}
	if l.pos >= len(l.src) || l.peek() != '\'' {
		return Token{}, errAt(CodeBadLiteral, line, col, "символьный литерал должен содержать ровно один символ")
	}
	l.advance() // закрывающая '
	return Token{Type: CHAR, Value: string(value), Line: line, Col: col}, nil
}

// validateUTF8 - небольшая защита от бинарного мусора на входе;
// вызывается из cmd/rech перед лексированием.
func ValidateUTF8(src string) error {
	if !utf8.ValidString(src) {
		return diagnostics.New(CodeBadEncoding, diagnostics.Position{}, "исходный файл не в кодировке UTF-8")
	}
	return nil
}

// ValidateNFC требует, чтобы исходник был в нормализованной форме Unicode
// NFC. Без этого два визуально одинаковых идентификатора (например, "ё" как
// один символ U+0451 против "е" + отдельный диакритический знак U+0308)
// оказываются разными строками и тихо не совпадают друг с другом - в
// таблице символов, при импорте по имени файла и т.п. Явная проверка
// превращает такое несовпадение в понятную ошибку компиляции, а не в
// необъяснимое "модуль/переменная не найдена". Правило и мотивация -
// docs/GRAMMAR_DRAFT.md, раздел "Имена файлов и кодировка".
func ValidateNFC(src string) error {
	if !norm.NFC.IsNormalString(src) {
		return diagnostics.New(CodeBadEncoding, diagnostics.Position{}, "исходный файл не в нормализованной форме Unicode NFC - пересохраните его в редакторе с нормализацией NFC (частая причина - буква \"ё\", набранная как \"е\" с отдельным диакритическим знаком)")
	}
	return nil
}
