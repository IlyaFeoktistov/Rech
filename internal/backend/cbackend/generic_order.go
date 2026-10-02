package cbackend

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// genericNeedsEntities - коллекции g нужны typedef'ы пользовательских
// сущностей: её элемент или ключ - сущность/структура, в том числе через
// вложенные коллекции и "?" ("Список<Список<Пара>>",
// "Словарь<Строка, ?Пара>"). Такая коллекция идёт в группу после сущностей
// (genericsCodeAfterEntities) - иначе её "равен"/конструктор ссылались бы на
// функции внутренней коллекции, объявленные ниже по тексту.
func genericNeedsEntities(ctx *irGenCtx, entities map[string]*ir.Entity, generics map[string]ir.GenericInstantiation, g ir.GenericInstantiation) bool {
	seen := map[string]bool{}
	var needs func(t ir.Type) bool
	needs = func(t ir.Type) bool {
		if t.Kind == ir.KindNullable {
			t = ctx.nullableInner[t.Entity]
		}
		if t.Kind != ir.KindEntity {
			return false
		}
		if entities[t.Entity] != nil {
			return true
		}
		inner, ok := generics[t.Entity]
		if !ok || seen[t.Entity] {
			return false
		}
		seen[t.Entity] = true
		return needs(inner.ElemType) || needs(inner.KeyType)
	}
	return needs(g.ElemType) || needs(g.KeyType)
}
