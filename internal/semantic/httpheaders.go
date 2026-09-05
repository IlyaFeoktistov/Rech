package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// registerHTTPHeaderFunc регистрирует "значение_заголовка(заголовки:
// Строка, имя: Строка): ?Строка" - общий разбор HTTP-заголовков одной
// строкой (тот же формат "имя: значение\r\n...", что и "заголовки" у
// Запрос/Ответ), не привязанный к конкретной сущности - вызывающий код сам
// передаёт "запрос.заголовки" или "ответ.заголовки". "?Строка"
// регистрируется лениво, при первом реальном вызове (см. package doc
// registerOptionalStringOnCall, nullable.go), не безусловно здесь - Ничто,
// если такого заголовка нет.
func registerHTTPHeaderFunc(funcs map[string]funcSig) {
	funcs["значение_заголовка"] = funcSig{
		Params:     []ir.Type{ir.TypeString, ir.TypeString},
		ReturnType: ir.NullableType(ir.TypeString), HasReturn: true,
	}
}
