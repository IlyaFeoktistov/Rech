package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// splitResultType - тип результата "разбить(строка, разделитель):
// Список<Строка>" (docs/SPEC.md, "Встроенные функции") - регистрируется
// лениво, только когда "разбить" реально вызвана хоть раз в программе
// (registerSplitTypeOnCall, вызывается из analyzeCallExpr), тем же
// принципом, что и queryParamsDictType (httpserver.go) - иначе каждая
// программа получала бы неиспользуемый typedef/методы Список<Строка>
// просто из факта существования этой встроенной функции в языке.
const splitResultType = "Список<Строка>"

// registerStringSplitFunc - "разбить(строка: Строка, разделитель: Строка):
// Список<Строка>" - разбивает строку на части по буквальному вхождению
// разделителя (не regex), тем же принципом, что и strings.Split/split() в
// других языках. Пустой разделитель - ошибка (см. rtmsg.SplitEmptySeparator).
func registerStringSplitFunc(funcs map[string]funcSig) {
	funcs["разбить"] = funcSig{Params: []ir.Type{ir.TypeString, ir.TypeString}, ReturnType: ir.EntityType(splitResultType), HasReturn: true}
}

// registerSplitTypeOnCall - вызывается из analyzeCallExpr при каждом
// вызове "разбить" - см. package doc splitResultType. resolveTypeName сам
// идемпотентен (resolveGenericTypeName не пересоздаёт запись, если она уже
// есть), поэтому безопасно звать на каждый вызов, не только на первый.
func registerSplitTypeOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, splitResultType)
	return err
}
