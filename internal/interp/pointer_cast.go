package interp

import (
	"unsafe"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// castPointer - "(Указатель<T>) x"/"(Целое) п" (internal/semantic/
// pointer_cast.go): обе стороны - адрес. Адрес от C (сырой) переносится
// как есть с новым типом; адрес Rech-хранилища (&переменная) настоящего
// адреса памяти не имеет (см. package doc pointer.go) - привести его к
// другому типу или к числу интерпретатор не может.
func (in *Interp) castPointer(v Value, to ir.Type) Value {
	var addr uintptr
	switch {
	case v.Type.Kind != ir.KindPointer:
		addr = uintptr(v.I)
	case v.Ptr == nil:
		addr = 0
	case v.Ptr.isRaw:
		addr = uintptr(v.Ptr.raw)
	default:
		panicRuntime("%s", rtmsg.InterpPointerCastUnsupported(v.Type.String(), to.String()))
	}
	if to.Kind != ir.KindPointer {
		return Value{Type: to, I: int64(addr)}
	}
	if addr == 0 {
		return Value{Type: to}
	}
	return rawPointerValue(to, in.pointerElemTypes[to.Entity], unsafe.Add(unsafe.Pointer(nil), addr), in)
}
