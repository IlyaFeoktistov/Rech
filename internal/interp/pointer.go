// "&получатель"/"*значение" - "Указатель<T>" (docs/SPEC.md, "Указатели") -
// адрес переменной/параметра, поля сущности или элемента списка.
//
// В отличие от cbackend (где адрес - буквально C-указатель "T *" на
// настоящую память), интерпретатор не может взять устойчивый Go-указатель
// на слот map (map-значения в Go неадресуемы, `&m[k]` не компилируется) -
// вместо этого pointerTarget - маленький дескриптор "где искать значение
// заново при каждом чтении/записи" (Env+имя / entityObj+поле /
// collection+индекс, ровно одно занято). Цена та же, что у обычного
// чтения/записи переменной/поля/элемента - не дороже.
//
// Из этого следует одно ЯВНОЕ расхождение с cbackend, документированное,
// не баг: адрес элемента списка в cbackend - настоящий указатель на ячейку
// growable-буфера, теряющий силу при следующем "добавить" в тот же список,
// реаллоцирующем буфер (классическое разыменование "протухшего" указателя,
// неопределённое поведение, как и в C); тот же адрес в интерпретаторе
// хранит индекс и указатель на сам *collection (не на буфер), поэтому
// продолжает резолвиться в СЕМАНТИЧЕСКИ верный элемент даже после
// реаллокации Go-слайса под капотом. Оба поведения - в рамках
// задокументированного "программист сам отвечает за адрес элемента списка,
// пережившего изменение списка" - интерпретатор просто более снисходителен
// в этом конкретном случае, а не давать одинаковый result требуется только
// там, где поведение вообще определено.
package interp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// pointerTarget - см. package doc.
type pointerTarget struct {
	env  *Env   // !=nil - переменная/параметр (env+name)
	name string

	obj   *entityObj // !=nil - поле сущности (obj+field)
	field string

	coll     *collection // !=nil - элемент списка (coll+index)
	index    int64
	collName string // имя инстанциации, только для сообщения об ошибке
}

func (p *pointerTarget) get() Value {
	switch {
	case p.env != nil:
		v, _ := p.env.get(p.name)
		return v
	case p.obj != nil:
		return p.obj.fields[p.field]
	default:
		if p.index < 0 || p.index >= int64(len(p.coll.elems)) {
			panicRuntime("%s", rtmsg.IndexOutOfRange(p.collName))
		}
		return p.coll.elems[p.index]
	}
}

func (p *pointerTarget) set(v Value) {
	switch {
	case p.env != nil:
		p.env.set(p.name, v)
	case p.obj != nil:
		p.obj.fields[p.field] = v
	default:
		if p.index < 0 || p.index >= int64(len(p.coll.elems)) {
			panicRuntime("%s", rtmsg.IndexOutOfRange(p.collName))
		}
		p.coll.elems[p.index] = v
	}
}

// pointerValue боксирует target в "Значение" типа "Указатель<T>" - t уже
// содержит каноническое имя ("Указатель<Целое>", резолвлено семантическим
// анализом).
func pointerValue(t ir.Type, target *pointerTarget) Value {
	return Value{Type: t, Ptr: target}
}

// evalAddressOfExpr - "&получатель.поле"/"&получатель[индекс]" (простое
// "&имя" - переменная или ссылка на функцию - см. evalFuncRefExpr,
// callable.go, для этого узла не строится).
func (in *Interp) evalAddressOfExpr(e *ir.AddressOfExpr, env *Env, cc *callCtx) (Value, error) {
	switch recv := e.Receiver.(type) {
	case *ir.Identifier:
		return pointerValue(e.Type, &pointerTarget{env: env, name: recv.Name}), nil
	case *ir.FieldAccess:
		obj, err := in.evalExpr(recv.Receiver, env, cc)
		if err != nil {
			return Value{}, err
		}
		return pointerValue(e.Type, &pointerTarget{obj: obj.Obj, field: recv.Field}), nil
	case *ir.ListElementRef:
		list, err := in.evalExpr(recv.Receiver, env, cc)
		if err != nil {
			return Value{}, err
		}
		idx, err := in.evalExpr(recv.Index, env, cc)
		if err != nil {
			return Value{}, err
		}
		coll := list.Obj.coll
		if idx.I < 0 || idx.I >= int64(len(coll.elems)) {
			panicRuntime("%s", rtmsg.IndexOutOfRange(recv.CollectionName))
		}
		return pointerValue(e.Type, &pointerTarget{coll: coll, index: idx.I, collName: recv.CollectionName}), nil
	default:
		panicRuntime("внутренняя ошибка: неподдержанный получатель \"&\" %T", e.Receiver)
		return Value{}, nil
	}
}

// evalDerefExpr - "*значение" - чтение через "Указатель<T>".
func (in *Interp) evalDerefExpr(e *ir.DerefExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	return v.Ptr.get(), nil
}

// execDerefAssignStmt - "*указатель = значение" - запись через
// "Указатель<T>".
func (in *Interp) execDerefAssignStmt(s *ir.DerefAssignStmt, env *Env, cc *callCtx) (ctrl, error) {
	ptr, err := in.evalExpr(s.Pointer, env, cc)
	if err != nil {
		return ctrl{}, err
	}
	value, err := in.evalExpr(s.Value, env, cc)
	if err != nil {
		return ctrl{}, err
	}
	ptr.Ptr.set(in.cloneValue(value))
	return ctrl{}, nil
}
