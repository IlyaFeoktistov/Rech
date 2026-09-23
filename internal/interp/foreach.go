package interp

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// execForEachCollection - "для [<ключ>,] <имя> в <коллекция>" по встроенной
// коллекции (не Файл) - порядок обхода тот же, что у cbackend
// (generateIRForEachStmt): Список/Очередь - от первого элемента, Стопка -
// сверху вниз (elems хранит вершину последней), Словарь - в порядке
// вставки (dictPairs; снимок ключей на входе в цикл - удаление внутри тела
// не сдвигает обход).
func (in *Interp) execForEachCollection(coll *collection, s *ir.ForEachStmt, env *Env, cc *callCtx) (ctrl, error) {
	var keys, vals []Value
	switch s.Kind {
	case ir.ForEachDict:
		keys = append(keys, coll.pairs.keys...)
		vals = append(vals, coll.pairs.vals...)
	case ir.ForEachStack:
		vals = reversedValues(coll.elems)
	default:
		vals = coll.elems
	}
	for i, val := range vals {
		loopEnv := newEnv(env)
		if s.KeyVar != "" {
			if s.Kind == ir.ForEachDict {
				loopEnv.declare(s.KeyVar, keys[i])
			} else {
				loopEnv.declare(s.KeyVar, intValue(int64(i)))
			}
		}
		loopEnv.declare(s.Var, in.cloneValue(val))
		c, err := in.execBlock(s.Body, loopEnv, cc)
		if err != nil {
			return ctrl{}, err
		}
		if out, ok := handleLoopCtrl(c); !ok {
			return out, nil
		}
	}
	return ctrl{}, nil
}

// listResult - новый Список типа typ ("ключи()"/"значения()") из копий
// elems в том же порядке.
func (in *Interp) listResult(typ ir.Type, elems []Value) Value {
	out := make([]Value, len(elems))
	for i, v := range elems {
		out[i] = in.cloneValue(v)
	}
	return Value{Type: typ, Obj: &entityObj{typeName: typ.Entity, coll: &collection{template: "Список", elems: out}}}
}

// reversedValues - elems в обратном порядке (Стопка: вершина - первой).
func reversedValues(elems []Value) []Value {
	out := make([]Value, len(elems))
	for i, v := range elems {
		out[len(elems)-1-i] = v
	}
	return out
}
