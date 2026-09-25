package lsp

import (
	"sort"
	"strings"
	"testing"
)

const probeSrc = `интерфейс Фигура
{
    площадь(): Целое
}

сущность Точка реализует Фигуру
{
    имеет:
        публичный x: Целое
        публичное имя: Строка

    публичный площадь(): Целое
    {
        вернуть этот.x
    }

    публичный сдвинуть(на: Целое, другая: Точка): Целое
    {
%METHOD%
        вернуть на
    }
}

функция применить(ф: Вызываемый(Точка): Целое, т: Точка): Целое
{
    вернуть ф(т)
}

функция главная
{
    Точка т = новая Точка()
    Список<Точка> точки = [т]
    Словарь<Строка, Точка> по_имени = новый Словарь<Строка, Точка>()
    Фигура фиг = т
    Целое внешнее = 5
%MAIN%
}
`

func probe(t *testing.T, method, main string) []string {
	t.Helper()
	target := main
	if method != "" {
		target = method
	}
	tail := ""
	if i := strings.Index(target, "\x00"); i >= 0 {
		target, tail = target[:i], "\n"+target[i+1:]
	}
	if method != "" {
		method = target + tail
	} else {
		main = target + tail
	}
	src := strings.Replace(strings.Replace(probeSrc, "%METHOD%", method, 1), "%MAIN%", main, 1)
	parts := strings.Split(target, "\n")
	last := parts[len(parts)-1]
	line := 0
	for i, l := range strings.Split(src, "\n") {
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

// TestCompletionScopesAndReceivers - члены получателя и видимые имена в
// лямбде, методе, цикле, "поймать", у интерфейса/словаря/элемента списка и
// внутри незакрытого вызова.
func TestCompletionScopesAndReceivers(t *testing.T) {
	const точка = "x имя площадь сдвинуть"
	members := []struct{ name, method, main, want string }{
		{"параметр лямбды", "", "    Целое р = применить(функция(п: Точка): Целое {\n        вернуть п.\x00    }, т)", точка},
		{"параметр метода", "        другая.", "", точка},
		{"этот", "        этот.", "", точка},
		{"элемент для", "", "    для точка в точках {\n        точка.\x00    }", точка},
		{"переменная поймать", "", "    попытаться {\n        вывести 1\n    } поймать(ош: Исключение) {\n        ош.\x00    }", "код сообщение"},
		{"интерфейс", "", "    фиг.", "площадь"},
		{"словарь", "", "    по_имени.", "длина получить пусто равен содержит удалить установить"},
		{"элемент списка", "", "    точки.получить(0).", точка},
		{"аргумент вызова", "", "    вывести(т.", точка},
	}
	for _, c := range members {
		if got := strings.Join(probe(t, c.method, c.main), " "); got != c.want {
			t.Errorf("%s: ожидалось [%s], получено [%s]", c.name, c.want, got)
		}
	}
	names := []struct {
		name, method, main string
		want               []string
	}{
		{"в лямбде", "", "    Целое р = применить(функция(п: Точка): Целое {\n        вернуть вн\x00    }, т)", []string{"п", "внешнее", "т", "точки", "применить"}},
		{"в методе", "        др", "", []string{"на", "другая", "этот", "применить"}},
		{"в цикле", "", "    пока истина {\n        вн\x00    }", []string{"внешнее", "т", "точки"}},
		{"в незакрытом вызове", "", "    применить(вн", []string{"внешнее", "т", "точки"}},
	}
	for _, c := range names {
		has := map[string]bool{}
		for _, l := range probe(t, c.method, c.main) {
			has[l] = true
		}
		for _, w := range c.want {
			if !has[w] {
				t.Errorf("%s: нет %q", c.name, w)
			}
		}
	}
}
