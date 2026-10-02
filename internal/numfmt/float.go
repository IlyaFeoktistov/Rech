// Package numfmt - текстовая запись дробного числа, общая для печати,
// "(Строка) x" и сериализации: кратчайшая, читающаяся обратно в то же
// значение. cbackend повторяет тот же алгоритм на C (float_format.go).
package numfmt

import (
	"math"
	"strconv"
	"strings"
)

// Float - кратчайшая запись f, при разборе дающая то же значение в
// точности bitSize (32 - Дробное, 64 - Действительное): 0.1 + 0.2 -
// "0.30000000000000004", Дробное 0.1 - "0.1". Цифры - наименьшая точность
// %e, при которой запись читается обратно в f; порядок от -4 до 15 -
// обычная запись ("0.0001", "123456789012345"), иначе экспонента с хотя бы
// двумя цифрами порядка ("1e-05", "1.5e+16"), как у %g. Бесконечности -
// "inf"/"-inf", не-число - "nan".
func Float(f float64, bitSize int) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	var e string
	for p := 1; p <= 17; p++ {
		e = strconv.FormatFloat(f, 'e', p-1, bitSize)
		if back, err := strconv.ParseFloat(e, bitSize); err == nil && back == f {
			break
		}
	}
	return Layout(e)
}

// Layout - раскладка записи вида "-1.2345e-05" (цифры уже выбраны) в
// обычную или экспоненциальную форму (см. Float).
func Layout(e string) string {
	var sb strings.Builder
	if strings.HasPrefix(e, "-") {
		sb.WriteByte('-')
		e = e[1:]
	}
	mant, expText, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expText)
	digits := strings.TrimRight(strings.Replace(mant, ".", "", 1), "0")
	if digits == "" {
		digits = "0"
	}
	if exp >= -4 && exp < 16 {
		if exp >= 0 {
			for i := 0; i <= exp; i++ {
				if i < len(digits) {
					sb.WriteByte(digits[i])
				} else {
					sb.WriteByte('0')
				}
			}
			if len(digits) > exp+1 {
				sb.WriteByte('.')
				sb.WriteString(digits[exp+1:])
			}
		} else {
			sb.WriteString("0.")
			sb.WriteString(strings.Repeat("0", -exp-1))
			sb.WriteString(digits)
		}
		return sb.String()
	}
	sb.WriteByte(digits[0])
	if len(digits) > 1 {
		sb.WriteByte('.')
		sb.WriteString(digits[1:])
	}
	sign := byte('+')
	if exp < 0 {
		sign, exp = '-', -exp
	}
	sb.WriteByte('e')
	sb.WriteByte(sign)
	if exp < 10 {
		sb.WriteByte('0')
	}
	sb.WriteString(strconv.Itoa(exp))
	return sb.String()
}
