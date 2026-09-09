package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// ColorFields - поля встроенной сущности "RGBЦвет" - Байт (0-255), не Целое:
// канал физически не бывает за пределами этого диапазона, явный (Байт)
// каст на месте вызова - единственная защита от "новый RGBЦвет(300, ...)"
// с неопределённым результатом (сравнимая ситуация уже была найдена вживую
// у Rech-версии examples/консоль/цвет.rech, откуда встроенная версия здесь
// и переносит то же самое решение).
var ColorFields = []ir.Field{
	{Name: "красный", Type: ir.TypeByte},
	{Name: "зелёный", Type: ir.TypeByte},
	{Name: "синий", Type: ir.TypeByte},
}

// registerColorFuncs регистрирует сущность "RGBЦвет" и функции "rgbцвет_из_hex"/
// "rgbцвет_в_hex" - тот же принцип, что и registerDateTimeFuncs (package doc
// там): принимают/возвращают сущность, не примитив, значит не входят в
// internal/builtins.All, регистрируются отдельно в каждом из трёх
// потребителей (semantic/cbackend/interp).
func registerColorFuncs(entities map[string]entitySig, funcs map[string]funcSig) {
	registerHTTPEntity(entities, "RGBЦвет", ColorFields)
	funcs["rgbцвет_из_hex"] = funcSig{Params: []ir.Type{ir.TypeString}, ReturnType: ir.EntityType("RGBЦвет"), HasReturn: true}
	funcs["rgbцвет_в_hex"] = funcSig{Params: []ir.Type{ir.EntityType("RGBЦвет")}, ReturnType: ir.TypeString, HasReturn: true}
}
