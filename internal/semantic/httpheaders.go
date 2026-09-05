package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// registerHTTPHeaderFunc регистрирует "значение_заголовка(заголовки:
// Строка, имя: Строка): ?Строка" - общий разбор HTTP-заголовков одной
// строкой (тот же формат "имя: значение\r\n...", что и "заголовки" у
// Запрос/Ответ), не привязанный к конкретной сущности - вызывающий код сам
// передаёт "запрос.заголовки" или "ответ.заголовки". Тот же принцип
// ленивой регистрации "?Строка", что и registerEnvFuncs (получить_
// переменную_окружения) - Ничто, если такого заголовка нет.
func registerHTTPHeaderFunc(entities map[string]entitySig, funcs map[string]funcSig) {
	if _, exists := entities["?Строка"]; !exists {
		entities["?Строка"] = entitySig{IsNullable: true, ElemType: ir.TypeString}
	}
	funcs["значение_заголовка"] = funcSig{
		Params:     []ir.Type{ir.TypeString, ir.TypeString},
		ReturnType: ir.NullableType(ir.TypeString), HasReturn: true,
	}
}
