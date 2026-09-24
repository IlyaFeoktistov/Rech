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
	if !strings.Contains(text, "новое Совпадение(текст: Строка, начало: Целое, группы: Список<Строка>)") {
		t.Fatalf("ожидалась сигнатура конструктора с реальными полями, получено: %q", text)
	}
}

// TestHoverOnReservedBuiltinFuncDescriptionOutsideCodeBlock - НАЙДЕННЫЙ БАГ
// (репортнуто пользователем - "для" подсвечивается как ключевое слово в
// описании "строка_куки"): hoverBuiltinFunc заворачивал ВЕСЬ текст
// (сигнатура + описание после " — ") в ОДИН ```rech код-блок - редактор
// подсвечивал обычную русскую прозу описания как код Rech, и любое слово,
// совпавшее с ключевым словом языка ("для", "если", "новый", "этот" и
// т. п.), подсвечивалось наравне с настоящим кодом. Описание должно быть
// СНАРУЖИ код-блока (обычный markdown-текст), как и у builtins.All-ветки
// чуть выше (builtinFuncShortDoc).
func TestHoverOnReservedBuiltinFuncDescriptionOutsideCodeBlock(t *testing.T) {
	text, ok := hoverBuiltinFunc("строка_куки")
	if !ok {
		t.Fatalf("ожидался hover на \"строка_куки\"")
	}
	closeFence := strings.Index(text, "```\n")
	if closeFence < 0 {
		t.Fatalf("ожидался закрывающий ```-код-блок: %q", text)
	}
	sigBlock, rest := text[:closeFence], text[closeFence+len("```\n"):]
	if strings.Contains(sigBlock, "для") {
		t.Fatalf("описание не должно попадать внутрь код-блока сигнатуры: %q", sigBlock)
	}
	if !strings.Contains(rest, "строка для заголовка") {
		t.Fatalf("описание должно остаться СНАРУЖИ код-блока: %q", rest)
	}
}

// TestHoverOnHTTPRequestEntityShowsFieldDefaults - "Запрос" (как и "Ответ"/
// "ДатаВремя") - ВСЕГДА зарегистрированная сущность (internal/semantic/
// analyzer.go), не лениво, значит попадает в ветку d.entities[name], не в
// reservedEntityFields/builtinTypeDocs выше - hover для неё строил сигнатуру
// конструктора через entityCtorSignature (hover.go), которая раньше не
// подставляла значения по умолчанию полей вовсе (в отличие от signature
// help на "новый Запрос(..." - callSignatureAt, signaturehelp.go, УЖЕ их
// показывала через appendParamDefaults/ctorParamDefaults) - НАЙДЕННЫЙ БАГ
// (репортнуто пользователем - "не вижу в ховере дефолтные значения хотя бы
// для Запроса"): одни и те же данные (ir.Field.Default), но hover и
// signature help собирали строку сигнатуры двумя разными путями, и только
// один из них подключал appendParamDefaults.
func TestHoverOnHTTPRequestEntityShowsFieldDefaults(t *testing.T) {
	d := newDoc("test://a.rech", "функция главная { вывести 1 }\n")
	text, ok := d.hoverBareName("Запрос", 1)
	if !ok {
		t.Fatalf("ожидался hover на \"Запрос\"")
	}
	if !strings.Contains(text, `метод: Строка = "GET"`) {
		t.Fatalf("ожидалось значение по умолчанию поля \"метод\", получено: %q", text)
	}
	if !strings.Contains(text, `путь: Строка = "/"`) {
		t.Fatalf("ожидалось значение по умолчанию поля \"путь\", получено: %q", text)
	}
	if !strings.Contains(text, "порт: Целое = 80") {
		t.Fatalf("ожидалось значение по умолчанию поля \"порт\", получено: %q", text)
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

// TestHoverShowsCallableTypeAsWritten - тип "Вызываемый" в hover - как
// в исходнике, не каноническое внутреннее имя.
func TestHoverShowsCallableTypeAsWritten(t *testing.T) {
	src := "функция главная\n{\n    переменная г: ?Вызываемый(Целое): Строка = Ничто\n    переменная х = г\n}\n"
	d := newDoc("test://a.rech", src)
	h, ok := d.hover(4, 20)
	if !ok || !strings.Contains(h, "?Вызываемый(Целое): Строка") {
		t.Fatalf("ожидался тип в записи исходника, получено %q", h)
	}
}
