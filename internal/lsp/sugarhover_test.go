package lsp

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// TestEverySugarMethodHasHover - у каждого метода встроенного типа-хендла
// (semantic.SugarMethods) и каждого метода "ДатаВремя" есть сигнатура для
// hover - новый метод без описания поймает этот тест.
func TestEverySugarMethodHasHover(t *testing.T) {
	recvs := []ir.Type{ir.TypeDB, ir.TypeFile, ir.TypeSocket, ir.TypeHTTP2Session, ir.EntityType("HTTP2Поток"), ir.EntityType("Процесс"), ir.TypeFloat, ir.TypeFloat32}
	d := newDoc("test://a.rech", "функция главная\n{\n}\n")
	for _, recv := range recvs {
		methods := semantic.SugarMethods(recv)
		if len(methods) == 0 {
			t.Fatalf("у %v нет методов-сахара - список типов в тесте устарел", recv)
		}
		for m := range methods {
			text, ok := d.hoverSugarMethod(recv, m)
			if !ok || !strings.Contains(text, "."+m+"(") {
				t.Errorf("%v.%s: нет hover (%q)", recv, m, text)
			}
		}
	}
	for _, m := range semantic.DateTimeMethodNames() {
		if _, ok := hoverDateTimeMethod(ir.EntityType("ДатаВремя"), m); !ok {
			t.Errorf("ДатаВремя.%s: нет описания в dateTimeMethodDocs", m)
		}
	}
}

// TestHoverOnBuiltinTypeMethodCall - hover на методе встроенного типа в
// коде: сигнатура метода без параметра-получателя и описание функции.
func TestHoverOnBuiltinTypeMethodCall(t *testing.T) {
	src := "функция главная\n{\n    переменная бд = подключиться_к_бд(\"sqlite://:memory:\")\n    бд.выполнить(\"CREATE TABLE t (a INTEGER)\")\n    переменная д = время_как_дата(0)\n    вывести д.добавить_дни(1)\n}\n"
	d := newDoc("test://a.rech", src)
	lines := strings.Split(src, "\n")
	for _, c := range []struct{ line, word, want string }{
		{"бд.выполнить", "выполнить", "БД.выполнить(sql: Строка, параметры: Список<Значение>): Целое"},
		{"д.добавить_дни", "добавить_дни", "ДатаВремя.добавить_дни(дни: Целое): ДатаВремя"},
	} {
		n := lineOf(t, src, c.line)
		text, ok := d.hover(n, runeCol(t, lines[n-1], c.word))
		if !ok || !strings.Contains(text, c.want) {
			t.Fatalf("hover %q: %q, %v", c.word, text, ok)
		}
	}
}
