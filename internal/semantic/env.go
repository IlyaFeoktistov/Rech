// Переменные окружения процесса - "получить_переменную_окружения(имя): ?Строка" -
// принимает/возвращает nullable (?Строка, не Kind из internal/builtins),
// поэтому регистрируется отдельно, тем же принципом, что и
// registerCookieFuncs/registerDateTimeFuncs (см. их package doc).
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// registerEnvFuncs регистрирует "получить_переменную_окружения" - Ничто, если такая
// переменная окружения не установлена (не путать с "установлена, но
// пустая строка" - разные вещи для типичного 12-factor конфига, см.
// internal/interp/env.go).
func registerEnvFuncs(entities map[string]entitySig, funcs map[string]funcSig) {
	if _, exists := entities["?Строка"]; !exists {
		entities["?Строка"] = entitySig{IsNullable: true, ElemType: ir.TypeString}
	}
	funcs["получить_переменную_окружения"] = funcSig{
		Params:     []ir.Type{ir.TypeString},
		ReturnType: ir.NullableType(ir.TypeString), HasReturn: true,
	}
}
