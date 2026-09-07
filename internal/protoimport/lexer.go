// Package protoimport переводит ".proto"-контракт (Protocol Buffers,
// синтаксис proto3) в Rech-исходник - сущности/структуры с полями,
// помеченными тегом “ `protobuf:<номер>` “, совместимые с уже
// существующим wire-кодеком (internal/interp/protobuf.go, internal/
// backend/cbackend/protobuf.go, internal/semantic/protobuf.go) - см.
// package doc generate.go для полного описания того, что поддержано, а что
// - явная, документированная ошибка вместо тихого несовпадения wire-формата.
//
// Разбор ".proto" - собственный небольшой лексер/парсер (не сторонняя
// библиотека, тем же принципом минимализма, что и JSON/XML/YAML/regex в
// остальном компиляторе) - грамматика ".proto" (protobuf.dev/reference/
// protobuf/proto3-spec) значительно проще самого Rech, отдельный полный
// EBNF не нужен для среза, который здесь реализован.
package protoimport

import (
	"fmt"
	"strconv"
	"strings"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokInt
	tokString
	tokPunct // одиночный символ пунктуации - "{", "}", "(", ")", "[", "]", ";", "=", ",", "<", ">", "."
)

type token struct {
	kind  tokenKind
	value string
	line  int
}

type lexer struct {
	src  []rune
	pos  int
	line int
}

func newLexer(src string) *lexer {
	return &lexer{src: []rune(src), line: 1}
}

// tokenize разбирает src целиком в список токенов (включая завершающий
// tokEOF) - файл ".proto" - обычно небольшой контракт, весь текст сразу в
// памяти не проблема (тот же подход, что и у остального разбора формата в
// этом компиляторе).
func tokenize(src string) ([]token, error) {
	l := newLexer(src)
	var out []token
	for {
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		out = append(out, tok)
		if tok.kind == tokEOF {
			return out, nil
		}
	}
}

func (l *lexer) peekRune() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *lexer) advance() rune {
	r := l.src[l.pos]
	l.pos++
	if r == '\n' {
		l.line++
	}
	return r
}

func (l *lexer) skipWhitespaceAndComments() error {
	for l.pos < len(l.src) {
		r := l.peekRune()
		switch {
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			l.advance()
		case r == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/':
			for l.pos < len(l.src) && l.peekRune() != '\n' {
				l.advance()
			}
		case r == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*':
			startLine := l.line
			l.advance()
			l.advance()
			closed := false
			for l.pos < len(l.src) {
				if l.peekRune() == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
					l.advance()
					l.advance()
					closed = true
					break
				}
				l.advance()
			}
			if !closed {
				return fmt.Errorf("строка %d: незакрытый комментарий /*", startLine)
			}
		default:
			return nil
		}
	}
	return nil
}

func isIdentStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isIdentCont(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}

func (l *lexer) next() (token, error) {
	if err := l.skipWhitespaceAndComments(); err != nil {
		return token{}, err
	}
	if l.pos >= len(l.src) {
		return token{kind: tokEOF, line: l.line}, nil
	}
	line := l.line
	r := l.peekRune()
	switch {
	case isIdentStart(r):
		start := l.pos
		for l.pos < len(l.src) && isIdentCont(l.peekRune()) {
			l.advance()
		}
		return token{kind: tokIdent, value: string(l.src[start:l.pos]), line: line}, nil
	case r >= '0' && r <= '9':
		start := l.pos
		for l.pos < len(l.src) && (l.peekRune() >= '0' && l.peekRune() <= '9' || l.peekRune() == 'x' || l.peekRune() == '.' || (l.peekRune() >= 'a' && l.peekRune() <= 'f') || (l.peekRune() >= 'A' && l.peekRune() <= 'F')) {
			l.advance()
		}
		return token{kind: tokInt, value: string(l.src[start:l.pos]), line: line}, nil
	case r == '"' || r == '\'':
		quote := r
		l.advance()
		var sb strings.Builder
		for {
			if l.pos >= len(l.src) {
				return token{}, fmt.Errorf("строка %d: незакрытая строка", line)
			}
			c := l.advance()
			if c == quote {
				break
			}
			if c == '\\' && l.pos < len(l.src) {
				sb.WriteRune(l.advance())
				continue
			}
			sb.WriteRune(c)
		}
		return token{kind: tokString, value: sb.String(), line: line}, nil
	case strings.ContainsRune("{}()[];=,<>.-+", r):
		l.advance()
		return token{kind: tokPunct, value: string(r), line: line}, nil
	default:
		return token{}, fmt.Errorf("строка %d: неожиданный символ %q", line, r)
	}
}

// parseIntLiteral разбирает числовой литерал токена tokInt (десятичный или
// шестнадцатеричный "0x...", отрицательные номера полей/значения enum'ов в
// ".proto" синтаксически невозможны для номера поля, но у значений enum
// - обычное дело, потому знак разбирается отдельно вызывающей стороной, не
// здесь).
func parseIntLiteral(s string) (int64, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return strconv.ParseInt(s[2:], 16, 64)
	}
	return strconv.ParseInt(s, 10, 64)
}
