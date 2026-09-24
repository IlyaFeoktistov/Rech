// Package textutf8 - правило "Строка - валидный UTF-8 без NUL" (docs/
// SPEC.md, раздел 5): байты, приходящие извне (файлы, сокеты, процессы,
// БД, FFI, строка_из_байт, декодеры), становятся Строкой через Fix -
// каждая невалидная последовательность и каждый NUL заменяются на U+FFFD.
// Невалидность - по правилам utf8.DecodeRune (overlong, суррогаты,
// > U+10FFFF, оборванная последовательность): при ошибке заменяется один
// байт, разбор продолжается со следующего. Та же функция на C -
// internal/backend/cbackend/runtime/strutil/utf8text.go (оба backend'а
// обязаны давать один и тот же результат).
package textutf8

import (
	"strings"
	"unicode/utf8"
)

// Replacement - U+FFFD в UTF-8.
const Replacement = "�"

// Fix - s как валидный текст (см. package doc). Уже валидная строка без
// NUL возвращается как есть.
func Fix(s string) string {
	if utf8.ValidString(s) && strings.IndexByte(s, 0) < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && size == 1) || r == 0 {
			b.WriteString(Replacement)
			i++
			continue
		}
		b.WriteString(s[i : i+size])
		i += size
	}
	return b.String()
}

// IncompleteTail - сколько последних байт b (0..3) - начало символа,
// оборванное концом буфера, но само по себе ещё валидное: при чтении
// кусками они не заменяются, а откладываются до следующего чтения.
func IncompleteTail(b []byte) int {
	for k := 1; k <= 3 && k <= len(b); k++ {
		start := len(b) - k
		c := b[start]
		if c < 0x80 {
			return 0
		}
		if c >= 0xC0 {
			// Нашли начало последовательности: оборвана ли она?
			if !utf8.FullRune(b[start:]) && validPrefix(b[start:]) {
				return k
			}
			return 0
		}
	}
	return 0
}

// validPrefix - p (короче полного символа) может быть началом валидной
// последовательности: те же границы второго байта, что у utf8.DecodeRune.
func validPrefix(p []byte) bool {
	c := p[0]
	lo, hi := byte(0x80), byte(0xBF)
	switch {
	case c >= 0xC2 && c <= 0xDF, c >= 0xE1 && c <= 0xEC, c == 0xEE, c == 0xEF, c >= 0xF1 && c <= 0xF3:
	case c == 0xE0:
		lo = 0xA0
	case c == 0xED:
		hi = 0x9F
	case c == 0xF0:
		lo = 0x90
	case c == 0xF4:
		hi = 0x8F
	default:
		return false
	}
	for i := 1; i < len(p); i++ {
		if i == 1 && (p[i] < lo || p[i] > hi) {
			return false
		}
		if i > 1 && (p[i] < 0x80 || p[i] > 0xBF) {
			return false
		}
	}
	return true
}
