package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// headerValuesResultType - тип результата "значения_заголовка" (см. ниже) -
// лениво регистрируется, тем же принципом, что splitResultType
// (stringsplit.go)/queryParamsDictType (httpserver.go).
const headerValuesResultType = "Список<Строка>"

// registerHTTPHeaderFunc регистрирует "значение_заголовка(заголовки:
// Строка, имя: Строка): ?Строка" и "значения_заголовка(заголовки: Строка,
// имя: Строка): Список<Строка>" - общий разбор HTTP-заголовков одной
// строкой (тот же формат "имя: значение\r\n...", что и "заголовки" у
// Запрос/Ответ), не привязанный к конкретной сущности - вызывающий код сам
// передаёт "запрос.заголовки" или "ответ.заголовки". Оба - ПОИСК ПО ИМЕНИ
// БЕЗ УЧЁТА РЕГИСТРА (RFC 7230 - имя поля заголовка регистронезависимо;
// до 2026-09-20 совпадение было точным - живая находка при аудите
// "поддерживаются ли ВСЕ заголовки" - клиент, приславший "content-type"
// строчными, не находился при поиске "Content-Type"). "значение_заголовка" -
// ПЕРВОЕ совпадение (типичный случай - заголовок не повторяется);
// "значения_заголовка" - ВСЕ совпадения по порядку появления (пустой
// список, если ни одного) - настоящие HTTP-заголовки МОГУТ повторяться
// (`X-Forwarded-For`, несколько `Accept` и т. п.), раньше добыть их можно
// было только вручную разобрав сырую строку "заголовки" самому. "?Строка"
// регистрируется лениво, при первом реальном вызове (см. package doc
// registerOptionalStringOnCall, nullable.go), Список<Строка> - тем же
// принципом через registerHeaderValuesTypeOnCall ниже - Ничто/пустой
// список, если такого заголовка нет.
func registerHTTPHeaderFunc(funcs map[string]funcSig) {
	funcs["значение_заголовка"] = funcSig{
		Params:     []ir.Type{ir.TypeString, ir.TypeString},
		ReturnType: ir.NullableType(ir.TypeString), HasReturn: true,
	}
	funcs["значения_заголовка"] = funcSig{
		Params:     []ir.Type{ir.TypeString, ir.TypeString},
		ReturnType: ir.EntityType(headerValuesResultType), HasReturn: true,
	}
}

// registerHeaderValuesTypeOnCall - вызывается из analyzeCallExpr при
// каждом вызове "значения_заголовка" - см. package doc headerValuesResultType.
func registerHeaderValuesTypeOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, headerValuesResultType)
	return err
}
