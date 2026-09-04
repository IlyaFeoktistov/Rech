package interp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// splitResultType - см. internal/semantic/stringsplit.go, splitResultType -
// имя generic-инстанциации, зарегистрированной для "разбить" принудительно
// (registerStringSplitFunc), тем же принципом, что queryParamsDictType у
// httpserver.go.
const splitResultType = "Список<Строка>"

// stringSplit - "разбить(строка, разделитель): Список<Строка>" - буквальное
// вхождение разделителя (strings.Split, не regex), тем же алгоритмом, что и
// cbackend.rech_split_string (stringsplit.go). Пустой разделитель - ошибка
// (rtmsg.SplitEmptySeparator), а не strings.Split-по-рунам, как без явного
// separator сделал бы Go.
func stringSplit(s, sep string) Value {
	if sep == "" {
		panicRuntime("%s", rtmsg.SplitEmptySeparator)
	}
	parts := strings.Split(s, sep)
	elems := make([]Value, len(parts))
	for i, p := range parts {
		elems[i] = stringValue(p)
	}
	return Value{Type: ir.EntityType(splitResultType), Obj: &entityObj{typeName: splitResultType, coll: &collection{template: "Список", elems: elems}}}
}
