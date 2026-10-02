// Package typename - канонические имена составных типов, общие для
// парсера (имя типа из исходника) и семантики (имя по уже известному
// типу элемента).
package typename

import (
	"strconv"
	"strings"
)

// Array - каноническое имя массива из n элементов elem: новое
// измерение - внешнее, поэтому встаёт сразу за базовым типом, перед
// измерениями elem ("Целое[4]" из 3 - "Целое[3][4]", как в C).
func Array(elem string, n int64) string {
	base, dims := SplitArrayDims(elem)
	return base + "[" + strconv.FormatInt(n, 10) + "]" + dims
}

// SplitArrayDims - базовый тип и хвост измерений "[N]..." у имени name
// ("Целое[3][4]" -> "Целое", "[3][4]"). У не-массива dims пусто.
func SplitArrayDims(name string) (base, dims string) {
	end := len(name)
	for end > 0 && name[end-1] == ']' {
		open := strings.LastIndexByte(name[:end], '[')
		if open < 0 || open+1 >= end-1 {
			break
		}
		if _, err := strconv.ParseInt(name[open+1:end-1], 10, 64); err != nil {
			break
		}
		end = open
	}
	return name[:end], name[end:]
}

// OuterDim - внешнее измерение массива name: тип элемента и длина
// ("Целое[3][4]" -> "Целое[4]", 3). ok == false - name не массив.
func OuterDim(name string) (elem string, n int64, ok bool) {
	base, dims := SplitArrayDims(name)
	if dims == "" {
		return "", 0, false
	}
	closeIdx := strings.IndexByte(dims, ']')
	n, err := strconv.ParseInt(dims[1:closeIdx], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return base + dims[closeIdx+1:], n, true
}
