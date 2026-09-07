// Package interp, продолжение - см. package doc internal/semantic/numbase.go.
package interp

import (
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// intToBaseString - "целое_в_hex"/"целое_в_oct"/"целое_в_bin" - форматирует
// БИТОВЫЙ ПАТТЕРН n (uint64(n), не величину - см. package doc numbase.go)
// с префиксом системы счисления, той же формы, что принимают литералы
// "0x"/"0o"/"0b" (lexer.go).
func intToBaseString(n int64, prefix string, base int) Value {
	return stringValue(prefix + strconv.FormatUint(uint64(n), base))
}

// intFromBaseString - "целое_из_hex"/"целое_из_oct"/"целое_из_bin" -
// обратное: необязательный префикс (регистронезависимо), затем цифры
// своей системы счисления - bit-паттерн, не проверка на переполнение
// величины (см. package doc numbase.go).
func intFromBaseString(op, s, prefixLower string, base int) Value {
	digits := s
	if len(s) >= len(prefixLower) && strings.EqualFold(s[:len(prefixLower)], prefixLower) {
		digits = s[len(prefixLower):]
	}
	n, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		panicRuntime("%s", rtmsg.NumBaseParseFailedFormat(op, s))
	}
	return intValue(int64(n))
}
