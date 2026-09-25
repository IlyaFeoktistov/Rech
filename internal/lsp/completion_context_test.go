package lsp

import (
	"sort"
	"strings"
	"testing"
)

const completionCtxBase = `перечисление Цвет
{
    КРАСНЫЙ,
    ЗЕЛЁНЫЙ,
}

сущность Фигура
{
    имеет:
        публичное имя: Строка
        приватный секрет: Целое

    константа СТОРОН = 0

    публичный площадь(): Целое
    {
        вернуть этот.секрет
    }

    статический создать(): Фигура
    {
        вернуть новая Фигура()
    }
}

сущность Точка наследует Фигуру
{
    имеет:
        публичный x: Целое

    публичный длина(): Целое
    {
        вернуть этот.x
    }

    публичный копия(): Точка
    {
        вернуть этот
    }
}

функция главная
{
    Точка т = новая Точка()
    ?Точка может = Ничто
    Файл ф = открыть("a")
    Действительное д = 1.5
    Список<Целое> сп = []
%s
}
`

// completionAt - автодополнение в конце строки cursorLine (подставляется
// вместо %s в completionCtxBase, может быть многострочной - курсор в конце
// последней строки).
func completionAt(t *testing.T, cursorLine string) []string {
	t.Helper()
	src := strings.Replace(completionCtxBase, "%s", cursorLine, 1)
	lines := strings.Split(src, "\n")
	parts := strings.Split(cursorLine, "\n")
	last := parts[len(parts)-1]
	line := 0
	for i, l := range lines {
		if l == last {
			line = i + 1
		}
	}
	d := newDoc("test://a.rech", src)
	var labels []string
	for _, it := range d.completion(line, len([]rune(last))+1) {
		labels = append(labels, it.Label)
	}
	sort.Strings(labels)
	return labels
}

func TestCompletionMembersWhileTyping(t *testing.T) {
	cases := map[string]string{
		"    т.":                   "x СТОРОН длина имя копия площадь",
		"    т.дл":                 "x СТОРОН длина имя копия площадь",
		"    переменная к = т.":    "x СТОРОН длина имя копия площадь",
		"    вывести `${т.":        "x СТОРОН длина имя копия площадь",
		"    т.копия().":           "x СТОРОН длина имя копия площадь",
		"    может.":               "x СТОРОН длина имя копия площадь",
		"    ф.":                   "закрыть записать записать_байты можно_прочитать_байт прочитать прочитать_байты",
		"    д.":                   "бесконечно конечно не_число",
		"    сп.":                  "длина добавить первый получить последний пусто равен удалить установить",
		"    Цвет.":                "ЗЕЛЁНЫЙ КРАСНЫЙ",
		"    Фигура.":              "СТОРОН",
		"    Фигура::":             "СТОРОН создать",
		"    Действительное::":     "БЕСКОНЕЧНОСТЬ МАКСИМУМ МИНИМУМ НЕ_ЧИСЛО ШИРИНА",
		"    Целое8 м = Целое8::М": "МАКСИМУМ МИНИМУМ ШИРИНА",
	}
	for line, want := range cases {
		if got := strings.Join(completionAt(t, line), " "); got != want {
			t.Errorf("%q: ожидалось [%s], получено [%s]", line, want, got)
		}
	}
}

func TestCompletionPrivateMemberVisibleOnlyInsideOwner(t *testing.T) {
	src := strings.Replace(completionCtxBase, "    вернуть этот.секрет", "    вернуть этот.", 1)
	src = strings.Replace(src, "%s", "", 1)
	line := lineOf(t, src, "    вернуть этот.")
	d := newDoc("test://a.rech", src)
	var labels []string
	for _, it := range d.completion(line, len([]rune("        вернуть этот."))+1) {
		labels = append(labels, it.Label)
	}
	sort.Strings(labels)
	if got := strings.Join(labels, " "); got != "СТОРОН имя площадь секрет" {
		t.Fatalf("внутри Фигура ожидались и приватные члены, получено [%s]", got)
	}
}

func TestCompletionGeneralWhileTyping(t *testing.T) {
	labels := completionAt(t, "    выв")
	has := map[string]bool{}
	for _, l := range labels {
		has[l] = true
	}
	for _, want := range []string{"т", "может", "ф", "д", "сп", "Точка", "Фигура", "Цвет", "открыть", "подключиться_к_бд", "Целое8", "Список", "ОшибкаФайла", "вывести", "выбрать", "прервать"} {
		if !has[want] {
			t.Errorf("в общем списке нет %q", want)
		}
	}
	for _, unwanted := range []string{"этот", "сущность", "интерфейс", "окончательно", "главная"} {
		if has[unwanted] {
			t.Errorf("%q в начале инструкции свободной функции предлагать не нужно", unwanted)
		}
	}
}

func TestCompletionTypeAnnotationOffersTypes(t *testing.T) {
	src := "функция ф(х: )\n{\n}\n\nфункция главная\n{\n}\n"
	d := newDoc("test://a.rech", src)
	has := map[string]bool{}
	for _, it := range d.completion(1, len([]rune("функция ф(х: "))+1) {
		has[it.Label] = true
	}
	for _, want := range []string{"Целое", "Строка", "Список", "ОшибкаФайла"} {
		if !has[want] {
			t.Errorf("после \"х:\" нет типа %q", want)
		}
	}
	for _, unwanted := range []string{"вывести", "открыть", "главная"} {
		if has[unwanted] {
			t.Errorf("после \"х:\" не тип %q", unwanted)
		}
	}
}

func TestCompletionTernaryColonIsNotType(t *testing.T) {
	labels := completionAt(t, "    Целое к = д > 1.0 ? 1 : ")
	has := map[string]bool{}
	for _, l := range labels {
		has[l] = true
	}
	if !has["д"] || !has["открыть"] {
		t.Fatalf("после \":\" тернарного оператора ожидался общий список, получено %v", labels)
	}
}
