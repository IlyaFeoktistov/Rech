package interp

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// evalListIndexExpr - "список[i]" (безопасная индексация, см. package doc
// ir.ListIndexExpr) - "?T" вместо паники при выходе за границу (в отличие
// от MethodCallExpr{Method: "получить"}, evalMethodCallExpr).
func (in *Interp) evalListIndexExpr(e *ir.ListIndexExpr, env *Env, cc *callCtx) (Value, error) {
	recv, err := in.evalExpr(e.Receiver, env, cc)
	if err != nil {
		return Value{}, err
	}
	idx, err := in.evalExpr(e.Index, env, cc)
	if err != nil {
		return Value{}, err
	}
	elems := recv.Obj.coll.elems
	i := idx.I
	if i < 0 || i >= int64(len(elems)) {
		return Value{Type: e.Type}, nil
	}
	wrapped := in.cloneValue(elems[i])
	return Value{Type: e.Type, Nullable: &wrapped}, nil
}

// evalDictIndexExpr - "словарь[ключ]" - тот же принцип, что
// evalListIndexExpr, но по ключу - "?В", Ничто при отсутствии ключа (не
// паника, в отличие от MethodCallExpr{Method: "получить"}).
func (in *Interp) evalDictIndexExpr(e *ir.DictIndexExpr, env *Env, cc *callCtx) (Value, error) {
	recv, err := in.evalExpr(e.Receiver, env, cc)
	if err != nil {
		return Value{}, err
	}
	key, err := in.evalExpr(e.Key, env, cc)
	if err != nil {
		return Value{}, err
	}
	v, ok := recv.Obj.coll.pairs[key]
	if !ok {
		return Value{Type: e.Type}, nil
	}
	wrapped := in.cloneValue(v)
	return Value{Type: e.Type, Nullable: &wrapped}, nil
}
