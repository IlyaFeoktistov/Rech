package lsp

import (
	"strings"
	"testing"
)

const builtinHoverTestSrc = `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    если истина
    {
        вывести корень(4.0)
    }
    закрыть_бд(бд)
}
`

func TestHoverOnKeywordShowsDoc(t *testing.T) {
	d := newDoc("test://a.rech", builtinHoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, builtinHoverTestSrc, "если истина")
	col := runeCol(t, "    если истина", "если")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на ключевом слове \"если\"")
	}
	if !strings.Contains(text, "условная инструкция") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
}

func TestHoverOnBuiltinTypeShowsDoc(t *testing.T) {
	src := "функция ф(параметр: БД) { вывести 1 }\nфункция главная { вывести 1 }\n"
	d := newDoc("test://a.rech", src)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, src, "параметр: БД")
	col := runeCol(t, "функция ф(параметр: БД) { вывести 1 }", "БД")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на встроенном типе \"БД\"")
	}
	if !strings.Contains(text, "SQLite/PostgreSQL/MySQL") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
}

// TestHoverOnReservedEntityShowsRealConstructor - зарезервированные
// сущности ("Совпадение"/"HTTP2Поток"/"ДекодированноеИзображение" и т. п.)
// регистрируются лениво (internal/semantic/analyzer.go) - в документе, где
// их имя ещё НИ РАЗУ не встретилось (d.entities их не содержит), hover для
// голого имени падает в статический fallback builtinTypeDocs - без
// reservedEntityFields (builtindocs.go) там не было бы конструктора вовсе.
// Пользователь попросил явно показывать реальные параметры конструктора и
// для зарезервированных сущностей тоже, не только для пользовательских.
func TestHoverOnReservedEntityShowsRealConstructor(t *testing.T) {
	d := newDoc("test://a.rech", "функция главная { вывести 1 }\n")
	if _, ok := d.entities["Совпадение"]; ok {
		t.Fatalf("\"Совпадение\" не должна быть в d.entities в этом документе - тест проверяет именно случай ДО регистрации")
	}
	text, ok := d.hoverBareName("Совпадение", 1)
	if !ok {
		t.Fatalf("ожидался hover на \"Совпадение\"")
	}
	if !strings.Contains(text, "новый Совпадение(текст: Строка, начало: Целое, группы: Список<Строка>)") {
		t.Fatalf("ожидалась сигнатура конструктора с реальными полями, получено: %q", text)
	}
}

func TestHoverOnBuiltinFuncFromAllShowsSignature(t *testing.T) {
	d := newDoc("test://a.rech", builtinHoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, builtinHoverTestSrc, "корень")
	col := runeCol(t, "        вывести корень(4.0)", "корень")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на встроенной функции \"корень\"")
	}
	if !strings.Contains(text, "корень(Действительное): Действительное") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
	if !strings.Contains(text, "квадратный корень") {
		t.Fatalf("неожиданное описание: %q", text)
	}
}

func TestHoverOnReservedBuiltinFuncShowsSignature(t *testing.T) {
	d := newDoc("test://a.rech", builtinHoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, builtinHoverTestSrc, "подключиться_к_бд")
	col := runeCol(t, "    переменная бд = подключиться_к_бд(\"sqlite::memory:\")", "подключиться_к_бд")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover на встроенной функции \"подключиться_к_бд\"")
	}
	if !strings.Contains(text, "подключиться_к_бд(строка_подключения: Строка): БД") {
		t.Fatalf("неожиданный текст hover: %q", text)
	}
}
