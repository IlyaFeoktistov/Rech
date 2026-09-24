package interp

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// evalListLiteralExpr - "[элемент, ...]" (см. package doc
// ir.ListLiteralExpr) - тот же принцип представления, что и у уже
// существующих встроенных построителей списка из данных (internal/interp/
// json.go, internal/interp/xml.go).
func (in *Interp) evalListLiteralExpr(e *ir.ListLiteralExpr, env *Env, cc *callCtx) (Value, error) {
	elems := make([]Value, 0, len(e.Elems))
	for i, el := range e.Elems {
		v, err := in.evalExpr(el, env, cc)
		if err != nil {
			return Value{}, err
		}
		if e.Spread != nil && e.Spread[i] {
			for _, item := range v.Obj.coll.elems {
				elems = append(elems, in.cloneValue(item))
			}
			continue
		}
		elems = append(elems, in.cloneValue(v))
	}
	return Value{Type: e.Type, Obj: &entityObj{typeName: e.CollectionName, coll: &collection{template: "Список", elems: elems}}}, nil
}
