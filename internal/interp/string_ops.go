package interp

import (
	"strings"
	"unicode/utf8"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// stringLength/substring/stringFind - "длина_строки"/"подстрока"/
// "позиция_в_строке" (internal/builtins.All) - индексация по КОДОВЫМ
// ТОЧКАМ Unicode, не байтам, тем же принципом, что и "Символ" - см.
// package doc string_ops.go в cbackend.
func stringLength(s string) int64 {
	return int64(utf8.RuneCountInString(s))
}

func substring(s string, start, length int64) string {
	r := []rune(s)
	if start < 0 || length < 0 || start+length > int64(len(r)) {
		panicRuntime("%s", rtmsg.SubstringInvalidRange)
	}
	return string(r[start : start+length])
}

func stringFind(s, needle string) int64 {
	before, _, found := strings.Cut(s, needle)
	if !found {
		return -1
	}
	return int64(utf8.RuneCountInString(before))
}
