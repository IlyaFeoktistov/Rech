package interp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// wrapFixedWidth усекает/переносит raw (полный 64-битный битовый паттерн
// результата операции) до реальной ширины типа k - Целое8/16/32/64,
// Натуральное/8/16/32/64. Та же операция, что явный C-каст в
// cbackend.generateIRArithmetic/generateIRUnaryExpr выполняет НА УРОВНЕ
// ВЫРАЖЕНИЯ (не только при присваивании) - без неё промежуточный результат
// остался бы в более широком int64, разойдясь со скомпилированной
// программой при переполнении. Для k без фиксированной ширины (обычное
// Целое, 64 бита) возвращает raw неизменным.
func wrapFixedWidth(k ir.TypeKind, raw int64) int64 {
	bits, signed, ok := k.IntWidthSigned()
	if !ok || bits >= 64 {
		return raw
	}
	mask := uint64(1)<<uint(bits) - 1
	v := uint64(raw) & mask
	if signed && v&(uint64(1)<<uint(bits-1)) != 0 {
		return int64(v) - (int64(1) << uint(bits))
	}
	return int64(v)
}

// isFixedWidthInt - k один из девяти типов Целое8/16/32/64, Натуральное/
// 8/16/32/64, НЕ обычное Целое - им нужна собственная ветка
// (сохранить исходный Value.Type, усечь по ширине через wrapFixedWidth)
// вместо промоции к Целое, которую получает обычная целочисленная
// арифметика.
func isFixedWidthInt(k ir.TypeKind) bool {
	_, _, ok := k.IntWidthSigned()
	return ok && k != ir.KindInt
}

// isUnsignedFixedWidth - k один из Натуральное/Натуральное8/16/32/64.
// Натуральное64 может нести значение выше INT64_MAX, битовый паттерн
// которого выглядит отрицательным в Value.I (int64) - деление/сравнение/
// касты со/в "Строка" для таких k должны явно переинтерпретировать I как
// uint64, а не работать с ним как со знаковым числом напрямую.
func isUnsignedFixedWidth(k ir.TypeKind) bool {
	_, signed, ok := k.IntWidthSigned()
	return ok && !signed
}

// fixedWidthToFloat - значение k (Целое8/16/32/64, Натуральное/8/16/32/64
// или обычное Целое/Байт) как float64 - через uint64(raw), не int64(raw),
// когда k беззнаковый, иначе Натуральное64 > INT64_MAX превратился бы в
// отрицательное float64 (та же причина, что и у fixedWidthArithmetic."/").
func fixedWidthToFloat(k ir.TypeKind, raw int64) float64 {
	if isUnsignedFixedWidth(k) {
		return float64(uint64(raw))
	}
	return float64(raw)
}

// fixedWidthArithmetic - "+"/"-"/"*"/"/" между двумя значениями ОДНОГО И
// ТОГО ЖЕ Целое8/16/32/64 / Натуральное/8/16/32/64 (internal/semantic уже
// гарантировал l.Type == r.Type, analyzeArithmetic - строго один и тот же
// тип, без неявной промоции). "/" беззнаковых - через uint64(...), как и
// cbackend.rech_uint_div - обычное int64-деление интерпретировало бы
// Натуральное64 > INT64_MAX как отрицательное и дало бы неверный частное.
func fixedWidthArithmetic(op string, l, r Value) Value {
	k := l.Type.Kind
	switch op {
	case "+":
		return Value{Type: l.Type, I: wrapFixedWidth(k, l.I+r.I)}
	case "-":
		return Value{Type: l.Type, I: wrapFixedWidth(k, l.I-r.I)}
	case "*":
		return Value{Type: l.Type, I: wrapFixedWidth(k, l.I*r.I)}
	case "/":
		if r.I == 0 {
			panicRuntime("%s", rtmsg.DivisionByZero)
		}
		if isUnsignedFixedWidth(k) {
			return Value{Type: l.Type, I: wrapFixedWidth(k, int64(uint64(l.I)/uint64(r.I)))}
		}
		return Value{Type: l.Type, I: wrapFixedWidth(k, l.I/r.I)}
	}
	return Value{}
}
