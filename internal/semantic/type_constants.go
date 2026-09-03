// Константы числовых примитивных типов ("Целое8::МАКСИМУМ"/"МИНИМУМ"/
// "ШИРИНА") - тот же синтаксис "Тип::имя", что уже есть у статических
// членов сущности (statics.go), но получатель - не объявленная
// сущность/структура, а один из встроенных числовых типов (primitiveTypes,
// types.go): Целое/Целое8/16/32/64, Натуральное/8/16/32/64, Дробное,
// Действительное. Перехватывается ДО staticReceiverEntity (statics.go) -
// у primitiveTypes нет записи в sc.entities, "Целое8" там дал бы обычную
// "сущность не объявлена".
package semantic

import (
	"fmt"
	"math"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// numericTypeConstantNames - единственные три признанных константы у
// числового примитива - остальные имена справа от "::" для такого типа
// считаются ошибкой (primitiveTypeConstant возвращает found=true с
// error != nil), а не "тип не объявлен".
const (
	typeConstMax   = "МАКСИМУМ"
	typeConstMin   = "МИНИМУМ"
	typeConstWidth = "ШИРИНА"
)

// primitiveTypeConstant - см. package doc. found=false означает "typeName -
// не числовой примитив вообще, это не про константы типа" (вызывающий код
// продолжает как раньше, считая typeName именем сущности/структуры).
// found=true всегда возвращает окончательный результат - либо выражение,
// либо содержательную ошибку про конкретную константу.
func primitiveTypeConstant(typeName, field string, line int) (expr ir.Expr, found bool, err error) {
	t, ok := primitiveTypes[typeName]
	if !ok {
		return nil, false, nil
	}
	if bits, signed, ok := t.Kind.IntWidthSigned(); ok {
		min, max := intWidthMinMax(bits, signed)
		switch field {
		case typeConstMax:
			return intConstAs(max, t), true, nil
		case typeConstMin:
			return intConstAs(min, t), true, nil
		case typeConstWidth:
			return &ir.IntLiteral{Value: int64(bits)}, true, nil
		default:
			return nil, true, unknownTypeConstError(typeName, field, line)
		}
	}
	if t == ir.TypeFloat || t == ir.TypeFloat32 {
		maxV, bits := math.MaxFloat64, 64
		if t == ir.TypeFloat32 {
			maxV, bits = math.MaxFloat32, 32
		}
		switch field {
		case typeConstMax:
			return floatConstAs(maxV, t), true, nil
		case typeConstMin:
			return floatConstAs(-maxV, t), true, nil
		case typeConstWidth:
			return &ir.IntLiteral{Value: int64(bits)}, true, nil
		default:
			return nil, true, unknownTypeConstError(typeName, field, line)
		}
	}
	// Примитив есть (Строка/Логическое/Символ/Байт/...), но у него нет
	// числового диапазона - "Строка::МАКСИМУМ" ничем не осмысленнее, чем
	// "Строка::что-то-ещё", тем же путём (found=false), что и просто
	// неизвестный тип - вызывающий код даст обычную ошибку про сущность.
	return nil, false, nil
}

func unknownTypeConstError(typeName, field string, line int) error {
	return fmt.Errorf("строка %d: у типа %q нет константы %q (доступны %s/%s/%s)",
		line, typeName, field, typeConstMax, typeConstMin, typeConstWidth)
}

// intWidthMinMax - минимальное/максимальное представимое значение
// целочисленного типа шириной bits (8/16/32/64), signed - как у
// ir.TypeKind.IntWidthSigned. Результат - int64 БИТОВЫЙ ПАТТЕРН (тот же
// принцип хранения, что и у Value.I в interp, internal/interp/widthints.go)
// - для беззнаковых 64 бит это означает max = -1 (все биты единицы,
// переинтерпретируется как 18446744073709551615 при чтении через
// uint64(...), см. isUnsignedFixedWidth).
func intWidthMinMax(bits int, signed bool) (min, max int64) {
	if !signed {
		if bits == 64 {
			return 0, -1
		}
		return 0, int64(1)<<uint(bits) - 1
	}
	if bits == 64 {
		return math.MinInt64, math.MaxInt64
	}
	max = int64(1)<<uint(bits-1) - 1
	return -(max + 1), max
}

// intConstAs - литерал v, типизированный как t: t == Целое (KindInt) уже
// совпадает с типом IntLiteral, приведение не нужно; любая фиксированная
// ширина - явный CastExpr, тем же способом, что и у пользовательского
// "(Целое8) выражение".
func intConstAs(v int64, t ir.Type) ir.Expr {
	lit := &ir.IntLiteral{Value: v}
	if t == ir.TypeInt {
		return lit
	}
	return &ir.CastExpr{Value: lit, To: t}
}

// floatConstAs - см. intConstAs, для Дробное/Действительное.
func floatConstAs(v float64, t ir.Type) ir.Expr {
	lit := &ir.FloatLiteral{Value: v}
	if t == ir.TypeFloat {
		return lit
	}
	return &ir.CastExpr{Value: lit, To: t}
}
