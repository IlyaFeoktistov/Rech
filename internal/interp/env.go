package interp

import (
	"os"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// envValue - "получить_переменную_окружения(имя): ?Строка" - переменная окружения
// процесса (os.Getenv не различает "пусто" и "не установлена" - os.
// LookupEnv, как раз то различие, которое здесь нужно: "не установлена" -
// Ничто, "установлена, но пустая строка" - непустой ?Строка со значением
// "", разные вещи для типичного конфига 12-factor).
func envValue(name string) Value {
	nullableType := ir.NullableType(ir.TypeString)
	if v, ok := os.LookupEnv(name); ok {
		s := stringValue(v)
		return Value{Type: nullableType, Nullable: &s}
	}
	return Value{Type: nullableType}
}
