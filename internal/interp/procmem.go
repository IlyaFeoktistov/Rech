// Мини-отладчик - см. package doc internal/semantic/procmem.go. Платформенные
// реализации - procmem_linux.go/procmem_windows.go/procmem_other.go (build
// tags) - этот файл содержит только то, что НЕ зависит от платформы: тип
// generic-коллекции результата "найти_байты".
package interp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// intListType - "Список<Целое>", результат "найти_байты" (адреса
// совпадений) - см. internal/semantic/procmem.go.
const intListType = "Список<Целое>"

// intListValue упаковывает addrs в значение "Список<Целое>" - тем же
// приёмом, что bytesValue (bytesio.go).
func intListValue(addrs []int64) Value {
	elems := make([]Value, len(addrs))
	for i, a := range addrs {
		elems[i] = intValue(a)
	}
	return Value{Type: ir.EntityType(intListType), Obj: &entityObj{typeName: intListType, coll: &collection{template: "Список", elems: elems}}}
}
