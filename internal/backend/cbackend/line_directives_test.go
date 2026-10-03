package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// generateFromSrcFile - как generateFromSrc, но функции знают свой файл
// (как после module.Load) и режим сборки задан явно.
func generateFromSrcFile(t *testing.T, src, file string, release bool) string {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	for _, fn := range prog.Functions {
		fn.File = file
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		t.Fatalf("семантический анализ: %v", err)
	}
	irProg.Release = release
	out, err := GenerateFromIR(irProg)
	if err != nil {
		t.Fatalf("GenerateFromIR: %v", err)
	}
	return out
}

// Каждая C-строка инструкции закреплена за её строкой Rech, "}" после
// вложенной инструкции - за внешней, вне инструкций - номер строки самого
// .c (директива сбрасывает счёт ровно на свою следующую строку).
func TestResolveLineMarkers(t *testing.T) {
	src := strings.Join([]string{
		"int f(void) {",
		lineMarkerPush + `3 "/a.rech"`,
		"if (x) {",
		lineMarkerPush + `4 "/a.rech"`,
		"y = 1;",
		"z = 2;",
		lineMarkerPop,
		"}",
		lineMarkerPop,
		"return 0;",
		"}",
		"",
	}, "\n")
	got := resolveLineMarkers(src)
	lines := strings.Split(got, "\n")
	want := []string{
		`#line 2 "` + CFilePlaceholder + `"`,
		"int f(void) {",
		`#line 3 "/a.rech"`,
		"if (x) {",
		`#line 4 "/a.rech"`,
		"y = 1;",
		`#line 4 "/a.rech"`,
		"z = 2;",
		`#line 3 "/a.rech"`,
		"}",
		`#line 12 "` + CFilePlaceholder + `"`, // return - 12-я строка вывода
		"return 0;",
		"}",
		"",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("получено:\n%s\nожидалось:\n%s", got, strings.Join(want, "\n"))
	}
	if resolveLineMarkers("int x;\n") != "int x;\n" {
		t.Fatal("без маркеров текст не меняется")
	}
}

func TestResolveCFile(t *testing.T) {
	got := ResolveCFile(`#line 2 "`+CFilePlaceholder+`"`, "/tmp/прог.c")
	if got != `#line 2 "/tmp/прог.c"` {
		t.Fatalf("получено %s", got)
	}
}

// Отладка: #line на исходник и стек вызовов; релиз: ни того, ни другого,
// имена функций не попадают в программу.
func TestGenerateDebugAndReleaseTrace(t *testing.T) {
	src := `
функция секретная_функция(x: Целое): Целое
{
    попытаться {
        x = x / 1
    } поймать(о: Паника) {
        вывести "ой"
    }
    вернуть x
}

функция главная
{
    вывести ` + "`${секретная_функция(1)}\\n`" + `
}
`
	dbg := generateFromSrcFile(t, src, "/src/прог.rech", false)
	for _, want := range []string{`#line 4 "/src/прог.rech"`, `#line 5 "/src/прог.rech"`, `rech_trace_push("секретная_функция")`, "= rech_trace_top;"} {
		if !strings.Contains(dbg, want) {
			t.Errorf("отладка: ожидалось %q", want)
		}
	}
	rel := generateFromSrcFile(t, src, "/src/прог.rech", true)
	if !strings.Contains(rel, releaseTraceHelper) {
		t.Error("релиз: ожидались пустые макросы стека вызовов")
	}
	for _, absent := range []string{"#line", "rech_trace_top", "rech_trace[", "/src/прог.rech", lineMarkerPush} {
		if strings.Contains(rel, absent) {
			t.Errorf("релиз: не ожидалось %q", absent)
		}
	}
}
