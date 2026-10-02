package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// decimalArithFuncs - C-функции арифметики "Десятичное" (decimalRuntime).
var decimalArithFuncs = map[string]string{
	"+": "rech_dec_add",
	"-": "rech_dec_sub",
	"*": "rech_dec_mul",
	"/": "rech_dec_div",
	"%": "rech_dec_rem",
}

// decimalBinaryOp - бинарная операция над двумя "Десятичное": арифметика -
// вызов rech_dec_*, сравнение - знак rech_dec_cmp.
func decimalBinaryOp(ctx *irGenCtx, op, lc, rc string) (string, valueType, error) {
	ctx.usesDecimal = true
	if fn, ok := decimalArithFuncs[op]; ok {
		return fmt.Sprintf("%s(%s, %s)", fn, lc, rc), typeDecimal, nil
	}
	switch op {
	case "==", "!=", "<", ">", "<=", ">=":
		return fmt.Sprintf("(rech_dec_cmp(%s, %s) %s 0)", lc, rc, op), typeBool, nil
	}
	return "", typeUnknown, fmt.Errorf("генерация C из IR: операция %q не определена для Десятичное", op)
}

// decimalLiteralCode - константа Десятичное как составной литерал rech_dec.
func decimalLiteralCode(ctx *irGenCtx, e *ir.DecimalLiteral) string {
	ctx.usesDecimal = true
	neg := 0
	if e.Value.Neg {
		neg = 1
	}
	return fmt.Sprintf("((rech_dec){%dULL, %du, %d, %d})", e.Value.Lo, e.Value.Hi, e.Value.Scale, neg)
}

// decimalCastCode - "(Тип) x", где Десятичное - исходный или целевой тип;
// ok=false - пара не про Десятичное.
func decimalCastCode(ctx *irGenCtx, code string, from, to valueType, toName string) (string, bool) {
	if from != typeDecimal && to != typeDecimal {
		return "", false
	}
	ctx.usesDecimal = true
	if from == to {
		return code, true
	}
	if to == typeDecimal {
		switch {
		case from == typeString:
			return fmt.Sprintf("rech_dec_from_text(%s, \"Десятичное\")", code), true
		case from == typeFloat || from == typeFloat32:
			return fmt.Sprintf("rech_dec_from_double((double)(%s))", code), true
		case isUnsignedFixedWidth(from):
			return fmt.Sprintf("rech_dec_from_uint((unsigned long long)(%s))", code), true
		default:
			return fmt.Sprintf("rech_dec_from_int((long long)(%s))", code), true
		}
	}
	switch {
	case to == typeString:
		ctx.usesCast = true
		return fmt.Sprintf("({ char rech_db[48]; rech_dec_to_text(rech_db, %s); rech_cast_alloc(rech_db, strlen(rech_db)); })", code), true
	case to == typeFloat || to == typeFloat32:
		return fmt.Sprintf("((%s)rech_dec_to_double(%s))", to.cType(ctx), code), true
	default:
		return fmt.Sprintf("((%s)rech_dec_to_int(%s, \"%s\"))", to.cType(ctx), code, escapeCString(toName)), true
	}
}

// decimalIncDecCode - "д++"/"++д"/"д--"/"--д": присваивание д -+ 1,
// значение - старое (постфикс) или новое (префикс), как у чисел в C.
func decimalIncDecCode(ctx *irGenCtx, name, op string, prefix bool) string {
	ctx.usesDecimal = true
	fn := "rech_dec_add"
	if op == "--" {
		fn = "rech_dec_sub"
	}
	update := fmt.Sprintf("%s = %s(%s, rech_dec_from_int(1))", name, fn, name)
	if prefix {
		return fmt.Sprintf("(%s)", update)
	}
	return fmt.Sprintf("({ rech_dec rech_old = %s; %s; rech_old; })", name, update)
}
