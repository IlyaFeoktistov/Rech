package lsp

import "testing"

func TestHoverOnListMethodCall(t *testing.T) {
	src := `
функция главная
{
    переменная сп = новый Список<Целое>()
    сп.добавить(1)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "сп.добавить(1)")
	col := runeCol(t, "    сп.добавить(1)", "добавить")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на методе Список<Целое>")
	}
	if text != "```rech\nСписок<Целое>.добавить(значение: Целое)\n```" {
		t.Fatalf("неожиданный текст: %q", text)
	}
}

func TestHoverOnListGetReturnsElementType(t *testing.T) {
	src := `
функция главная
{
    переменная сп = новый Список<Строка>()
    переменная а = сп.получить(0)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "сп.получить(0)")
	col := runeCol(t, "    переменная а = сп.получить(0)", "получить")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover")
	}
	if text != "```rech\nСписок<Строка>.получить(индекс: Целое): Строка\n```" {
		t.Fatalf("неожиданный текст: %q", text)
	}
}

func TestHoverOnDictMethodCall(t *testing.T) {
	src := `
функция главная
{
    переменная сл = новый Словарь<Строка, Целое>()
    сл.установить("а", 1)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, `сл.установить("а", 1)`)
	col := runeCol(t, `    сл.установить("а", 1)`, "установить")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на методе Словарь<Строка, Целое>")
	}
	if text != "```rech\nСловарь<Строка, Целое>.установить(ключ: Строка, значение: Целое)\n```" {
		t.Fatalf("неожиданный текст: %q", text)
	}
}

func TestHoverOnStackAndQueueMethods(t *testing.T) {
	src := `
функция главная
{
    переменная ст = новая Стопка<Целое>()
    ст.положить(1)
    переменная оч = новая Очередь<Целое>()
    оч.добавить(1)
}
`
	d := newDoc("test://a.rech", src)

	line := lineOf(t, src, "ст.положить(1)")
	col := runeCol(t, "    ст.положить(1)", "положить")
	text, ok := d.hover(line, col)
	if !ok || text != "```rech\nСтопка<Целое>.положить(значение: Целое)\n```" {
		t.Fatalf("Стопка: неожиданный результат ok=%v текст=%q", ok, text)
	}

	line = lineOf(t, src, "оч.добавить(1)")
	col = runeCol(t, "    оч.добавить(1)", "добавить")
	text, ok = d.hover(line, col)
	if !ok || text != "```rech\nОчередь<Целое>.добавить(значение: Целое)\n```" {
		t.Fatalf("Очередь: неожиданный результат ok=%v текст=%q", ok, text)
	}
}

func TestHoverOnNestedListMethodCall(t *testing.T) {
	src := `
функция главная
{
    переменная сп = новый Список<Список<Целое>>()
    сп.добавить(новый Список<Целое>())
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "сп.добавить(новый")
	col := runeCol(t, "    сп.добавить(новый Список<Целое>())", "добавить")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на методе Список<Список<Целое>> (вложенный generic-аргумент)")
	}
	if text != "```rech\nСписок<Список<Целое>>.добавить(значение: Список<Целое>)\n```" {
		t.Fatalf("неожиданный текст: %q", text)
	}
}

func TestCompletionAfterDotOnListValue(t *testing.T) {
	// "сп.длина()" - синтаксически валидный вызов (не оборванная точка без
	// продолжения) - курсор сразу после точки, там, где пользователь
	// печатает имя метода, тем же приёмом, что и
	// TestCompletionAfterDotListsFieldsAndMethods (completion_test.go) -
	// оборванная точка без ничего после нарушила бы разбор ЦЕЛОЙ функции
	// (лениентный парсер восстанавливается только на границе верхнеуровневой
	// декларации, не внутри тела), d.irProg стал бы nil, и completion()
	// отказала бы ещё до diagnosticа генерик-коллекций - не то, что здесь
	// проверяется.
	src := `
функция главная
{
    переменная сп = новый Список<Целое>()
    сп.длина()
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "сп.длина()")
	col := runeCol(t, "    сп.длина()", ".") + 1
	items := d.completion(line, col)
	if len(items) == 0 {
		t.Fatalf("ожидался непустой список методов Список<Целое>")
	}
	found := map[string]bool{}
	for _, it := range items {
		if it.Kind != kindMethod {
			t.Fatalf("ожидался Kind == kindMethod у всех элементов, получено %+v", it)
		}
		found[it.Label] = true
	}
	for _, want := range []string{"добавить", "получить", "установить", "удалить", "первый", "последний", "длина", "пусто", "равен"} {
		if !found[want] {
			t.Fatalf("ожидался метод %q в автодополнении, получено %+v", want, items)
		}
	}
}

func TestSignatureHelpOnListMethodCall(t *testing.T) {
	src := `
функция главная
{
    переменная сп = новый Список<Целое>()
    сп.установить(0, 5)
}
`
	d := newDoc("test://a.rech", src)
	line := lineOf(t, src, "сп.установить(0, 5)")
	col := runeCol(t, "    сп.установить(0, 5)", "5)")
	sig, ok := d.signatureHelp(line, col)
	if !ok {
		t.Fatalf("ожидалась signature help на методе Список<Целое>")
	}
	if sig.Label != "Список<Целое>.установить(индекс: Целое, значение: Целое)" {
		t.Fatalf("неожиданный label: %q", sig.Label)
	}
	if sig.ActiveParam != 1 {
		t.Fatalf("ожидался активный параметр 1, получено %d", sig.ActiveParam)
	}
}

func TestHoverOnUserGenericTypeStillFailsGracefully(t *testing.T) {
	// Не встроенная коллекция - и не сущность вовсе (без объявления) -
	// hoverGenericCollectionMember/genericCollectionCompletion не должны
	// паниковать и не должны выдумывать методы для произвольного имени.
	if _, ok := hoverGenericCollectionMember("НеСуществующийТип<Целое>", "метод"); ok {
		t.Fatalf("не ожидался hover для несуществующего generic-имени")
	}
	if _, ok := genericCollectionCompletion("Список"); ok {
		t.Fatalf("не ожидался completion для \"Список\" без аргумента типа")
	}
}
