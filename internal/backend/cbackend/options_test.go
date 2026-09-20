package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// TestGenerateFromIROptionsBuiltinCall - "опции_проекта"/
// "установить_опции_проекта" (internal/builtins) - runtime/options.Helpers
// эмитируется, только если программа реально их вызывает (ctx.usesOptions).
func TestGenerateFromIROptionsBuiltinCall(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    вывести опции_проекта()
    установить_опции_проекта("новое значение")
}
`)
	for _, want := range []string{"rech_options(", "rech_set_options(", "rech_options_mutex", "PTHREAD_MUTEX_INITIALIZER"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWithoutOptionsOmitsHelper(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    вывести 1
}
`)
	if strings.Contains(out, "rech_options") {
		t.Fatalf("не ожидался rech_options без использования \"опции_проекта\"/\"установить_опции_проекта\", получено:\n%s", out)
	}
}

// TestGenerateFromIROptionsEmbedsDefaultFromProgram - ir.Program.OptionsYAML
// ("опции:" .rechproj, internal/project.File.Опции) встраивается как
// значение по умолчанию, экранированное в C-строковый литерал - тем же
// порядком (выставлено ПОСЛЕ semantic.Analyze, ДО GenerateFromIR), что и
// generateFromSrcWithDBPoolTimeoutMs (dbtimeout_test.go).
func TestGenerateFromIROptionsEmbedsDefaultFromProgram(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести опции_проекта()
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	astProg, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	irProg, err := semantic.Analyze(astProg)
	if err != nil {
		t.Fatalf("семантический анализ: %v", err)
	}
	irProg.OptionsYAML = "мой_заголовок:\n  поле: \"значение\""
	out, err := GenerateFromIR(irProg)
	if err != nil {
		t.Fatalf("GenerateFromIR: %v", err)
	}
	if !strings.Contains(out, `"мой_заголовок:\n  поле: \"значение\""`) {
		t.Fatalf("ожидался экранированный литерал OptionsYAML, получено:\n%s", out)
	}
}
