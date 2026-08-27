package interp

import (
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// evalCastExpr - "(Тип) значение". Допустимость пары (исходный тип
// значения, e.To) уже проверена семантическим анализом (internal/semantic/
// cast.go, allowedCasts) - здесь только сама конвертация во время
// исполнения, тот же набор пар и то же поведение при ошибке разбора
// строки (сообщения - internal/rtmsg, байт-в-байт с cbackend, см.
// internal/backend/cbackend/cast.go).
func (in *Interp) evalCastExpr(e *ir.CastExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	from := v.Type
	to := e.To
	if from == to {
		return v, nil
	}
	switch to.Kind {
	case ir.KindInt:
		switch from.Kind {
		case ir.KindFloat, ir.KindFloat32:
			return intValue(int64(v.F)), nil
		case ir.KindChar, ir.KindByte:
			return intValue(v.I), nil
		case ir.KindString:
			n, err := strconv.ParseInt(v.S, 10, 64)
			if err != nil {
				panicRuntime("%s", rtmsg.CastStrToIntFailedFormat(to.String(), v.S))
			}
			return intValue(n), nil
		}
	case ir.KindByte:
		switch from.Kind {
		case ir.KindInt, ir.KindChar:
			return byteValue(v.I), nil
		case ir.KindString:
			n, err := strconv.ParseInt(v.S, 10, 64)
			if err != nil {
				panicRuntime("%s", rtmsg.CastStrToIntFailedFormat(to.String(), v.S))
			}
			return byteValue(n), nil
		}
	case ir.KindFloat32:
		switch from.Kind {
		case ir.KindInt, ir.KindByte:
			return float32Value(float64(v.I)), nil
		case ir.KindFloat:
			return float32Value(v.F), nil
		case ir.KindString:
			f, err := strconv.ParseFloat(v.S, 64)
			if err != nil {
				panicRuntime("%s", rtmsg.CastStrToFloatFailedFormat(to.String(), v.S))
			}
			return float32Value(f), nil
		}
	case ir.KindFloat:
		switch from.Kind {
		case ir.KindInt, ir.KindByte:
			return floatValue(float64(v.I)), nil
		case ir.KindFloat32:
			return floatValue(v.F), nil
		case ir.KindString:
			f, err := strconv.ParseFloat(v.S, 64)
			if err != nil {
				panicRuntime("%s", rtmsg.CastStrToFloatFailedFormat(to.String(), v.S))
			}
			return floatValue(f), nil
		}
	case ir.KindString:
		switch from.Kind {
		case ir.KindInt, ir.KindByte:
			return stringValue(strconv.FormatInt(v.I, 10)), nil
		case ir.KindFloat, ir.KindFloat32:
			return stringValue(formatFloatForPrint(v.F)), nil
		case ir.KindChar:
			return stringValue(string(rune(v.I))), nil
		}
	case ir.KindChar:
		switch from.Kind {
		case ir.KindInt, ir.KindByte:
			return charValue(rune(int32(v.I))), nil
		case ir.KindString:
			r, ok := singleRune(v.S)
			if !ok {
				panicRuntime("%s", rtmsg.CastStrToCharFailedFormat(v.S))
			}
			return charValue(r), nil
		}
	}
	return Value{}, fmt.Errorf("каст: неподдерживаемое приведение %s -> %s", from, to)
}

// singleRune декодирует s как ровно одну кодовую точку UTF-8 - ok == false,
// если s пустая, повреждена или состоит больше чем из одного символа (тот
// же критерий, что и у rech_cast_str_to_char в cbackend).
func singleRune(s string) (rune, bool) {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 || size != len(s) || r == utf8.RuneError && size == 1 {
		return 0, false
	}
	return r, true
}
