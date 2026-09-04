package lexer

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Lexer превращает исходный текст на Rech в поток токенов.
// Идентификаторы и ключевые слова опираются на unicode.IsLetter,
// поэтому кириллица обрабатывается так же естественно, как латиница.
type Lexer struct {
	src  []rune
	pos  int
	line int
	col  int
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
		tokens = append(tokens, tok)
		if tok.Type == EOF {
			break
		}
	}
	return tokens, nil
}

func (l *Lexer) next() (Token, error) {
	if err := l.skipInsignificantWhitespaceAndComments(); err != nil {
		return Token{}, err
	}

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
		return Token{}, fmt.Errorf("строка %d, столбец %d: одиночный \"!\" не поддерживается, используйте \"не\"", line, col)
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
		return Token{}, fmt.Errorf("строка %d, столбец %d: неожиданный символ %q", line, col, r)
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
				return fmt.Errorf("строка %d, столбец %d: комментарий не закрыт (нет \"*/\")", line, col)
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
			return Token{}, fmt.Errorf("строка %d, столбец %d: doc-комментарий не закрыт (нет \"*/\")", line, col)
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

func (l *Lexer) readNumber(line, col int) Token {
	start := l.pos
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
	value := string(l.src[start:l.pos])
	if isFloat {
		return Token{Type: FLOAT, Value: value, Line: line, Col: col}
	}
	return Token{Type: INT, Value: value, Line: line, Col: col}
}

func (l *Lexer) readString(line, col int) (Token, error) {
	l.advance() // открывающая "
	var sb []rune
	for {
		if l.pos >= len(l.src) {
			return Token{}, fmt.Errorf("строка %d, столбец %d: строка не закрыта кавычкой", line, col)
		}
		r := l.peek()
		if r == '"' {
			l.advance()
			break
		}
		if r == '\n' {
			return Token{}, fmt.Errorf("строка %d, столбец %d: перенос строки внутри строкового литерала не поддерживается в v0.1", line, col)
		}
		if r == '\\' {
			l.advance()
			esc := l.peek()
			switch esc {
			case 'n':
				sb = append(sb, '\n')
			case 't':
				sb = append(sb, '\t')
			case 'r':
				sb = append(sb, '\r')
			case '"':
				sb = append(sb, '"')
			case '\\':
				sb = append(sb, '\\')
			default:
				return Token{}, fmt.Errorf("строка %d, столбец %d: неизвестная последовательность экранирования \\%c", l.line, l.col, esc)
			}
			l.advance()
			continue
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
		return Token{}, fmt.Errorf("строка %d, столбец %d: символьный литерал не закрыт кавычкой", line, col)
	}
	var value rune
	switch r := l.peek(); r {
	case '\'':
		return Token{}, fmt.Errorf("строка %d, столбец %d: пустой символьный литерал", line, col)
	case '\n':
		return Token{}, fmt.Errorf("строка %d, столбец %d: перенос строки внутри символьного литерала не поддерживается", line, col)
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
			return Token{}, fmt.Errorf("строка %d, столбец %d: неизвестная последовательность экранирования \\%c", l.line, l.col, esc)
		}
		l.advance()
	default:
		value = r
		l.advance()
	}
	if l.pos >= len(l.src) || l.peek() != '\'' {
		return Token{}, fmt.Errorf("строка %d, столбец %d: символьный литерал должен содержать ровно один символ", line, col)
	}
	l.advance() // закрывающая '
	return Token{Type: CHAR, Value: string(value), Line: line, Col: col}, nil
}

// readTag разбирает содержимое между обратными кавычками - ОДИН и тот же
// токен (TAG) обслуживает ДВЕ разные синтаксические роли, различаемые
// парсером по ПОЗИЦИИ (не лексером): тег поля/метода (`json:"name"` и
// т. п., см. internal/structtag) в позиции перед объявлением поля/метода -
// парсер просто берёт .Value как есть, ничего в нём не разбирая; шаблонная
// строка (`Привет, ${имя}!`) в позиции выражения - internal/parser
// дополнительно разбирает .Value на текстовые куски и "${...}"-вставки
// (см. internal/parser/template.go). Настоящий перенос строки в исходнике -
// уже настоящий перенос строки в значении (многострочность работает без
// какого-либо спецсимвола, как в heredoc с незакавыченным маркером); сверх
// этого поддерживается небольшой набор explicit-экранирований для
// символов, которые неудобно/невозможно набрать как есть.
func (l *Lexer) readTag(line, col int) (Token, error) {
	l.advance() // "`"
	var sb []rune
	for {
		if l.pos >= len(l.src) {
			return Token{}, fmt.Errorf("строка %d, столбец %d: строка в обратных кавычках не закрыта (нет \"`\")", line, col)
		}
		r := l.peek()
		if r == '`' {
			l.advance()
			break
		}
		if r == '\\' {
			l.advance()
			esc := l.peek()
			switch esc {
			case 'n':
				sb = append(sb, '\n')
			case 't':
				sb = append(sb, '\t')
			case 'r':
				sb = append(sb, '\r')
			case '`':
				sb = append(sb, '`')
			case '\\':
				sb = append(sb, '\\')
			case '$':
				sb = append(sb, '$')
			default:
				return Token{}, fmt.Errorf("строка %d, столбец %d: неизвестная последовательность экранирования \\%c", l.line, l.col, esc)
			}
			l.advance()
			continue
		}
		sb = append(sb, r)
		l.advance()
	}
	return Token{Type: TAG, Value: string(sb), Line: line, Col: col}, nil
}

// validateUTF8 - небольшая защита от бинарного мусора на входе;
// вызывается из cmd/rech перед лексированием.
func ValidateUTF8(src string) error {
	if !utf8.ValidString(src) {
		return fmt.Errorf("исходный файл не в кодировке UTF-8")
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
// docs/GRAMMAR.md, раздел "Имена файлов и кодировка".
func ValidateNFC(src string) error {
	if !norm.NFC.IsNormalString(src) {
		return fmt.Errorf("исходный файл не в нормализованной форме Unicode NFC - пересохраните его в редакторе с нормализацией NFC (частая причина - буква \"ё\", набранная как \"е\" с отдельным диакритическим знаком)")
	}
	return nil
}
