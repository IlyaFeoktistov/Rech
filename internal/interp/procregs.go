// Мини-отладчик, tier3 - регистры процессора и точки останова - см.
// package doc internal/semantic/procregs.go. Этот файл - только то, что
// НЕ зависит от платформы: упаковка/распаковка сущности "Регистры" и
// таблица точек останова (обычные данные - pid/адрес/исходный байт, без
// единого системного вызова). Платформенные реализации -
// procregs_linux.go/procregs_windows.go/procregs_other.go (build tags).
package interp

import (
	"sync"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// registersFieldNames - имена полей "Регистры" - см. internal/semantic/
// procregs.go, RegistersFields (тот же принцип дублирования имени, что и
// byteListType/intListType).
var registersFieldNames = []string{
	"rax", "rbx", "rcx", "rdx", "rsi", "rdi", "rbp", "rsp", "rip",
	"r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15", "eflags",
}

// registersValue упаковывает значения регистров (по имени поля) в
// значение "Регистры" - тем же приёмом, что bytesValue/intListValue.
func registersValue(vals map[string]int64) Value {
	fields := make(map[string]Value, len(registersFieldNames))
	for _, name := range registersFieldNames {
		fields[name] = intValue(vals[name])
	}
	return Value{Type: ir.EntityType("Регистры"), Obj: &entityObj{typeName: "Регистры", fields: fields}}
}

// registersFromValue распаковывает значение "Регистры" обратно в
// map[имя поля]значение - обратная операция к registersValue.
func registersFromValue(v Value) map[string]int64 {
	out := make(map[string]int64, len(registersFieldNames))
	for _, name := range registersFieldNames {
		out[name] = v.Obj.fields[name].I
	}
	return out
}

// bpKey/bpTable - таблица точек останова (pid, адрес) -> исходный байт -
// нужна восстановить байт при "снять_точку_останова"/внутреннем шаге
// через уже поставленную точку в "возобновить" (см. platform-specific
// procResume). Общая для обеих платформ - обычные данные, ни одного
// системного вызова.
type bpKey struct {
	pid  int64
	addr int64
}

var bpTable sync.Map
