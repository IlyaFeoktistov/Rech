package semantic

import (
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestAnalyzeNestedListElementOK - Список<Список<Целое>> (встроенная
// generic-коллекция как элемент другой) - разрешено, в отличие от
// пользовательской сущности как элемента.
func TestAnalyzeNestedListElementOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная матрица = новый Список<Список<Целое>>()
    переменная строка = новый Список<Целое>()
    строка.добавить(1)
    матрица.добавить(строка)
    вывести матрица.длина()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDictWithListValueOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная карта = новый Словарь<Строка, Список<Целое>>()
    переменная список = новый Список<Целое>()
    карта.установить("а", список)
    вывести карта.длина()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestTopoSortGenericNamesOrdersInnerFirst - "Список<Список<Целое>>"
// сортируется алфавитно РАНЬШЕ "Список<Целое>" (кириллическая "С" < "Ц"),
// но её C-typedef физически ЗАВИСИТ от уже готового typedef "Список<Целое>"
// - topoSortGenericNames обязана вернуть внутренний тип первым несмотря на
// алфавитный порядок.
func TestTopoSortGenericNamesOrdersInnerFirst(t *testing.T) {
	entities := map[string]entitySig{
		"Список<Целое>":         {ElemType: ir.TypeInt},
		"Список<Список<Целое>>": {ElemType: ir.EntityType("Список<Целое>")},
	}
	names := []string{"Список<Список<Целое>>", "Список<Целое>"}
	got := topoSortGenericNames(names, entities)
	if len(got) != 2 || got[0] != "Список<Целое>" || got[1] != "Список<Список<Целое>>" {
		t.Fatalf("topoSortGenericNames = %v, ожидался внутренний тип первым", got)
	}
}
