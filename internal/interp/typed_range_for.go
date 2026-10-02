package interp

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// execTypedRangeFor - "для Байт б от 0 до 255": переменная своего типа,
// конец проверяется после тела - как в cbackend (typed_range_for.go),
// диапазон до максимума типа не зацикливается.
func (in *Interp) execTypedRangeFor(s *ir.ForStmt, from, to Value, env *Env, cc *callCtx) (ctrl, error) {
	unsigned := isUnsignedFixedWidth(s.VarType.Kind) || s.VarType == ir.TypeByte
	if unsigned && uint64(from.I) > uint64(to.I) || !unsigned && from.I > to.I {
		return ctrl{}, nil
	}
	for i := from.I; ; i++ {
		loopEnv := newEnv(env)
		loopEnv.declare(s.Var, Value{Type: s.VarType, I: i})
		c, err := in.execBlock(s.Body, loopEnv, cc)
		if err != nil {
			return ctrl{}, err
		}
		if out, ok := handleLoopCtrl(c); !ok {
			return out, nil
		}
		if i == to.I {
			return ctrl{}, nil
		}
	}
}
