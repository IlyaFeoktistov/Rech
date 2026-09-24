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
	return in.safeIndexResult(e.Type, elems[i]), nil
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
	v, ok := recv.Obj.coll.pairs.get(key)
	if !ok {
		return Value{Type: e.Type}, nil
	}
	return in.safeIndexResult(e.Type, v), nil
}

// safeIndexResult - найденный элемент как результат "[]" типа typ ("?T"):
// обёрнутый, либо как есть, если элемент коллекции сам уже "?T".
func (in *Interp) safeIndexResult(typ ir.Type, elem Value) Value {
	wrapped := in.cloneValue(elem)
	if elem.Type.Kind == ir.KindNullable {
		wrapped.Type = typ
		return wrapped
	}
	return Value{Type: typ, Nullable: &wrapped}
}
