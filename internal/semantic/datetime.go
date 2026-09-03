package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// dateTimeFields - фиксированные поля встроенной сущности "ДатаВремя", в
// этом порядке (позиционный конструктор "новый ДатаВремя(...)" следует
// ему) - календарные поля (не единственный timestamp), тем же выбором,
// что и "Запрос"/"Ответ": бесплатный доступ к полю без отдельных функций-
// геттеров, ценой фиксированного представления (всегда распакован в
// год/месяц/день/..., не хранится как единое число секунд с эпохи).
// Часовой пояс - только UTC (время_как_дата/дата_как_время,
// datetime_shared.go) - без него платформенная база часовых поясов не
// нужна вовсе, тот же принцип простоты, что и у HTTP-клиента выше
// (собственный минимальный протокол вместо libcurl).
var dateTimeFields = []ir.Field{
	{Name: "год", Type: ir.TypeInt},
	{Name: "месяц", Type: ir.TypeInt},
	{Name: "день", Type: ir.TypeInt},
	{Name: "час", Type: ir.TypeInt},
	{Name: "минута", Type: ir.TypeInt},
	{Name: "секунда", Type: ir.TypeInt},
}

// registerDateTimeFuncs регистрирует сущность "ДатаВремя" и функции
// "время_как_дата"/"дата_как_время"/"дату_в_строку" - тот же принцип, что
// и у "запрос" (HTTP-клиент): они принимают/возвращают сущность, не
// примитив, значит не входят в internal/builtins.All (там только Kind
// примитивов) и регистрируются отдельно в каждом из трёх потребителей
// (semantic/cbackend/interp). "время" (текущее время, без сущности в
// сигнатуре) - в internal/builtins.All как обычно.
func registerDateTimeFuncs(entities map[string]entitySig, funcs map[string]funcSig) {
	registerHTTPEntity(entities, "ДатаВремя", dateTimeFields)
	funcs["время_как_дата"] = funcSig{Params: []ir.Type{ir.TypeInt}, ReturnType: ir.EntityType("ДатаВремя"), HasReturn: true}
	funcs["дата_как_время"] = funcSig{Params: []ir.Type{ir.EntityType("ДатаВремя")}, ReturnType: ir.TypeInt, HasReturn: true}
	funcs["дату_в_строку"] = funcSig{Params: []ir.Type{ir.EntityType("ДатаВремя")}, ReturnType: ir.TypeString, HasReturn: true}
}
