package interp

import (
	"fmt"
	"unsafe"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Внешняя переменная ("внешняя Целое счётчик", ir.GlobalVar.ExternCName) -
// значение живёт в памяти C-символа: каждое чтение и запись идут по его
// адресу (Env.lockedGet/lockedSetExisting), а не в копию - как и в
// cbackend, где имя переменной - макрос на сам символ. Ширина - та же, что
// у C-типа в cbackend (valuetype.go).

// externVar - адрес C-символа и тип переменной.
type externVar struct {
	addr unsafe.Pointer
	typ  ir.Type
}

// read - текущее значение переменной.
func (v *externVar) read() Value {
	p := v.addr
	switch v.typ.Kind {
	case ir.KindFloat:
		return floatValue(*(*float64)(p))
	case ir.KindFloat32:
		return float32Value(float64(*(*float32)(p)))
	case ir.KindBool:
		return boolValue(*(*bool)(p))
	case ir.KindChar:
		return charValue(rune(*(*int32)(p)))
	case ir.KindByte:
		return byteValue(int64(*(*uint8)(p)))
	case ir.KindInt8:
		return Value{Type: v.typ, I: int64(*(*int8)(p))}
	case ir.KindInt16:
		return Value{Type: v.typ, I: int64(*(*int16)(p))}
	case ir.KindInt32:
		return Value{Type: v.typ, I: int64(*(*int32)(p))}
	case ir.KindUint16:
		return Value{Type: v.typ, I: int64(*(*uint16)(p))}
	case ir.KindUint32:
		return Value{Type: v.typ, I: int64(*(*uint32)(p))}
	default: // Целое, Натуральное - 64 бита
		return Value{Type: v.typ, I: *(*int64)(p)}
	}
}

// write - записать значение в переменную.
func (v *externVar) write(val Value) {
	p := v.addr
	switch v.typ.Kind {
	case ir.KindFloat:
		*(*float64)(p) = val.F
	case ir.KindFloat32:
		*(*float32)(p) = float32(val.F)
	case ir.KindBool:
		*(*bool)(p) = val.B
	case ir.KindChar, ir.KindInt32:
		*(*int32)(p) = int32(val.I)
	case ir.KindByte:
		*(*uint8)(p) = uint8(val.I)
	case ir.KindInt8:
		*(*int8)(p) = int8(val.I)
	case ir.KindInt16:
		*(*int16)(p) = int16(val.I)
	case ir.KindUint16:
		*(*uint16)(p) = uint16(val.I)
	case ir.KindUint32:
		*(*uint32)(p) = uint32(val.I)
	default:
		*(*int64)(p) = val.I
	}
}

// declareExternVar - найти символ g.ExternCName (те же библиотеки, что у
// внешних функций) и зарегистрировать переменную в окружении модуля.
func (in *Interp) declareExternVar(g *ir.GlobalVar) error {
	addr, err := in.resolveFFISymbol(g.ExternCName)
	if err != nil {
		return fmt.Errorf("внешняя переменная %q: %w", g.Name, err)
	}
	if in.globals.externs == nil {
		in.globals.externs = map[string]*externVar{}
	}
	// Адрес C-символа от dlsym (не память Go - сборщик мусора его не
	// двигает); тип и ширина проверены семантикой (isFFIVarType).
	// #nosec G103
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	in.globals.externs[g.Name] = &externVar{addr: ptr, typ: g.Type}
	return nil
}
