package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Множество<T> - неупорядоченный набор без повторов (порядок перебора -
// порядок добавления). Хранится так же, как Словарь<T, Логическое>
// (ключ - элемент), поэтому тип элемента - те же типы, что ключ словаря.

// resolveSetTypeName - сигнатура "Множество<T>" при первой встрече.
func resolveSetTypeName(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64, name, argName string) (ir.Type, error) {
	elem, err := resolveTypeName(entities, methods, aliases, interfaces, enums, argName)
	if err != nil {
		return ir.TypeUnknown, diagnostics.Wrap(err, "тип элемента %q в %q", argName, name)
	}
	if !dictKeyKinds[elem.Kind] {
		return ir.TypeUnknown, errNoPos(CodeTypeResolution, "%q: тип элемента %s не поддерживается - допустимы только Целое/Байт/Символ/Логическое/Строка (и перечисления)", name, elem)
	}
	if _, exists := entities[name]; !exists {
		entities[name] = entitySig{IsSet: true, KeyType: elem, ElemType: elem, Equatable: true}
		methods[name] = map[string]funcSig{
			"добавить": {Params: []ir.Type{elem}},
			"содержит": {Params: []ir.Type{elem}, ReturnType: ir.TypeBool, HasReturn: true},
			"удалить":  {Params: []ir.Type{elem}},
			"длина":    {ReturnType: ir.TypeInt, HasReturn: true},
			"пусто":    {ReturnType: ir.TypeBool, HasReturn: true},
			"значения": {ReturnType: collectionListType(elem), HasReturn: true},
			"равен":    {Params: []ir.Type{ir.EntityType(name)}, ReturnType: ir.TypeBool, HasReturn: true},
		}
	}
	return ir.EntityType(name), nil
}
