package interp

import (
	"math"
	"strconv"

	"github.com/IlyaFeoktistov/Rech/internal/decimal"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/numfmt"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// "Десятичное" - internal/decimal; ошибки арифметики - те же паники и
// сообщения, что у rech_dec_* в cbackend (decimal_runtime.go).

func decimalValue(d decimal.Decimal) Value { return Value{Type: ir.TypeDecimal, Dec: d} }

// decimalResult - результат операции или паника (переполнение - Паника,
// деление на ноль - ДелениеНаНоль).
func decimalResult(d decimal.Decimal, err error) Value {
	switch err {
	case nil:
		return decimalValue(d)
	case decimal.ErrDivByZero:
		panicRuntimeAs(rtmsg.PanicDivisionByZero, "%s", rtmsg.DivisionByZero)
	default:
		panicRuntime("%s", rtmsg.DecimalOverflow)
	}
	return Value{}
}

var decimalOps = map[string]func(a, b decimal.Decimal) (decimal.Decimal, error){
	"+": decimal.Add,
	"-": decimal.Sub,
	"*": decimal.Mul,
	"/": decimal.Div,
	"%": decimal.Rem,
}

// decimalBinary - op над двумя Десятичное (семантика уже привела операнды
// к одному типу).
func decimalBinary(op string, l, r Value) (Value, bool) {
	if l.Type != ir.TypeDecimal || r.Type != ir.TypeDecimal {
		return Value{}, false
	}
	if f, ok := decimalOps[op]; ok {
		return decimalResult(f(l.Dec, r.Dec)), true
	}
	c := decimal.Cmp(l.Dec, r.Dec)
	switch op {
	case "==":
		return boolValue(c == 0), true
	case "!=":
		return boolValue(c != 0), true
	case "<":
		return boolValue(c < 0), true
	case ">":
		return boolValue(c > 0), true
	case "<=":
		return boolValue(c <= 0), true
	case ">=":
		return boolValue(c >= 0), true
	}
	return Value{}, false
}

// castDecimal - "(Тип) x", где Десятичное - исходный или целевой тип;
// ok=false - пара не про Десятичное.
func castDecimal(v Value, to ir.Type) (Value, bool) {
	from := v.Type
	if from != ir.TypeDecimal && to != ir.TypeDecimal || to.Kind == ir.KindAny {
		return Value{}, false
	}
	if to == ir.TypeDecimal {
		switch {
		case from == ir.TypeString:
			d, err := decimal.Parse(v.S)
			if err == decimal.ErrOverflow {
				panicRuntime("%s", rtmsg.DecimalOverflow)
			}
			if err != nil {
				throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.CastStrToFloatFailedFormat("Десятичное", v.S))
			}
			return decimalValue(d), true
		case from == ir.TypeFloat || from == ir.TypeFloat32:
			if math.IsNaN(v.F) || math.IsInf(v.F, 0) {
				panicRuntimeAs(rtmsg.PanicBadCast, "%s", rtmsg.CastStrToFloatFailedFormat("Десятичное", numfmt.Float(v.F, floatBits(from))))
			}
			return decimalResult(decimal.Parse(numfmt.Float(v.F, floatBits(from)))), true
		case isUnsignedFixedWidth(from.Kind):
			return decimalValue(decimal.FromUint(uint64(v.I))), true
		default:
			return decimalValue(decimal.FromInt(v.I)), true
		}
	}
	switch {
	case to == ir.TypeString:
		return stringValue(v.Dec.String()), true
	case to == ir.TypeFloat:
		return floatValue(decimalToFloat(v.Dec)), true
	case to == ir.TypeFloat32:
		return float32Value(decimalToFloat(v.Dec)), true
	}
	n, ok := decimal.ToInt(v.Dec)
	if !ok {
		panicRuntimeAs(rtmsg.PanicBadCast, "%s", rtmsg.DecimalToIntOverflowFormat(to.String(), v.Dec.String()))
	}
	switch {
	case to == ir.TypeByte:
		return Value{Type: to, I: int64(uint8(n))}, true
	case isFixedWidthInt(to.Kind):
		return Value{Type: to, I: wrapFixedWidth(to.Kind, n)}, true
	}
	return intValue(n), true
}

// decimalToFloat - ближайшее double (через запись числа, как strtod в
// cbackend).
func decimalToFloat(d decimal.Decimal) float64 {
	f, _ := strconv.ParseFloat(d.String(), 64)
	return f
}

// floatBits - разрядность дробного типа t для записи его значения.
func floatBits(t ir.Type) int {
	if t == ir.TypeFloat32 {
		return 32
	}
	return 64
}
