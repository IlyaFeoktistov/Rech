package interp

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// evalSetMethod - методы Множество<T>: хранилище - dictPairs, элемент -
// ключ (значение - истина), см. internal/semantic/set_type.go.
func (in *Interp) evalSetMethod(obj *entityObj, e *ir.MethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return Value{}, err
		}
		args[i] = v
	}
	switch e.Method {
	case "добавить":
		obj.coll.pairs.set(args[0], boolValue(true))
		return Value{}, nil
	case "содержит":
		return boolValue(obj.coll.pairs.has(args[0])), nil
	case "удалить":
		obj.coll.pairs.del(args[0])
		return Value{}, nil
	case "длина":
		return intValue(int64(obj.coll.pairs.size())), nil
	case "пусто":
		return boolValue(obj.coll.pairs.size() == 0), nil
	case "значения":
		return in.listResult(ir.TypeOf(e), obj.coll.pairs.keys), nil
	case "равен":
		return boolValue(in.collectionsDeepEqual(obj.coll, args[0].Obj.coll)), nil
	default:
		return Value{}, fmt.Errorf("встроенное множество %q: неизвестный метод %q", obj.typeName, e.Method)
	}
}
