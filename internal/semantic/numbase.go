// Package semantic, продолжение - "целое_в_hex"/"целое_из_hex" и то же для
// "oct"/"bin" (Шаг 98, docs/ROADMAP.md) - обратное к литералам "0x"/"0o"/
// "0b" (lexer.go/parser/expr.go, parseIntTokenValue): число <-> строка в
// нужной системе счисления, тем же принципом, что и "rgbцвет_из_hex"/
// "rgbцвет_в_hex" (color.go), но без сущности - оба конца только примитивы
// (Целое/Строка), поэтому регистрация проще (не нужна ленивая регистрация
// типа при вызове).
//
// "целое_в_*" форматирует БИТОВЫЙ ПАТТЕРН n (uint64(n)), не проверяет знак -
// мотивация та же, что у самих hex/oct/bin-литералов (адреса/битовые маски,
// не "число со знаком минус спереди") - позволяет заодно выразить полный
// 64-битный диапазон, недоступный десятичному литералу напрямую (docs/
// ROADMAP.md, Шаг 65/примечание про 18446744073709551615 через "0 - 1").
// "целое_из_*" разбирает ОБРАТНО тем же путём (bit-паттерн, не величина) -
// "целое_из_hex(целое_в_hex(-1))" возвращает -1, не переполнение.
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

func registerNumberBaseFuncs(funcs map[string]funcSig) {
	for _, name := range []string{"целое_в_hex", "целое_в_oct", "целое_в_bin"} {
		funcs[name] = funcSig{Params: []ir.Type{ir.TypeInt}, ReturnType: ir.TypeString, HasReturn: true}
	}
	for _, name := range []string{"целое_из_hex", "целое_из_oct", "целое_из_bin"} {
		funcs[name] = funcSig{Params: []ir.Type{ir.TypeString}, ReturnType: ir.TypeInt, HasReturn: true}
	}
}
