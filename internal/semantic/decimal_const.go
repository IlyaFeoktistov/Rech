package semantic

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/decimal"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/numfmt"
)

// constDecimalValue - числовая константа e (литералы, "-", + - * /),
// вычисленная точно в Десятичное: "Десятичное ц = 0.1 + 0.2" - ровно 0.3,
// дробный литерал берётся из текста исходника, не из double.
func constDecimalValue(e ir.Expr) (decimal.Decimal, bool, error) {
	if ir.TypeOf(e) == ir.TypeInt {
		// Целое выражение - целая арифметика ("1 / 3" - 0), как и без
		// Десятичное.
		v, ok := constIntValue(e)
		return decimal.FromInt(v), ok, nil
	}
	switch v := e.(type) {
	case *ir.FloatLiteral:
		text := strings.ReplaceAll(v.Text, "_", "")
		if text == "" {
			text = numfmt.Float(v.Value, 64)
		}
		d, err := decimal.Parse(text)
		return d, err == nil, err
	case *ir.UnaryExpr:
		if v.Op != "-" {
			return decimal.Decimal{}, false, nil
		}
		d, ok, err := constDecimalValue(v.Operand)
		return decimal.Neg(d), ok, err
	case *ir.BinaryExpr:
		op, known := decimalConstOps[v.Op]
		if !known {
			return decimal.Decimal{}, false, nil
		}
		l, lok, err := constDecimalValue(v.Left)
		if !lok || err != nil {
			return l, false, err
		}
		r, rok, err := constDecimalValue(v.Right)
		if !rok || err != nil {
			return r, false, err
		}
		d, err := op(l, r)
		return d, err == nil, err
	}
	return decimal.Decimal{}, false, nil
}

var decimalConstOps = map[string]func(a, b decimal.Decimal) (decimal.Decimal, error){
	"+": decimal.Add,
	"-": decimal.Sub,
	"*": decimal.Mul,
	"/": decimal.Div,
	"%": decimal.Rem,
}
