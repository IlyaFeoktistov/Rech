package semantic

import (
	"sort"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Справочные данные семантики для автодополнения internal/lsp: какие
// методы-сахар есть у встроенного типа получателя и какие константы у
// числового примитива - те же таблицы, по которым анализируется сам код.

// SugarMethods - методы-сахар получателя типа t ("бд.выполнить" -> свободная
// "выполнить"): имя метода -> имя свободной функции; nil - таких нет.
func SugarMethods(t ir.Type) map[string]string {
	return handleMethodSugar[t]
}

// DateTimeMethodNames - методы "ДатаВремя" ("добавить_дни", "раньше", ...)
// по алфавиту.
func DateTimeMethodNames() []string {
	out := make([]string, 0, len(dateTimeSugarMethodNames))
	for name := range dateTimeSugarMethodNames {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// PrimitiveTypeConstantNames - константы "Тип::ИМЯ" числового примитива
// typeName (МАКСИМУМ/МИНИМУМ/ШИРИНА, у Дробное/Действительное ещё
// БЕСКОНЕЧНОСТЬ/НЕ_ЧИСЛО); nil - typeName не числовой примитив.
func PrimitiveTypeConstantNames(typeName string) []string {
	t, ok := primitiveTypes[typeName]
	if !ok {
		return nil
	}
	if _, _, ok := t.Kind.IntWidthSigned(); ok {
		return intTypeConstNames
	}
	if t == ir.TypeFloat || t == ir.TypeFloat32 {
		return floatTypeConstNames
	}
	return nil
}

// PrimitiveTypeNames - имена встроенных примитивных типов по алфавиту.
func PrimitiveTypeNames() []string {
	out := make([]string, 0, len(primitiveTypes))
	for name := range primitiveTypes {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// PrimitiveType - встроенный примитивный тип по имени ("Целое8").
func PrimitiveType(name string) (ir.Type, bool) {
	t, ok := primitiveTypes[name]
	return t, ok
}
