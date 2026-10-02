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
	"unsafe"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// pointerTarget - см. package doc. Четвёртый, отдельный вид - raw (FFI,
// ffi.go) - адрес не RECH-хранилища, а чужой (C) памяти: результат внешней
// функции типа "Указатель<T>" или поле "Указатель<T>" структуры, читаемой
// из C. get/set в этом случае идут через unsafe.Pointer/reflect (см.
// ffi.go, in.readFFIValue/in.setFFIValue) - интерпретатору нужен доступ к
// той же таблице типов (in.pointerElemTypes и т. д.), поэтому raw несёт
// *Interp, в отличие от остальных трёх случаев. raw хранится уже как
// unsafe.Pointer (не uintptr) - `go vet` (unsafeptr) справедливо не
// одобряет хранение адреса как uintptr с последующим обратным
// преобразованием в unsafe.Pointer (это небезопасно для памяти, за
// которой следит Go GC), но здесь адрес всегда чужой (C), не Go-heap -
// хранить его сразу как unsafe.Pointer одновременно и корректно, и не
// вызывает эту диагностику.
type pointerTarget struct {
	env  *Env // !=nil - переменная/параметр (env+name)
	name string

	obj   *entityObj // !=nil - поле сущности (obj+field)
	field string

	coll     *collection // !=nil - элемент списка (coll+index)
	index    int64
	collName string // имя инстанциации, только для сообщения об ошибке

	// isRaw - четвёртый случай, см. package doc выше.
	isRaw    bool
	raw      unsafe.Pointer
	elemType ir.Type
	interp   *Interp
}

// rawPointerValue - Указатель<T>, несущий сырой чужой (C) адрес - см.
// package doc pointerTarget. elemType - T (ir.Program.Pointers).
func rawPointerValue(t, elemType ir.Type, addr unsafe.Pointer, in *Interp) Value {
	return pointerValue(t, &pointerTarget{isRaw: true, raw: addr, elemType: elemType, interp: in})
}

func (p *pointerTarget) get() Value {
	if p == nil {
		// "Указатель<T>", равный "Ничто" (NULL) - см. package doc
		// evalNullLiteral/evalNullCheckExpr (nullable.go) - разыменование
		// такого указателя недопустимо, тот же rtmsg, что и у "?Тип".
		panicRuntime("%s", rtmsg.NullAccess)
	}
	switch {
	case p.env != nil:
		v, _ := p.env.get(p.name)
		return v
	case p.obj != nil:
		return p.obj.getField(p.field)
	case p.isRaw:
		return p.interp.readRawPointerValue(p.raw, p.elemType)
	default:
		if p.index < 0 || p.index >= int64(len(p.coll.elems)) {
			panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.IndexOutOfRange(p.collName))
		}
		return p.coll.elems[p.index]
	}
}

func (p *pointerTarget) set(v Value) {
	if p == nil {
		panicRuntime("%s", rtmsg.NullAccess)
	}
	switch {
	case p.env != nil:
		p.env.set(p.name, v)
	case p.obj != nil:
		p.obj.setField(p.field, v)
	case p.isRaw:
		p.interp.setRawPointerValue(p.raw, p.elemType, v)
	default:
		if p.index < 0 || p.index >= int64(len(p.coll.elems)) {
			panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.IndexOutOfRange(p.collName))
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
			panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.IndexOutOfRange(recv.CollectionName))
		}
		return pointerValue(e.Type, &pointerTarget{coll: coll, index: idx.I, collName: recv.CollectionName}), nil
	case *ir.ArrayIndexExpr:
		target, err := in.arrayElemTarget(recv, env, cc)
		if err != nil {
			return Value{}, err
		}
		return pointerValue(e.Type, target), nil
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

// derefRoot - разыменование в основании цепочки полей получателя
// ("(*п).поле", "(*п).а.б", "п[i].поле").
func derefRoot(e ir.Expr) (*ir.DerefExpr, bool) {
	for {
		switch v := e.(type) {
		case *ir.FieldAccess:
			e = v.Receiver
		case *ir.DerefExpr:
			return v, true
		default:
			return nil, false
		}
	}
}

// execFieldAssignThroughPointer - запись поля структуры, лежащей по
// указателю: по сырому C-адресу значение читается копией, поэтому после
// записи поля оно пишется обратно по тому же адресу.
func (in *Interp) execFieldAssignThroughPointer(s *ir.FieldAssign, root *ir.DerefExpr, env *Env, cc *callCtx) (ctrl, error) {
	ptr, err := in.evalExpr(root.Value, env, cc)
	if err != nil {
		return ctrl{}, err
	}
	if ptr.Ptr == nil {
		panicRuntime("%s", rtmsg.NullAccess)
	}
	value, err := in.evalExpr(s.Value, env, cc)
	if err != nil {
		return ctrl{}, err
	}
	base := ptr.Ptr.get()
	var path []string
	for e := s.Receiver; e != ir.Expr(root); {
		fa := e.(*ir.FieldAccess)
		path = append([]string{fa.Field}, path...)
		e = fa.Receiver
	}
	recv := base
	for _, f := range path {
		recv = recv.Obj.getField(f)
	}
	recv.Obj.setField(s.Field, in.cloneValue(value))
	if ptr.Ptr.isRaw {
		ptr.Ptr.set(base)
	}
	return ctrl{}, nil
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

// offset - адрес i-го элемента массива, начинающегося по адресу p
// ("&п[i]", ir.PointerOffsetExpr). У сырого адреса - настоящая арифметика
// указателей (i * размер T); у адреса элемента списка - соседний элемент;
// у адреса переменной/поля массива за ним нет - годится только i == 0.
func (p *pointerTarget) offset(i int64) *pointerTarget {
	if p == nil {
		panicRuntime("%s", rtmsg.NullAccess)
	}
	switch {
	case p.isRaw:
		goType, err := p.interp.ffiGoType(p.elemType)
		if err != nil {
			panicRuntime("%v", err)
		}
		moved := *p
		moved.raw = unsafe.Add(p.raw, uintptr(i)*goType.Size())
		return &moved
	case p.coll != nil:
		moved := *p
		moved.index += i
		return &moved
	default:
		if i != 0 {
			panicRuntimeAs(rtmsg.PanicOutOfRange, "%s", rtmsg.PointerIndexOutOfRange(i))
		}
		return p
	}
}

// evalPointerOffsetExpr - "&п[i]"; чтение/запись "п[i]" - DerefExpr/
// DerefAssignStmt над ним.
func (in *Interp) evalPointerOffsetExpr(e *ir.PointerOffsetExpr, env *Env, cc *callCtx) (Value, error) {
	ptr, err := in.evalExpr(e.Pointer, env, cc)
	if err != nil {
		return Value{}, err
	}
	idx, err := in.evalExpr(e.Index, env, cc)
	if err != nil {
		return Value{}, err
	}
	return pointerValue(e.Type, ptr.Ptr.offset(idx.I)), nil
}
