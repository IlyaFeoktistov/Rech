package semantic

import (
	"strconv"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// Длина массива константой модуля - "Байт[РАЗМЕР]" при "константа РАЗМЕР
// = 65", как "#define" в C-заголовке. Значение - целая константа: литерал
// или выражение из литералов, операторов и других таких констант.

// arrayLenConsts - целые значения констант модуля текущего анализа (как
// currentFile - на время одного Analyze).
var arrayLenConsts map[string]int64

// collectArrayLenConsts - значения тех констант prog, что вычисляются в
// целое число; остальные просто не попадают в таблицу.
func collectArrayLenConsts(prog *ast.Program) {
	exprs := make(map[string]ast.Expr, len(prog.Consts))
	for _, c := range prog.Consts {
		exprs[c.Name] = c.Value
	}
	arrayLenConsts = make(map[string]int64, len(exprs))
	visiting := map[string]bool{}
	var value func(name string) (int64, bool)
	var eval func(e ast.Expr) (int64, bool)
	value = func(name string) (int64, bool) {
		if v, ok := arrayLenConsts[name]; ok {
			return v, true
		}
		e, ok := exprs[name]
		if !ok || visiting[name] {
			return 0, false
		}
		visiting[name] = true
		v, ok := eval(e)
		visiting[name] = false
		if ok {
			arrayLenConsts[name] = v
		}
		return v, ok
	}
	eval = func(e ast.Expr) (int64, bool) {
		switch v := e.(type) {
		case *ast.IntLiteral:
			return v.Value, true
		case *ast.Identifier:
			return value(v.Name)
		case *ast.UnaryExpr:
			x, ok := eval(v.Operand)
			if !ok {
				return 0, false
			}
			switch v.Op {
			case "-":
				return -x, true
			case "~":
				return ^x, true
			}
		case *ast.BinaryExpr:
			l, lok := eval(v.Left)
			r, rok := eval(v.Right)
			if !lok || !rok {
				return 0, false
			}
			switch v.Op {
			case "+":
				return l + r, true
			case "-":
				return l - r, true
			case "*":
				return l * r, true
			case "/", "%":
				if r == 0 {
					return 0, false
				}
				if v.Op == "/" {
					return l / r, true
				}
				return l % r, true
			case "<<":
				return l << uint64(r), true
			case ">>":
				return l >> uint64(r), true
			case "&":
				return l & r, true
			case "|":
				return l | r, true
			case "^":
				return l ^ r, true
			}
		}
		return 0, false
	}
	for name := range exprs {
		value(name)
	}
}

// arrayLen - длина массива size, как она написана в типе name: число или
// имя целой константы модуля.
func arrayLen(name, size string) (int64, error) {
	if n, err := strconv.ParseInt(size, 10, 64); err == nil {
		return n, nil
	}
	if n, ok := arrayLenConsts[size]; ok {
		return n, nil
	}
	return 0, errNoPos(CodeArray, "%q: длина массива %q - не целая константа модуля (нужно \"константа %s = <целое число>\")", name, size, size)
}
