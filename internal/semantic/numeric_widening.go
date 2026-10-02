package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Операнды разных числовых типов (арифметика, сравнение, & | ^) приводятся
// к общему типу, как usual arithmetic conversions в C, но без ловушки
// "знаковое молча становится беззнаковым" (в C "-1 < 1u" - ложь):
//   - есть дробный операнд - самый широкий из дробных (Дробное ->
//     Действительное), целый приводится к нему;
//   - оба целые одной знаковости - более широкий;
//   - знаковое и беззнаковое - знаковый тип, вмещающий оба ("Целое32 +
//     Натуральное32" -> Целое64); "Целое64 + Натуральное64" такого типа не
//     имеет - ошибка, нужен явный каст.
// Символ в этом не участвует (код символа, не число).

// intTypeByWidth - целый тип по ширине и знаковости.
var intTypeByWidth = map[[2]int]ir.Type{
	{8, 1}: ir.TypeInt8, {16, 1}: ir.TypeInt16, {32, 1}: ir.TypeInt32, {64, 1}: ir.TypeInt,
	{8, 0}: ir.TypeByte, {16, 0}: ir.TypeUint16, {32, 0}: ir.TypeUint32, {64, 0}: ir.TypeUint,
}

func intType(bits int, signed bool) ir.Type {
	s := 0
	if signed {
		s = 1
	}
	return intTypeByWidth[[2]int{bits, s}]
}

// commonNumericType - общий тип чисел a и b; ok=false - a или b не число
// (или Символ); err - общего типа нет.
func commonNumericType(a, b ir.Type) (ir.Type, bool, error) {
	if a == ir.TypeDecimal || b == ir.TypeDecimal {
		return commonDecimalType(a, b)
	}
	af, aFloat := numericRanks[a]
	bf, bFloat := numericRanks[b]
	aFloat = aFloat && a != ir.TypeInt
	bFloat = bFloat && b != ir.TypeInt
	ab, as, aInt := a.Kind.IntWidthSigned()
	bb, bs, bInt := b.Kind.IntWidthSigned()
	if !(aFloat || aInt) || !(bFloat || bInt) {
		return ir.TypeUnknown, false, nil
	}
	switch {
	case aFloat && bFloat:
		if af >= bf {
			return a, true, nil
		}
		return b, true, nil
	case aFloat:
		return a, true, nil
	case bFloat:
		return b, true, nil
	case as == bs:
		return intType(max(ab, bb), as), true, nil
	}
	sBits, uBits := ab, bb
	if !as {
		sBits, uBits = bb, ab
	}
	if sBits > uBits {
		return intType(sBits, true), true, nil
	}
	if uBits < 64 {
		return intType(uBits*2, true), true, nil
	}
	return ir.TypeUnknown, true, fmt.Errorf("нет знакового типа, вмещающего и %s, и %s - приведите один операнд явно, например (Целое) значение", a, b)
}

// commonDecimalType - общий тип пары с Десятичное: с целым - Десятичное
// (целое в нём точно); с двоичным дробным неявно нельзя (как в C#) - 0.1
// в Действительное уже не 0.1, молча смешать значило бы потерять
// точность.
func commonDecimalType(a, b ir.Type) (ir.Type, bool, error) {
	other := a
	if a == ir.TypeDecimal {
		other = b
	}
	if other == ir.TypeDecimal {
		return ir.TypeDecimal, true, nil
	}
	if _, _, isInt := other.Kind.IntWidthSigned(); isInt {
		return ir.TypeDecimal, true, nil
	}
	if other == ir.TypeFloat || other == ir.TypeFloat32 {
		return ir.TypeUnknown, true, fmt.Errorf("Десятичное и %s неявно не смешиваются - приведите явно: (Десятичное) x или (%s) д", other, other)
	}
	return ir.TypeUnknown, false, nil
}

// widenOperands - операнды бинарного op, приведённые к общему типу (см.
// package doc). Сдвиг "<<"/">>" - тип результата у левого операнда,
// правый (число позиций) приводится к нему.
func widenOperands(op string, left, right ir.Expr, line int) (ir.Expr, ir.Expr, error) {
	lt, rt := ir.TypeOf(left), ir.TypeOf(right)
	if lt == rt {
		return left, right, nil
	}
	switch op {
	case "+", "-", "*", "/", "%", "==", "!=", "<", ">", "<=", ">=", "&", "|", "^":
	case "<<", ">>":
		_, _, lInt := lt.Kind.IntWidthSigned()
		_, _, rInt := rt.Kind.IntWidthSigned()
		if lInt && rInt {
			return left, &ir.CastExpr{Value: right, To: lt}, nil
		}
		return left, right, nil
	default:
		return left, right, nil
	}
	common, ok, err := commonNumericType(lt, rt)
	if !ok {
		return left, right, nil
	}
	if err != nil {
		return nil, nil, errAtLine(line, CodeExprCheck, "оператор %q: %v", op, err)
	}
	if lt != common {
		left = &ir.CastExpr{Value: left, To: common}
	}
	if rt != common {
		right = &ir.CastExpr{Value: right, To: common}
	}
	return left, right, nil
}
