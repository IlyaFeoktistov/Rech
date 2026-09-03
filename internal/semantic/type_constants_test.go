package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
)

func TestAnalyzeFixedWidthIntTypeConstants(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = Целое8::МАКСИМУМ
    переменная б = Целое8::МИНИМУМ
    переменная ц = Целое8::ШИРИНА
    переменная г = Натуральное64::МАКСИМУМ
}
`)
	main := irProg.Functions[0]
	a := main.Body[0].(*ir.VarDecl).Value.(*ir.CastExpr)
	if a.To != ir.TypeInt8 || a.Value.(*ir.IntLiteral).Value != 127 {
		t.Fatalf("Целое8::МАКСИМУМ: получено %#v", a)
	}
	b := main.Body[1].(*ir.VarDecl).Value.(*ir.CastExpr)
	if b.To != ir.TypeInt8 || b.Value.(*ir.IntLiteral).Value != -128 {
		t.Fatalf("Целое8::МИНИМУМ: получено %#v", b)
	}
	c := main.Body[2].(*ir.VarDecl).Value.(*ir.IntLiteral)
	if c.Value != 8 {
		t.Fatalf("Целое8::ШИРИНА: получено %#v", c)
	}
	g := main.Body[3].(*ir.VarDecl).Value.(*ir.CastExpr)
	if g.To != ir.TypeUint64 || g.Value.(*ir.IntLiteral).Value != -1 {
		t.Fatalf("Натуральное64::МАКСИМУМ (битовый паттерн -1 == UINT64_MAX): получено %#v", g)
	}
}

func TestAnalyzePlainIntTypeConstantsNoCast(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = Целое::МАКСИМУМ
}
`)
	main := irProg.Functions[0]
	if _, ok := main.Body[0].(*ir.VarDecl).Value.(*ir.IntLiteral); !ok {
		t.Fatalf("Целое::МАКСИМУМ должен быть голым IntLiteral (Целое == cType уже совпадает), получено %#v", main.Body[0].(*ir.VarDecl).Value)
	}
}

func TestAnalyzeFloatTypeConstants(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная а = Действительное::МАКСИМУМ
    переменная б = Дробное::МИНИМУМ
}
`)
	main := irProg.Functions[0]
	if got := ir.TypeOf(main.Body[0].(*ir.VarDecl).Value); got != ir.TypeFloat {
		t.Fatalf("Действительное::МАКСИМУМ: ожидался тип Действительное, получено %s", got)
	}
	if got := ir.TypeOf(main.Body[1].(*ir.VarDecl).Value); got != ir.TypeFloat32 {
		t.Fatalf("Дробное::МИНИМУМ: ожидался тип Дробное, получено %s", got)
	}
}

func TestAnalyzeUnknownTypeConstantFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная а = Целое8::ЧТОТО
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), `нет константы "ЧТОТО"`) {
		t.Fatalf("ожидалась ошибка про неизвестную константу, получено: %v", err)
	}
}

func TestAnalyzeTypeConstantAssignmentFails(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    Целое8::МАКСИМУМ = 5
}
`).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	_, err = Analyze(prog)
	if err == nil || !strings.Contains(err.Error(), "ей нельзя присвоить заново") {
		t.Fatalf("ожидалась ошибка про присваивание константе, получено: %v", err)
	}
}
