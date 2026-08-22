package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

func parseSource(t *testing.T, src string) (*ast.Program, error) {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	return New(toks).ParseProgram()
}

func TestParseFunctionParamsAndReturnType(t *testing.T) {
	src := `
функция сложить(кого: Целое, кому: Целое): Целое
{
    вернуть кого + кому
}
функция главная { вывести "ок" }
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if fn.Name != "сложить" {
		t.Fatalf("ожидалось имя \"сложить\", получено %q", fn.Name)
	}
	if len(fn.Params) != 2 {
		t.Fatalf("ожидалось 2 параметра, получено %d", len(fn.Params))
	}
	if fn.Params[0].Name != "кого" || fn.Params[0].Type != "Целое" {
		t.Fatalf("неожиданный первый параметр: %+v", fn.Params[0])
	}
	if fn.Params[1].Name != "кому" || fn.Params[1].Type != "Целое" {
		t.Fatalf("неожиданный второй параметр: %+v", fn.Params[1])
	}
	if fn.ReturnType != "Целое" {
		t.Fatalf("ожидался тип результата \"Целое\", получено %q", fn.ReturnType)
	}
	ret, ok := fn.Body[0].(*ast.ReturnStmt)
	if !ok || ret.Value == nil {
		t.Fatalf("ожидался \"вернуть <значение>\", получено %+v", fn.Body[0])
	}
}

func TestParseFunctionInvalidParamName(t *testing.T) {
	_, err := parseSource(t, `
функция сложить(х: Целое): Целое { вернуть х }
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "падежных вопросительных слов") {
		t.Fatalf("ожидалась ошибка про падежные имена параметров, получено: %v", err)
	}
}

func TestParseFunctionDuplicateCase(t *testing.T) {
	_, err := parseSource(t, `
функция сложить(кого: Целое, что: Целое): Целое { вернуть кого + что }
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "дублирует падежную роль") {
		t.Fatalf("ожидалась ошибка про дублирование падежа, получено: %v", err)
	}
}

func TestParseEntryPointCannotHaveParams(t *testing.T) {
	_, err := parseSource(t, `функция главная(кого: Целое) { вывести кого }`)
	if err == nil || !strings.Contains(err.Error(), "точка входа") {
		t.Fatalf("ожидалась ошибка про точку входа, получено: %v", err)
	}
}

func TestParseFunctionWithoutParamsStillWorks(t *testing.T) {
	prog, err := parseSource(t, `функция главная { вывести "ок" }`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	fn := prog.Functions[0]
	if len(fn.Params) != 0 || fn.ReturnType != "" {
		t.Fatalf("функция без параметров/типа результата должна остаться пустой: %+v", fn)
	}
}

func TestParseCallExprAsExpression(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    переменная х = сложить(1, 2)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	varDecl := prog.Functions[0].Body[0].(*ast.VarDecl)
	call, ok := varDecl.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("ожидался CallExpr, получено %T", varDecl.Value)
	}
	if call.Callee != "сложить" || len(call.Args) != 2 {
		t.Fatalf("неожиданный вызов: %+v", call)
	}
}

func TestParseCallAsStatement(t *testing.T) {
	prog, err := parseSource(t, `функция главная { поздороваться() }`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	stmt, ok := prog.Functions[0].Body[0].(*ast.ExprStmt)
	if !ok || stmt.Call.Callee != "поздороваться" {
		t.Fatalf("ожидался ExprStmt с вызовом \"поздороваться\", получено %+v", prog.Functions[0].Body[0])
	}
}

func TestParseReturnWithoutValueStillWorks(t *testing.T) {
	prog, err := parseSource(t, `
функция главная {
    если истина {
        вернуть
    }
    вывести "ок"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ifStmt := prog.Functions[0].Body[0].(*ast.IfStmt)
	ret := ifStmt.Then[0].(*ast.ReturnStmt)
	if ret.Value != nil {
		t.Fatalf("ожидался \"вернуть\" без значения, получено значение: %+v", ret.Value)
	}
}
