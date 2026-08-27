package lsp

import (
	"strings"
	"testing"
)

func TestCompletionAfterDotListsFieldsAndMethods(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	// "вывести т.x" - точка сразу после "т", позиция курсора - сразу после
	// точки (там, где пользователь печатает имя поля/метода).
	line := lineOf(t, hoverTestSrc, "вывести т.x")
	dotCol := runeCol(t, "    вывести т.x", ".")
	items := d.completion(line, dotCol+1)

	labels := map[string]CompletionItem{}
	for _, it := range items {
		labels[it.Label] = it
	}
	if _, ok := labels["x"]; !ok {
		t.Fatalf("ожидалось поле x в автодополнении, получено %+v", items)
	}
	if _, ok := labels["y"]; !ok {
		t.Fatalf("ожидалось поле y в автодополнении, получено %+v", items)
	}
	if _, ok := labels["длина"]; !ok {
		t.Fatalf("ожидался метод длина в автодополнении, получено %+v", items)
	}
	if labels["x"].Kind != kindField {
		t.Fatalf("поле x должно иметь Kind поля, получено %d", labels["x"].Kind)
	}
	if labels["длина"].Kind != kindMethod {
		t.Fatalf("метод длина должен иметь Kind метода, получено %d", labels["длина"].Kind)
	}
	if !strings.Contains(labels["x"].Documentation, "Горизонтальная координата.") {
		t.Fatalf("doc-комментарий не попал в автодополнение поля: %+v", labels["x"])
	}
}

func TestCompletionGeneralListsLocalsAndFunctions(t *testing.T) {
	d := newDoc("test://a.rech", hoverTestSrc)
	line := lineOf(t, hoverTestSrc, "вернуть итог")
	col := runeCol(t, "    вернуть итог", "итог")
	items := d.completion(line, col)

	labels := map[string]bool{}
	for _, it := range items {
		labels[it.Label] = true
	}
	if !labels["итог"] {
		t.Fatalf("ожидалась локальная переменная итог в автодополнении")
	}
	if !labels["кого"] || !labels["кому"] {
		t.Fatalf("ожидались параметры функции в автодополнении, получено %v", labels)
	}
	if !labels["сложить"] {
		t.Fatalf("ожидалась функция сложить в автодополнении")
	}
	if !labels["Точка"] {
		t.Fatalf("ожидалась сущность Точка в автодополнении")
	}
	if !labels["если"] {
		t.Fatalf("ожидалось ключевое слово если в автодополнении")
	}
}
