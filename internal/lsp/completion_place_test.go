package lsp

import (
	"sort"
	"strings"
	"testing"
)

const placeSrc = `интерфейс Фигура
{
    площадь(): Целое
}

сущность База
{
    публичный описать(): Строка
    {
        вернуть "база"
    }
}

сущность Точка наследует Базу реализует Фигуру
{
    имеет:
        публичное Целое x

    публичный сдвинуть(на: Целое): Целое
    {
        Целое сдвиг = на
        вернуть сдвиг
    }
%ENTITY%
}

функция применить(ф: Вызываемый(Точка): Целое, т: Точка): Целое
{
    вернуть ф(т)
}

функция главная
{
    Точка т = новая Точка()
    Фигура фиг = т
%MAIN%
}
%TOP%
`

// placeLabels - автодополнение в конце строки cursor, подставленной вместо
// одной из меток placeSrc; tail - строки после неё (закрывающие скобки).
func placeLabels(t *testing.T, mark, cursor, tail string) map[string]bool {
	t.Helper()
	src := placeSrc
	for _, m := range []string{"%ENTITY%", "%MAIN%", "%TOP%"} {
		repl := ""
		if m == mark {
			repl = cursor
			if tail != "" {
				repl += "\n" + tail
			}
		}
		src = strings.Replace(src, m, repl, 1)
	}
	parts := strings.Split(cursor, "\n")
	last := parts[len(parts)-1]
	line := 0
	for i, l := range strings.Split(src, "\n") {
		if l == last {
			line = i + 1
		}
	}
	if line == 0 {
		t.Fatalf("строка %q не найдена", last)
	}
	d := newDoc("test://a.rech", src)
	out := map[string]bool{}
	for _, it := range d.completion(line, len([]rune(last))+1) {
		out[it.Label] = true
	}
	return out
}

func sortedKeys(m map[string]bool) string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, " ")
}

// TestCompletionMemberDeclInEntity - в теле сущности вне методов: модификаторы,
// конструктор, методы базы и интерфейса для переопределения - без
// переменных соседнего метода и без функций.
func TestCompletionMemberDeclInEntity(t *testing.T) {
	got := placeLabels(t, "%ENTITY%", "    пуб", "")
	for _, want := range []string{"публичный", "конструктор", "имеет", "описать", "площадь"} {
		if !got[want] {
			t.Errorf("нет %q: %s", want, sortedKeys(got))
		}
	}
	for _, unwanted := range []string{"сдвиг", "на", "т", "применить", "если", "открыть"} {
		if got[unwanted] {
			t.Errorf("%q в объявлении члена сущности не нужен", unwanted)
		}
	}
}

// TestCompletionNewNameGivesNothing - там, где набирается новое имя,
// подсказок нет.
func TestCompletionNewNameGivesNothing(t *testing.T) {
	cases := []struct{ mark, cursor string }{
		{"%TOP%", "функция сло"},
		{"%TOP%", "сущность Кру"},
		{"%MAIN%", "    Целое сч"},
		{"%MAIN%", "    Список<Точка> то"},
		{"%MAIN%", "    переменная и"},
		{"%MAIN%", "    для и"},
		{"%TOP%", "функция сложить(а"},
	}
	for _, c := range cases {
		if got := placeLabels(t, c.mark, c.cursor, ""); len(got) != 0 {
			t.Errorf("%q: ожидалось пусто, получено %s", c.cursor, sortedKeys(got))
		}
	}
}

// TestCompletionAfterNewOffersTypes - после "новый" - только создаваемые типы.
func TestCompletionAfterNewOffersTypes(t *testing.T) {
	got := placeLabels(t, "%MAIN%", "    Точка другая = новая То", "")
	if !got["Точка"] || !got["Список"] || got["т"] || got["применить"] || got["Фигура"] {
		t.Fatalf("после \"новая\" ожидались только создаваемые типы: %s", sortedKeys(got))
	}
}

// TestCompletionAfterBrace - "} " - продолжение "если"/"попытаться".
func TestCompletionAfterBrace(t *testing.T) {
	got := placeLabels(t, "%MAIN%", "    если истина {\n    } ина", "    }")
	if sortedKeys(got) != "иначе окончательно поймать" {
		t.Fatalf("после \"}\" ожидались иначе/поймать/окончательно: %s", sortedKeys(got))
	}
}

// TestCompletionExpressionWithoutStatements - внутри выражения нет
// ключевых слов инструкций.
func TestCompletionExpressionWithoutStatements(t *testing.T) {
	got := placeLabels(t, "%MAIN%", "    Целое р = т", "")
	if got["если"] || got["пока"] || got["вернуть"] || !got["т"] || !got["истина"] {
		t.Fatalf("выражение: %s", sortedKeys(got))
	}
}

// TestCompletionTopLevelWithExplicitMain - на верхнем уровне при явной
// "главная" - только объявления.
func TestCompletionTopLevelWithExplicitMain(t *testing.T) {
	got := placeLabels(t, "%TOP%", "фу", "")
	if !got["функция"] || !got["сущность"] || got["т"] || got["если"] || got["открыть"] {
		t.Fatalf("верхний уровень: %s", sortedKeys(got))
	}
}

// TestCompletionLambdaScope - параметр лямбды виден в её теле, в том числе
// после точки, и переменные внешней функции тоже.
func TestCompletionLambdaScope(t *testing.T) {
	got := placeLabels(t, "%MAIN%", "    Целое р = применить(функция(п: Точка): Целое {\n        вернуть п", "    }, т)")
	if !got["п"] || !got["т"] || !got["фиг"] {
		t.Fatalf("в лямбде ожидались п, т, фиг: %s", sortedKeys(got))
	}
	got = placeLabels(t, "%MAIN%", "    Целое р = применить(функция(п: Точка): Целое {\n        вернуть п.", "    }, т)")
	if !got["x"] || !got["сдвинуть"] {
		t.Fatalf("члены параметра лямбды: %s", sortedKeys(got))
	}
}

// TestCompletionInterfaceVariableMembers - у переменной с типом-интерфейсом -
// только методы интерфейса.
func TestCompletionInterfaceVariableMembers(t *testing.T) {
	got := placeLabels(t, "%MAIN%", "    фиг.", "")
	if sortedKeys(got) != "площадь" {
		t.Fatalf("фиг.: %s", sortedKeys(got))
	}
}

// TestCompletionFieldSectionSuggestsTypes - в "имеет:" поле начинается с
// типа ("[модификаторы] Тип имя"): типы в подсказках есть.
func TestCompletionFieldSectionSuggestsTypes(t *testing.T) {
	got := placeLabels(t, "%ENTITY%", "    имеет:\n        приватная Ст", "")
	for _, want := range []string{"Строка", "Точка", "Целое"} {
		if !got[want] {
			t.Errorf("нет %q: %s", want, sortedKeys(got))
		}
	}
}
