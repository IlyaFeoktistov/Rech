package lexer

import (
	"fmt"
	"unicode"
	"unicode/utf8"
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

func New(src string) *Lexer {
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
	l.skipInsignificantWhitespaceAndComments()

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
	case r == '+':
		l.advance()
		return Token{Type: PLUS, Value: "+", Line: line, Col: col}, nil
	case r == '-':
		l.advance()
		return Token{Type: MINUS, Value: "-", Line: line, Col: col}, nil
	case r == '*':
		l.advance()
		return Token{Type: STAR, Value: "*", Line: line, Col: col}, nil
	case r == '/':
		l.advance()
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
		return Token{Type: LT, Value: "<", Line: line, Col: col}, nil
	case r == '>':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: GE, Value: ">=", Line: line, Col: col}, nil
		}
		return Token{Type: GT, Value: ">", Line: line, Col: col}, nil
	case r == '"':
		return l.readString(line, col)
	case unicode.IsDigit(r):
		return l.readNumber(line, col), nil
	case unicode.IsLetter(r) || r == '_':
		return l.readIdentOrKeyword(line, col), nil
	default:
		return Token{}, fmt.Errorf("строка %d, столбец %d: неожиданный символ %q", line, col, r)
	}
}

// skipInsignificantWhitespaceAndComments съедает пробелы, табуляции и
// комментарии вида "// ...". Переносы строк здесь не трогаем - они значимы
// как разделители инструкций и превращаются в токен NEWLINE.
func (l *Lexer) skipInsignificantWhitespaceAndComments() {
	for l.pos < len(l.src) {
		r := l.peek()
		switch {
		case r == ' ' || r == '\t' || r == '\r':
			l.advance()
		case r == '/' && l.peekAt(1) == '/':
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
		default:
			return
		}
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
			case '"':
				sb = append(sb, '"')
			case '\\':
				sb = append(sb, '\\')
			default:
				return Token{}, fmt.Errorf("строка %d, столбец %d: неизвестная escape-последовательность \\%c", l.line, l.col, esc)
			}
			l.advance()
			continue
		}
		sb = append(sb, r)
		l.advance()
	}
	return Token{Type: STRING, Value: string(sb), Line: line, Col: col}, nil
}

// validateUTF8 - небольшая защита от бинарного мусора на входе;
// вызывается из cmd/rech перед лексированием.
func ValidateUTF8(src string) error {
	if !utf8.ValidString(src) {
		return fmt.Errorf("исходный файл не в кодировке UTF-8")
	}
	return nil
}
