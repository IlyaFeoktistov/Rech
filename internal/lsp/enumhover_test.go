package lsp

import (
	"strings"
	"testing"
)

const enumHoverTestSrc = `
перечисление Цвет
{
    Красный,
    Зелёный,
    Синий = 10
}

функция главная
{
    Целое значение = Цвет.Красный
    вывести значение
}
`

// TestHoverOnBuiltinEnumMemberShowsValue - "HTTPКод.ОК" (встроенное
// перечисление, semantic.StandardEnums, не объявлено ни в одном файле)
// должно показывать значение 200.
func TestHoverOnBuiltinEnumMemberShowsValue(t *testing.T) {
	d := newDoc("test://a.rech", enumHoverTestSrc)
	if d.irProg == nil {
		t.Fatalf("ожидался успешный разбор")
	}
	// HTTPКод не объявлено в тексте документа - строка/столбец не важны
	// для резолюции самого имени, но должны указывать на существующий
	// токен, значит подставим прямо в исходник курсором на "к" и проверим
	// hoverEnumMember напрямую вместо конструирования курсора на
	// несуществующий текст.
	text, ok := d.hoverEnumMember("HTTPКод", "ОК")
	if !ok {
		t.Fatalf("ожидался hover \"HTTPКод.ОК\"")
	}
	if !strings.Contains(text, "200") {
		t.Fatalf("ожидалось значение 200, получено %q", text)
	}
}

// TestHoverOnUserEnumMemberShowsValue - курсор на "Красный" в "Цвет.Красный"
// (перечисление, объявленное в этом же документе) должен показать 0.
func TestHoverOnUserEnumMemberShowsValue(t *testing.T) {
	d := newDoc("test://a.rech", enumHoverTestSrc)
	line := lineOf(t, enumHoverTestSrc, "Цвет.Красный")
	col := runeCol(t, "    Целое значение = Цвет.Красный", "Красный")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover \"Цвет.Красный\"")
	}
	if !strings.Contains(text, "Цвет.Красный: Целое = 0") {
		t.Fatalf("ожидалось значение 0, получено %q", text)
	}
}

// TestHoverOnUserEnumMemberWithExplicitValue - "Синий = 10" - должен
// показать именно 10, не позиционное значение по умолчанию.
func TestHoverOnUserEnumMemberWithExplicitValue(t *testing.T) {
	d := newDoc("test://a.rech", enumHoverTestSrc)
	text, ok := d.hoverEnumMember("Цвет", "Синий")
	if !ok {
		t.Fatalf("ожидался hover \"Цвет.Синий\"")
	}
	if !strings.Contains(text, "= 10") {
		t.Fatalf("ожидалось значение 10, получено %q", text)
	}
}

// TestHoverOnEnumTypeNameListsMembers - курсор на самом имени "Цвет"
// (не на члене) должен перечислить все члены со значениями.
func TestHoverOnEnumTypeNameListsMembers(t *testing.T) {
	d := newDoc("test://a.rech", enumHoverTestSrc)
	line := lineOf(t, enumHoverTestSrc, "перечисление Цвет")
	col := runeCol(t, "перечисление Цвет", "Цвет")
	text, ok := d.hover(line, col)
	if !ok {
		t.Fatalf("ожидался hover типа \"Цвет\"")
	}
	for _, want := range []string{"Красный = 0", "Зелёный = 1", "Синий = 10"} {
		if !strings.Contains(text, want) {
			t.Fatalf("ожидалось %q в тексте, получено:\n%s", want, text)
		}
	}
}
