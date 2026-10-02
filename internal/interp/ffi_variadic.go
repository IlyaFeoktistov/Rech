package interp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Вариативная C-функция ("..." у "внешняя функция"): аргументы сверх
// объявленных параметров передаются со своими типами, расширенными по
// правилам C (default argument promotions): Дробное -> double, целые уже
// int и Логическое -> int.

// cVariadicArgs - типы параметров конкретного вызова (объявленные плюс
// расширенные типы аргументов "...") и сами аргументы после расширения.
func (in *Interp) cVariadicArgs(decl *ir.ExternFuncDecl, args []Value) ([]ir.Type, []Value) {
	params := append([]ir.Type(nil), decl.Params...)
	out := append([]Value(nil), args[:len(decl.Params)]...)
	for _, a := range args[len(decl.Params):] {
		p := promoteCVariadic(a)
		params = append(params, p.Type)
		out = append(out, p)
	}
	return params, out
}

// promoteCVariadic - v после расширения аргумента "..." по правилам C.
func promoteCVariadic(v Value) Value {
	switch v.Type.Kind {
	case ir.KindFloat32:
		return Value{Type: ir.TypeFloat, F: v.F}
	case ir.KindBool:
		i := int64(0)
		if v.B {
			i = 1
		}
		return Value{Type: ir.TypeInt32, I: i}
	case ir.KindInt8, ir.KindInt16, ir.KindByte, ir.KindUint16:
		return Value{Type: ir.TypeInt32, I: v.I}
	}
	return v
}

// cVariadicKey - ключ кэша Go-сигнатуры вызова alias с типами params.
func cVariadicKey(alias string, params []ir.Type) string {
	var b strings.Builder
	b.WriteString(alias)
	for _, p := range params {
		b.WriteByte('|')
		b.WriteString(p.String())
	}
	return b.String()
}
