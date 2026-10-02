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
	if e.AsPrinted {
		return Value{Type: ir.TypeString, S: in.printText(v)}, nil
	}
	if from.Kind == ir.KindPointer || to.Kind == ir.KindPointer {
		return in.castPointer(v, to), nil
	}
	if to.Kind == ir.KindAny {
		// Боксирование - клонирование как при "коммите" в новое хранилище
		// (та же семантика, что переменная/присваивание/параметр, см.
		// cloneValue) - сущность/интерфейс копируется по значению, если
		// это структура, иначе делит тот же *entityObj, что и оригинал;
		// после этого момента боксированное значение живёт независимо от
		// v (дальнейшая мутация v, если это структура, не видна через
		// коробку, и наоборот).
		if v.Type.Kind == ir.KindNullable {
			// "(Значение) ?Тип": Ничто - пустое Значение, иначе содержимое.
			if v.Nullable == nil {
				return Value{Type: ir.TypeAny, Boxed: &Value{Type: ir.TypeNullValue}}, nil
			}
			v = *v.Nullable
		}
		boxed := in.cloneValue(v)
		return Value{Type: ir.TypeAny, Boxed: &boxed}, nil
	}
	if isFixedWidthInt(to.Kind) {
		switch from.Kind {
		case ir.KindFloat, ir.KindFloat32:
			return Value{Type: to, I: wrapFixedWidth(to.Kind, int64(v.F))}, nil
		case ir.KindString:
			if isUnsignedFixedWidth(to.Kind) {
				n, err := strconv.ParseUint(v.S, 10, 64)
				if err != nil {
					throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.CastStrToIntFailedFormat(to.String(), v.S))
				}
				return Value{Type: to, I: wrapFixedWidth(to.Kind, int64(n))}, nil
			}
			n, err := strconv.ParseInt(v.S, 10, 64)
			if err != nil {
				throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.CastStrToIntFailedFormat(to.String(), v.S))
			}
			return Value{Type: to, I: wrapFixedWidth(to.Kind, n)}, nil
		case ir.KindInt, ir.KindByte, ir.KindChar,
			ir.KindInt8, ir.KindInt16, ir.KindInt32,
			ir.KindUint, ir.KindUint16, ir.KindUint32:
			// Битовый паттерн v.I уже несёт корректно знако-/нуле-
			// расширенное 64-битное представление исходного значения
			// (см. wrapFixedWidth) - остаётся только усечь его до
			// ширины целевого типа.
			return Value{Type: to, I: wrapFixedWidth(to.Kind, v.I)}, nil
		}
		return Value{}, fmt.Errorf("каст: неподдерживаемое приведение %s -> %s", from, to)
	}

	switch to.Kind {
	case ir.KindInt:
		switch {
		case from.Kind == ir.KindFloat || from.Kind == ir.KindFloat32:
			return intValue(int64(v.F)), nil
		case from.Kind == ir.KindChar || from.Kind == ir.KindByte || isFixedWidthInt(from.Kind):
			return intValue(v.I), nil
		case from.Kind == ir.KindString:
			n, err := strconv.ParseInt(v.S, 10, 64)
			if err != nil {
				throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.CastStrToIntFailedFormat(to.String(), v.S))
			}
			return intValue(n), nil
		}
	case ir.KindByte:
		switch {
		case from.Kind == ir.KindInt || from.Kind == ir.KindChar || isFixedWidthInt(from.Kind):
			return byteValue(v.I), nil
		case from.Kind == ir.KindString:
			n, err := strconv.ParseInt(v.S, 10, 64)
			if err != nil {
				throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.CastStrToIntFailedFormat(to.String(), v.S))
			}
			return byteValue(n), nil
		}
	case ir.KindFloat32:
		switch {
		case from.Kind == ir.KindInt || from.Kind == ir.KindByte || isFixedWidthInt(from.Kind):
			return float32Value(fixedWidthToFloat(from.Kind, v.I)), nil
		case from.Kind == ir.KindFloat:
			return float32Value(v.F), nil
		case from.Kind == ir.KindString:
			f, err := strconv.ParseFloat(v.S, 64)
			if err != nil {
				throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.CastStrToFloatFailedFormat(to.String(), v.S))
			}
			return float32Value(f), nil
		}
	case ir.KindFloat:
		switch {
		case from.Kind == ir.KindInt || from.Kind == ir.KindByte || isFixedWidthInt(from.Kind):
			return floatValue(fixedWidthToFloat(from.Kind, v.I)), nil
		case from.Kind == ir.KindFloat32:
			return floatValue(v.F), nil
		case from.Kind == ir.KindString:
			f, err := strconv.ParseFloat(v.S, 64)
			if err != nil {
				throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.CastStrToFloatFailedFormat(to.String(), v.S))
			}
			return floatValue(f), nil
		}
	case ir.KindString:
		switch {
		case isFixedWidthInt(from.Kind) && isUnsignedFixedWidth(from.Kind):
			return stringValue(strconv.FormatUint(uint64(v.I), 10)), nil
		case from.Kind == ir.KindInt || from.Kind == ir.KindByte || isFixedWidthInt(from.Kind):
			return stringValue(strconv.FormatInt(v.I, 10)), nil
		case from.Kind == ir.KindFloat || from.Kind == ir.KindFloat32:
			return stringValue(formatFloat(v)), nil
		case from.Kind == ir.KindChar:
			return stringValue(string(rune(v.I))), nil
		case from.Kind == ir.KindBool:
			if v.B {
				return stringValue("истина"), nil
			}
			return stringValue("ложь"), nil
		}
	case ir.KindChar:
		switch {
		case from.Kind == ir.KindInt || from.Kind == ir.KindByte || isFixedWidthInt(from.Kind):
			return charValue(rune(int32(v.I))), nil
		case from.Kind == ir.KindString:
			r, ok := singleRune(v.S)
			if !ok {
				throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.CastStrToCharFailedFormat(v.S))
			}
			return charValue(r), nil
		}
	case ir.KindBool:
		if from.Kind == ir.KindString {
			// Только буквальные "истина"/"ложь" (тот же текст, что и вывод
			// "вывести"/"показать" для Логическое) - симметричный round-trip,
			// не парсер произвольных "правдоподобных" слов ("true"/"1" и
			// т. п.).
			switch v.S {
			case "истина":
				return boolValue(true), nil
			case "ложь":
				return boolValue(false), nil
			}
			throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.CastStrToBoolFailedFormat(v.S))
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
