package lsp

import (
	"strings"
	"testing"
)

const typeRefsSrc = `/** Что-то с площадью. */
интерфейс Фигура
{
    площадь(): Действительное
}

сущность Птица
{
    имеет:
        Строка имя
}

сущность Утка наследует Птицу реализует Фигуру
{
    площадь(): Действительное
    {
        вернуть 1.0
    }
}

функция измерить(ф: Фигура): Действительное
{
    вернуть ф.площадь()
}

функция главная
{
    Список<Фигура> все = [новая Утка()]
    Птица п = новая Утка()
    вывести измерить(все.получить(0))
}
`

// TestTypeRefsCodeLensAndDefinition - "N ссылок" у интерфейса и сущности
// считает все использования типа (реализует/наследует в склонённой форме,
// тип параметра, аргумент generic, тип переменной, "новый"), переход к
// определению работает из любого из них, в том числе со словоформы.
func TestTypeRefsCodeLensAndDefinition(t *testing.T) {
	d := newDoc("test://a.rech", typeRefsSrc)
	if len(d.diagnostics) != 0 {
		t.Fatalf("неожиданные диагностики %v", d.diagnostics)
	}
	titles := map[int]string{}
	for _, l := range d.codeLenses() {
		titles[l.Line] = l.Title
	}
	if titles[2] != "3 ссылки" {
		t.Fatalf("интерфейс Фигура: %q, ожидалось \"3 ссылки\" (реализует, параметр, Список<>)", titles[2])
	}
	if titles[7] != "2 ссылки" {
		t.Fatalf("сущность Птица: %q, ожидалось \"2 ссылки\" (наследует, тип переменной)", titles[7])
	}
	if titles[13] != "2 ссылки" {
		t.Fatalf("сущность Утка: %q, ожидалось \"2 ссылки\" (новая x2)", titles[13])
	}
	lines := strings.Split(typeRefsSrc, "\n")
	for _, c := range []struct {
		line, word string
		want       int
	}{
		{"наследует Птицу", "Фигуру", 2},
		{"наследует Птицу", "Птицу", 7},
		{"функция измерить", "Фигура", 2},
		{"Список<Фигура>", "Фигура", 2},
	} {
		n := lineOf(t, typeRefsSrc, c.line)
		col := runeCol(t, lines[n-1], c.word)
		if loc, ok := d.definition(n, col); !ok || loc.Line != c.want {
			t.Fatalf("определение %q на строке %d: %+v, %v", c.word, n, loc, ok)
		}
		if occ, ok := d.occurrences(n, col); !ok || len(occ) < 3 {
			t.Fatalf("вхождения %q: %v, %v", c.word, occ, ok)
		}
	}
}
