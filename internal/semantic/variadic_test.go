package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

const variadicSumDecl = `
функция сумма(база: Целое, ...числа: Целое): Целое
{
    вернуть база
}
`

func TestVariadicRejectsMisplacedSpreadAndBadTypes(t *testing.T) {
	cases := []struct{ body, want string }{
		{"сумма()", "минимум 1"},
		{"Список<Целое> набор = [1]\n    сумма(...набор)", "с аргумента 2"},
		{"сумма(1, \"а\")", "аргумент 2"},
		{"Список<Строка> набор = [\"а\"]\n    сумма(1, ...набор)", "должно раскрывать"},
		{"сумма(1, ...5)", "должно раскрывать"},
		{"Список<Целое> набор = [1]\n    спать_мс(...набор)", "\"...\" допустимо только"},
		{"переменная набор = [...5]", "раскрывает только Список"},
	}
	for _, c := range cases {
		src := variadicSumDecl + "\nфункция главная\n{\n    " + c.body + "\n}\n"
		_, err := AnalyzeSource(t, src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: ожидалась ошибка с %q, получено %v", c.body, c.want, err)
		}
	}
}

func TestVariadicSpreadOnNonVariadicCall(t *testing.T) {
	src := `
функция один(х: Целое): Целое
{
    вернуть х
}

функция главная
{
    Список<Целое> набор = [1]
    один(...набор)
}
`
	_, err := AnalyzeSource(t, src)
	if err == nil || !strings.Contains(err.Error(), "не вариативна") {
		t.Fatalf("ожидалась ошибка спреда в невариативный вызов, получено %v", err)
	}
}

func TestVariadicRejectsOverloadAndOverrideMismatch(t *testing.T) {
	overload := `
функция ф(...х: Целое)
{
}

функция ф(х: Строка)
{
}

функция главная
{
}
`
	if _, err := AnalyzeSource(t, overload); err == nil || !strings.Contains(err.Error(), "нельзя перегружать") {
		t.Fatalf("ожидалась ошибка перегрузки вариативной функции, получено %v", err)
	}
	override := `
сущность База
{
    публичный ф(...х: Целое)
    {
    }
}

сущность Потомок наследует Базу
{
    публичный ф(х: Список<Целое>)
    {
    }
}

функция главная
{
}
`
	if _, err := AnalyzeSource(t, override); err == nil || !strings.Contains(err.Error(), "не совпадает по сигнатуре") {
		t.Fatalf("переопределение без \"...\" должно отличаться по сигнатуре, получено %v", err)
	}
}

func TestVariadicParserRejections(t *testing.T) {
	cases := []struct{ src, want string }{
		{"функция ф(...х: Целое, у: Целое)\n{\n}\n", "должен быть последним"},
		{"функция ф(у: Целое = 1, ...х: Целое)\n{\n}\n", "нельзя сочетать"},
		{"функция главная\n{\n    переменная л = функция (...х: Целое) { }\n}\n", "лямбды"},
		{"сущность Т\n{\n    конструктор(...х: Целое)\n    {\n    }\n}\n", "конструктора"},
		{"внешняя функция puts(...х: Строка): Целое32\n", "внешней функции"},
		{"функция накормить(...кого: Строка)\n{\n}\n\nфункция главная\n{\n    переменная кошка = \"Мурка\"\n    накормить кошку\n}\n", "только строгим синтаксисом"},
	}
	for _, c := range cases {
		toks, err := lexer.New(c.src).Tokenize()
		if err != nil {
			t.Fatalf("лексер: %v", err)
		}
		_, err = parser.New(toks).ParseProgram()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: ожидалась ошибка с %q, получено %v", c.src, c.want, err)
		}
	}
}
