// Бинарно-безопасный файловый ввод-вывод - см. package doc internal/
// semantic/bytesio.go для полного описания решения.
package interp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// byteListType - см. internal/semantic/bytesio.go, package doc
// queryParamsDictType (cbackend/httpserver.go) для того же принципа
// дублирования имени константой в нескольких пакетах.
const byteListType = "Список<Байт>"

// bytesValue упаковывает data в значение "Список<Байт>" - тем же приёмом,
// что и stringSplit (stringsplit.go): элементы - обычные byteValue,
// generic-коллекция здесь не отличается устройством от любой другой
// Список<T>, построенной пользовательским кодом.
func bytesValue(data []byte) Value {
	elems := make([]Value, len(data))
	for i, b := range data {
		elems[i] = byteValue(int64(b))
	}
	return Value{Type: ir.EntityType(byteListType), Obj: &entityObj{typeName: byteListType, coll: &collection{template: "Список", elems: elems}}}
}

// bytesFromValue распаковывает значение "Список<Байт>" обратно в []byte -
// обратная операция к bytesValue.
func bytesFromValue(v Value) []byte {
	elems := v.Obj.coll.elems
	out := make([]byte, len(elems))
	for i, e := range elems {
		out[i] = byte(e.I)
	}
	return out
}
