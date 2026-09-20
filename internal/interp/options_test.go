package interp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// runSrcWithOptionsYAML - как runSrc (interp_test.go), но с
// irProg.OptionsYAML, выставленным ПОСЛЕ semantic.Analyze, ДО Run - тем же
// порядком, что и runSrcWithDBPoolTimeoutMs (dbtimeout_test.go)/
// internal/compiler.defaultBackend.Generate (единственное реальное место
// вне тестов).
func runSrcWithOptionsYAML(t *testing.T, src, optionsYAML string) string {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		t.Fatalf("семантический анализ: %v", err)
	}
	irProg.OptionsYAML = optionsYAML
	var buf bytes.Buffer
	if err := Run(irProg, &buf, strings.NewReader("")); err != nil {
		t.Fatalf("исполнение: %v", err)
	}
	return buf.String()
}

// TestRunOptionsProjectReturnsEmbeddedDefault - "опции_проекта()" отдаёт
// ir.Program.OptionsYAML ("опции:" .rechproj) как есть, без разбора.
func TestRunOptionsProjectReturnsEmbeddedDefault(t *testing.T) {
	out := runSrcWithOptionsYAML(t, `
функция главная
{
    вывести опции_проекта()
}
`, "мой_заголовок:\n  поле: значение")
	if out != "мой_заголовок:\n  поле: значение" {
		t.Fatalf("опции_проекта() = %q, ожидалось встроенное значение по умолчанию", out)
	}
}

// TestRunOptionsProjectDefaultIsEmptyWithoutSection - нет "опции:" -
// "опции_проекта()" - пустая строка, не паника.
func TestRunOptionsProjectDefaultIsEmptyWithoutSection(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    вывести опции_проекта()
}
`)
	if out != "" {
		t.Fatalf("опции_проекта() = %q, ожидалась пустая строка", out)
	}
}

// TestRunSetOptionsProjectIsSessionOnly - "установить_опции_проекта"
// переопределяет ТОЛЬКО в памяти текущего запуска (PHP ini_set-стиль) -
// последующий "опции_проекта()" В ТОМ ЖЕ запуске видит новое значение.
func TestRunSetOptionsProjectIsSessionOnly(t *testing.T) {
	out := runSrcWithOptionsYAML(t, `
функция главная
{
    установить_опции_проекта("новое значение")
    вывести опции_проекта()
}
`, "старое значение")
	if out != "новое значение" {
		t.Fatalf("опции_проекта() после установить_опции_проекта() = %q, ожидалось \"новое значение\"", out)
	}
}
