// Hover/автодополнение методов встроенных generic-коллекций (Список<T>/
// Стопка<T>/Очередь<T>/Словарь<К, В>, docs/SPEC.md, раздел 8) - до этой
// фичи наведение/автодополнение после точки на значении одного из этих
// типов не показывало НИЧЕГО: их инстанциация ("Список<Целое>") хоть и
// несёт ir.Type{Kind: KindEntity, Entity: "Список<Целое>"} (та же форма,
// что и у пользовательской сущности), но НЕ получает настоящий *ir.Entity
// в выходном IR - C-представление растущего массива/хеш-таблицы у неё
// строковый шаблон (internal/backend/cbackend), мономорфизированный под
// каждую встретившуюся инстанциацию, а не обычная структура полей с
// методами, которую стоило бы эмитить в ir.Program.Entities. d.entities
// (заполняется из irProg.Entities, document.go) поэтому никогда не несёт
// ключ "Список<Целое>" - hoverMember/memberCompletion, которым нужен
// настоящий *ir.Entity, не находили её вовсе.
//
// Курируемая таблица методов (methodsFor) - не настоящая интроспекция
// компилятора (там этих методов тоже нет как отдельных ir.Method - они
// либо синтезируются генератором C-кода по имени вызова, либо разбираются
// напрямую интерпретатором, см. internal/backend/cbackend/collections.go/
// internal/interp/collections.go) - список должен обновляться вручную,
// если методы встроенных коллекций когда-нибудь изменятся. Element-типы
// (T/К/В) - СТРОКИ аргументов из самого имени инстанциации
// (semantic.ParseGenericTypeNameMulti), не ir.Type - показывать в hover
// нужно ровно то, что написал пользователь ("Список<Пользователь>" -
// "значение: Пользователь"), лишнее разрешение типа не требуется.
package lsp

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// collectionMethod - одна запись курируемой таблицы (methodsFor) - Params
// уже отформатированы ("имя: Тип"), Return - "" у методов без результата.
type collectionMethod struct {
	Name   string
	Params []string
	Return string
}

// methodsFor - методы template<args...> в порядке объявления в SPEC.md -
// "", false, если template - не одна из четырёх встроенных коллекций, или
// число аргументов не совпадает с ожидаемым (Список/Стопка/Очередь - один,
// Словарь - два) - тогда это не встроенная коллекция вовсе (пользовательский
// generic-тип с таким же именем невозможен - все четыре зарезервированы,
// см. internal/builtins.ReservedEntityNames), а незавершённая инстанциация
// каким-то другим путём, показывать нечего.
func methodsFor(template string, args []string) ([]collectionMethod, bool) {
	switch template {
	case "Список":
		if len(args) != 1 {
			return nil, false
		}
		t := args[0]
		return []collectionMethod{
			{"добавить", []string{"значение: " + t}, ""},
			{"получить", []string{"индекс: Целое"}, t},
			{"установить", []string{"индекс: Целое", "значение: " + t}, ""},
			{"удалить", []string{"индекс: Целое"}, t},
			{"первый", nil, t},
			{"последний", nil, t},
			{"длина", nil, "Целое"},
			{"пусто", nil, "Логическое"},
			{"равен", []string{"другой: Список<" + t + ">"}, "Логическое"},
		}, true
	case "Стопка":
		if len(args) != 1 {
			return nil, false
		}
		t := args[0]
		return []collectionMethod{
			{"положить", []string{"значение: " + t}, ""},
			{"снять", nil, t},
			{"верх", nil, t},
			{"длина", nil, "Целое"},
			{"пусто", nil, "Логическое"},
		}, true
	case "Очередь":
		if len(args) != 1 {
			return nil, false
		}
		t := args[0]
		return []collectionMethod{
			{"добавить", []string{"значение: " + t}, ""},
			{"извлечь", nil, t},
			{"первый", nil, t},
			{"длина", nil, "Целое"},
			{"пусто", nil, "Логическое"},
		}, true
	case "Словарь":
		if len(args) != 2 {
			return nil, false
		}
		k, v := args[0], args[1]
		return []collectionMethod{
			{"установить", []string{"ключ: " + k, "значение: " + v}, ""},
			{"получить", []string{"ключ: " + k}, v},
			{"содержит", []string{"ключ: " + k}, "Логическое"},
			{"удалить", []string{"ключ: " + k}, ""},
			{"длина", nil, "Целое"},
			{"пусто", nil, "Логическое"},
			{"равен", []string{"другой: Словарь<" + k + ", " + v + ">"}, "Логическое"},
		}, true
	}
	return nil, false
}

// collectionMethodsOf - methodsFor по готовому имени инстанциации
// ("Список<Целое>") - разбирает его через semantic.ParseGenericTypeNameMulti
// (тот же разбор, что использует сам компилятор для generic-имён, включая
// вложенные - "Список<Список<Целое>>" разбирается на template "Список",
// args ["Список<Целое>"], не спотыкается о внутреннюю "<...>").
func collectionMethodsOf(typeName string) ([]collectionMethod, bool) {
	template, args, ok := semantic.ParseGenericTypeNameMulti(typeName)
	if !ok {
		return nil, false
	}
	return methodsFor(template, args)
}

// collectionMethodSignature - "имя(параметр: Тип, ...): ТипРезультата".
func collectionMethodSignature(name string, m collectionMethod) string {
	sig := fmt.Sprintf("%s(%s)", name, strings.Join(m.Params, ", "))
	if m.Return != "" {
		sig += ": " + m.Return
	}
	return sig
}

// hoverGenericCollectionMember - name - метод, вызванный на значении типа
// typeName ("Список<Целое>" и т. п.) - "", false, если typeName - не
// встроенная коллекция или у неё нет метода с таким именем.
func hoverGenericCollectionMember(typeName, name string) (string, bool) {
	methods, ok := collectionMethodsOf(typeName)
	if !ok {
		return "", false
	}
	for _, m := range methods {
		if m.Name == name {
			base, _, _ := strings.Cut(typeName, "<")
			return withThrows(codeBlock(typeName+"."+collectionMethodSignature(m.Name, m)), describeNames(builtins.CollectionMethodThrows[base][name])), true
		}
	}
	return "", false
}

// genericCollectionCompletion - автодополнение после точки на значении
// typeName ("Список<Целое>" и т. п.) - nil, false, если typeName - не
// встроенная коллекция.
func genericCollectionCompletion(typeName string) ([]CompletionItem, bool) {
	methods, ok := collectionMethodsOf(typeName)
	if !ok {
		return nil, false
	}
	out := make([]CompletionItem, len(methods))
	for i, m := range methods {
		out[i] = CompletionItem{Label: m.Name, Kind: kindMethod, Detail: collectionMethodSignature(m.Name, m)}
	}
	return out, true
}
