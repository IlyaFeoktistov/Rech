package interp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// Массив фиксированной длины "T[N]" - Value с Obj.coll (элементы, ровно
// N). Значение: cloneValue копирует элементы (присваивание/передача/
// возврат), навигация ("м[i]", "м.поле[i]") - нет, поэтому запись "м[i] =
// x" меняет то же хранилище, что держит переменная/поле.

// arrayTemplate - collection.template у массива.
const arrayTemplate = "Массив"

// zeroArray - массив t из нулевых значений элемента: числа - 0, Строка -
// "", указатель - Ничто, структура - все поля нулевые (умолчания полей не
// применяются, как и у нулей в C).
func (in *Interp) zeroArray(t ir.Type) Value {
	a := in.arrays[t.Entity]
	elems := make([]Value, a.Len)
	for i := range elems {
		elems[i] = in.zeroElem(a.Elem)
	}
	return arrayValue(t, elems)
}

func (in *Interp) zeroElem(t ir.Type) Value {
	if t.Kind == ir.KindArray {
		return in.zeroArray(t)
	}
	if ent, ok := in.entities[t.Entity]; ok && t.Kind == ir.KindEntity && ent.IsStruct {
		obj := &entityObj{typeName: t.Entity, fields: make(map[string]Value, len(ent.Fields))}
		for _, f := range ent.Fields {
			obj.setField(f.Name, in.zeroElem(f.Type))
		}
		return Value{Type: t, Obj: obj}
	}
	return zeroValue(t)
}

func arrayValue(t ir.Type, elems []Value) Value {
	return Value{Type: t, Obj: &entityObj{typeName: t.Entity, coll: &collection{template: arrayTemplate, elems: elems}}}
}

// cloneArray - копия массива v (элементы - cloneValue, вложенные массивы и
// структуры тоже копируются).
func (in *Interp) cloneArray(v Value) Value {
	src := v.Obj.coll.elems
	elems := make([]Value, len(src))
	for i, el := range src {
		elems[i] = in.cloneValue(el)
	}
	return arrayValue(v.Type, elems)
}

// arrayElemIndex - индекс idx у массива arr, проверенный по границам.
func arrayElemIndex(arr Value, idx int64) int64 {
	if idx < 0 || idx >= int64(len(arr.Obj.coll.elems)) {
		panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.IndexOutOfRange(arr.Type.String()))
	}
	return idx
}

func (in *Interp) evalArrayIndexExpr(e *ir.ArrayIndexExpr, env *Env, cc *callCtx) (Value, error) {
	arr, err := in.evalExpr(e.Array, env, cc)
	if err != nil {
		return Value{}, err
	}
	idx, err := in.evalExpr(e.Index, env, cc)
	if err != nil {
		return Value{}, err
	}
	return arr.Obj.coll.elems[arrayElemIndex(arr, idx.I)], nil
}

func (in *Interp) evalArrayLiteralExpr(e *ir.ArrayLiteralExpr, env *Env, cc *callCtx) (Value, error) {
	if e.Elems == nil {
		return in.zeroArray(e.Type), nil
	}
	elems := make([]Value, len(e.Elems))
	for i, el := range e.Elems {
		v, err := in.evalExpr(el, env, cc)
		if err != nil {
			return Value{}, err
		}
		elems[i] = in.cloneValue(v)
	}
	return arrayValue(e.Type, elems), nil
}

func (in *Interp) execArrayAssignStmt(s *ir.ArrayAssignStmt, env *Env, cc *callCtx) (ctrl, error) {
	arr, err := in.evalExpr(s.Array, env, cc)
	if err != nil {
		return ctrl{}, err
	}
	idx, err := in.evalExpr(s.Index, env, cc)
	if err != nil {
		return ctrl{}, err
	}
	value, err := in.evalExpr(s.Value, env, cc)
	if err != nil {
		return ctrl{}, err
	}
	arr.Obj.coll.elems[arrayElemIndex(arr, idx.I)] = in.cloneValue(value)
	return ctrl{}, nil
}

// arrayElemTarget - "&м[i]": адрес элемента - тот же вид pointerTarget,
// что у элемента списка (соседние элементы - "п[1]").
func (in *Interp) arrayElemTarget(e *ir.ArrayIndexExpr, env *Env, cc *callCtx) (*pointerTarget, error) {
	arr, err := in.evalExpr(e.Array, env, cc)
	if err != nil {
		return nil, err
	}
	idx, err := in.evalExpr(e.Index, env, cc)
	if err != nil {
		return nil, err
	}
	return &pointerTarget{coll: arr.Obj.coll, index: arrayElemIndex(arr, idx.I), collName: arr.Type.String()}, nil
}
