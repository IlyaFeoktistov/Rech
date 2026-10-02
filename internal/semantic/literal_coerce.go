// Приведение числовой константы к ожидаемому типу: "Целое8 а = 0",
// "Байт б = 255", "Дробное д = 1.5", "а + 1" при а: Целое8. Константа -
// литерал или выражение из литералов и операторов (-, ~, + - * / % & | ^
// << >>); её значение обязано укладываться в диапазон целевого типа, иначе
// ошибка компиляции. Значение переменной по-прежнему требует явного
// "(Тип) выражение" - неявно приводится только то, что известно при
// компиляции.
package semantic

import (
	"math"
	"strconv"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// coerceNumericConst - value, приведённое к target, если value - числовая
// константа типа Целое/Действительное, а target - другой числовой тип
// (Целое8..64, Натуральное.., Байт, Дробное). ok=false - правило
// неприменимо (value не константа или target не числовой) - value
// возвращается как есть. err != nil - константа не укладывается в target.
func coerceNumericConst(value ir.Expr, target ir.Type, line int) (ir.Expr, bool, error) {
	typ := ir.TypeOf(value)
	if typ == target {
		return value, false, nil
	}
	if target == ir.TypeDecimal && (typ == ir.TypeInt || typ == ir.TypeFloat) {
		d, ok, err := constDecimalValue(value)
		if err != nil {
			return nil, true, errAtLine(line, CodeExprCheck, "константа Десятичное: %v", err)
		}
		if ok {
			return &ir.DecimalLiteral{Value: d}, true, nil
		}
		return value, false, nil
	}
	if typ == ir.TypeInt {
		v, ok := constIntValue(value)
		if !ok {
			return value, false, nil
		}
		if target == ir.TypeFloat || target == ir.TypeFloat32 {
			// "д < 2", "Дробное д = 2" - как в C.
			return floatConstAs(float64(v), target), true, nil
		}
		min, max, ok := intTargetRange(target)
		if !ok {
			return value, false, nil
		}
		if !intFits(v, min, max, target) {
			return nil, true, errAtLine(line, CodeExprCheck, "значение %d не помещается в тип %s (от %s до %s)", v, target, formatIntBound(min, target), formatIntBound(max, target))
		}
		return intConstAs(v, target), true, nil
	}
	if typ == ir.TypeFloat && target == ir.TypeFloat32 {
		v, ok := constFloatValue(value)
		if !ok {
			return value, false, nil
		}
		if !math.IsInf(v, 0) && !math.IsNaN(v) && math.Abs(v) > math.MaxFloat32 {
			return nil, true, errAtLine(line, CodeExprCheck, "значение %g не помещается в тип %s", v, target)
		}
		return floatConstAs(v, target), true, nil
	}
	return value, false, nil
}

// coerceNumericConstPair - операнды бинарного оператора: константа
// типа Целое/Действительное рядом с операндом другого числового типа
// принимает его тип ("а + 1", "б > 0"). Две константы или два неконстантных
// операнда не трогаются.
func coerceNumericConstPair(left, right ir.Expr, line int) (ir.Expr, ir.Expr, error) {
	lt, rt := ir.TypeOf(left), ir.TypeOf(right)
	if lt == rt {
		return left, right, nil
	}
	if coerced, ok, err := coerceNumericConst(right, lt, line); ok || err != nil {
		if err != nil {
			return nil, nil, err
		}
		return left, coerced, nil
	}
	if coerced, ok, err := coerceNumericConst(left, rt, line); ok || err != nil {
		if err != nil {
			return nil, nil, err
		}
		return coerced, right, nil
	}
	return left, right, nil
}

// intTargetRange - диапазон целочисленного типа t, кроме самого Целое
// (у него приводить нечего). Байт - отдельно: не входит в IntWidthSigned.
func intTargetRange(t ir.Type) (min, max int64, ok bool) {
	if t == ir.TypeByte {
		return 0, 255, true
	}
	if t == ir.TypeInt {
		return 0, 0, false
	}
	bits, signed, ok := t.Kind.IntWidthSigned()
	if !ok {
		return 0, 0, false
	}
	min, max = intWidthMinMax(bits, signed)
	return min, max, true
}

// intFits - v в [min, max]. У 64-битных беззнаковых max хранится битовым
// паттерном -1 (intWidthMinMax), а Целое-константа не превышает
// math.MaxInt64 - достаточно v >= 0.
func intFits(v, min, max int64, t ir.Type) bool {
	if bits, signed, ok := t.Kind.IntWidthSigned(); ok && !signed && bits == 64 {
		return v >= 0
	}
	return v >= min && v <= max
}

func formatIntBound(v int64, t ir.Type) string {
	if bits, signed, ok := t.Kind.IntWidthSigned(); ok && !signed && bits == 64 {
		return strconv.FormatUint(uint64(v), 10)
	}
	return strconv.FormatInt(v, 10)
}

// constIntValue - значение Целое-выражения, собранного только из
// литералов и операторов. ok=false - в выражении есть что-то, известное
// лишь при выполнении, или деление на ноль (его покажет сама программа).
func constIntValue(e ir.Expr) (int64, bool) {
	switch e := e.(type) {
	case *ir.IntLiteral:
		return e.Value, true
	case *ir.UnaryExpr:
		if e.Type != ir.TypeInt {
			return 0, false
		}
		v, ok := constIntValue(e.Operand)
		if !ok {
			return 0, false
		}
		switch e.Op {
		case "-":
			return -v, true
		case "~":
			return ^v, true
		}
	case *ir.BinaryExpr:
		if e.Type != ir.TypeInt {
			return 0, false
		}
		l, ok := constIntValue(e.Left)
		if !ok {
			return 0, false
		}
		r, ok := constIntValue(e.Right)
		if !ok {
			return 0, false
		}
		if fold, ok := constIntOps[e.Op]; ok {
			return fold(l, r)
		}
	}
	return 0, false
}

var constIntOps = map[string]func(l, r int64) (int64, bool){
	"+": func(l, r int64) (int64, bool) { return l + r, true },
	"-": func(l, r int64) (int64, bool) { return l - r, true },
	"*": func(l, r int64) (int64, bool) { return l * r, true },
	"/": func(l, r int64) (int64, bool) { return l / r, r != 0 },
	"%": func(l, r int64) (int64, bool) { return l % r, r != 0 },
	"&": func(l, r int64) (int64, bool) { return l & r, true },
	"|": func(l, r int64) (int64, bool) { return l | r, true },
	"^": func(l, r int64) (int64, bool) { return l ^ r, true },
	"<<": func(l, r int64) (int64, bool) {
		return l << uint(r), r >= 0 && r < 64
	},
	">>": func(l, r int64) (int64, bool) {
		return l >> uint(r), r >= 0 && r < 64
	},
}

// constFloatValue - литерал Действительное, возможно с унарным минусом.
func constFloatValue(e ir.Expr) (float64, bool) {
	switch e := e.(type) {
	case *ir.FloatLiteral:
		return e.Value, true
	case *ir.UnaryExpr:
		if e.Op != "-" || e.Type != ir.TypeFloat {
			return 0, false
		}
		v, ok := constFloatValue(e.Operand)
		return -v, ok
	}
	return 0, false
}
