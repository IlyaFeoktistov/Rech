// Переменные окружения процесса - "получить_переменную_окружения(имя): ?Строка" -
// принимает/возвращает nullable (?Строка, не Kind из internal/builtins),
// поэтому регистрируется отдельно, тем же принципом, что и
// registerCookieFuncs/registerDateTimeFuncs (см. их package doc).
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// registerEnvFuncs регистрирует "получить_переменную_окружения" - Ничто, если такая
// переменная окружения не установлена (не путать с "установлена, но
// пустая строка" - разные вещи для типичного 12-factor конфига, см.
// internal/interp/env.go). "?Строка" НЕ регистрируется здесь безусловно -
// см. package doc registerOptionalStringOnCall (nullable.go).
func registerEnvFuncs(funcs map[string]funcSig) {
	funcs["получить_переменную_окружения"] = funcSig{
		Params:     []ir.Type{ir.TypeString},
		ReturnType: ir.NullableType(ir.TypeString), HasReturn: true,
	}
}
