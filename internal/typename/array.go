// Package typename - канонические имена составных типов, общие для
// парсера (имя типа из исходника) и семантики (имя по уже известному
// типу элемента).
package typename

import (
	"strconv"
	"strings"
	"unicode"
)

// Array - каноническое имя массива из n элементов elem: новое
// измерение - внешнее, поэтому встаёт сразу за базовым типом, перед
// измерениями elem ("Целое[4]" из 3 - "Целое[3][4]", как в C).
func Array(elem string, n int64) string {
	return ArrayOf(elem, strconv.FormatInt(n, 10))
}

// ArrayOf - то же, что Array, с длиной size как написано: число или имя
// константы ("Байт[РАЗМЕР]" - её значение подставляет семантика).
func ArrayOf(elem, size string) string {
	base, dims := SplitArrayDims(elem)
	return base + "[" + size + "]" + dims
}

// SplitArrayDims - базовый тип и хвост измерений "[N]..." у имени name
// ("Целое[3][4]" -> "Целое", "[3][4]"). У не-массива dims пусто.
func SplitArrayDims(name string) (base, dims string) {
	end := len(name)
	for end > 0 && name[end-1] == ']' {
		open := strings.LastIndexByte(name[:end], '[')
		if open < 0 || !isDimSize(name[open+1:end-1]) {
			break
		}
		end = open
	}
	return name[:end], name[end:]
}

// OuterDim - внешнее измерение массива name: тип элемента и длина как
// написана - число или имя константы ("Целое[3][4]" -> "Целое[4]", "3").
// ok == false - name не массив.
func OuterDim(name string) (elem, size string, ok bool) {
	base, dims := SplitArrayDims(name)
	if dims == "" {
		return "", "", false
	}
	closeIdx := strings.IndexByte(dims, ']')
	return base + dims[closeIdx+1:], dims[1:closeIdx], true
}

// isDimSize - s годится длиной массива: десятичное число или имя.
func isDimSize(s string) bool {
	if s == "" {
		return false
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return true
	}
	for i, r := range s {
		if !(unicode.IsLetter(r) || r == '_' || i > 0 && unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
