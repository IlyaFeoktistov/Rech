package interp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// TestRunRegexInvalidPatternPanicsWithClearMessage - "регулярное" с
// шаблоном, который движок не может скомпилировать, - паника с текстом,
// включающим и сам шаблон, и диагностику движка (rtmsg.
// RegexInvalidPatternFormat) - см. package doc internal/backend/cbackend/
// regex.go для той же проверки на cbackend (там текст диагностики
// расходится - другой движок, - но сам факт паники одинаков).
func TestRunRegexInvalidPatternPanicsWithClearMessage(t *testing.T) {
	src := `
функция главная
{
    переменная м = регулярное("текст", "(abc")
}
`
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), `регулярное: некорректный шаблон "(abc"`) {
		t.Fatalf("ожидалась паника с текстом про некорректный шаблон, получено: %v", err)
	}
}

// TestRegexTranslatePatternRewritesWordClassToUnicode - "\w"/"\W" должны
// стать Unicode-осознанными классами ДО regexp.Compile - у Go regexp/RE2
// голый "\w" ASCII-only, не видит кириллицу (см. package doc regex.go).
func TestRegexTranslatePatternRewritesWordClassToUnicode(t *testing.T) {
	got := regexTranslatePattern(`прив\w+ \W \d\D`)
	want := `прив[\p{L}\p{N}_]+ [^\p{L}\p{N}_] \d\D`
	if got != want {
		t.Fatalf("regexTranslatePattern(...) = %q, хотели %q", got, want)
	}
}

// TestRunRegexFindAllReturnsGroupsAndPositions - сквозная проверка через
// настоящий "регулярное" (не regexFindAll напрямую) - позиции в КОДОВЫХ
// ТОЧКАХ Unicode (кириллица перед совпадением сдвигает "начало" на число
// букв, не байт), группы без совпадения (необязательная часть шаблона) -
// пустая строка, не Ничто.
func TestRunRegexFindAllReturnsGroupsAndPositions(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная м = регулярное("привет, a@b.com!", "(\\w+)(@x)?")
    вывести м.длина()
    вывести м.получить(1).текст
    вывести м.получить(1).начало
    вывести м.получить(1).группы.получить(0)
    вывести м.получить(1).группы.получить(1)
}
`)
	want := "4\na\n8\na\n\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}
