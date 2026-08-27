package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeFuncRefExpr(t *testing.T) {
	prog := parseProgram(t, `
функция удвоить(x: Целое): Целое { вернуть x * 2 }
функция главная
{
    переменная ф = &удвоить
    вывести ф(21)
}
`)
	fn := prog.Functions[1]
	decl := fn.Body[0].(*ir.VarDecl)
	ref, ok := decl.Value.(*ir.FuncRefExpr)
	if !ok {
		t.Fatalf("ожидался FuncRefExpr, получено %T", decl.Value)
	}
	if ref.Type.Kind != ir.KindCallable {
		t.Fatalf("ожидался тип KindCallable, получено %s", ref.Type)
	}
	want := "Вызываемый<Целое>Целое"
	if ref.Type.String() != want {
		t.Fatalf("ожидалась сигнатура %q, получено %q", want, ref.Type.String())
	}
}

func TestAnalyzeFuncRefRejectsBuiltin(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная ф = &корень
}
`)
	if err == nil || !strings.Contains(err.Error(), "встроенные функции") {
		t.Fatalf("ожидалась ошибка про встроенные функции, получено %v", err)
	}
}

func TestAnalyzeFuncRefRejectsExtern(t *testing.T) {
	_, err := AnalyzeSource(t, `
внешняя функция puts(строка: Строка)
функция главная
{
    переменная ф = &puts
}
`)
	if err == nil || !strings.Contains(err.Error(), "внешние функции") {
		t.Fatalf("ожидалась ошибка про внешние функции, получено %v", err)
	}
}

func TestAnalyzeLambdaCapture(t *testing.T) {
	prog := parseProgram(t, `
функция главная
{
    переменная база = 10
    переменная сложить = функция(x: Целое): Целое
    {
        вернуть x + база
    }
    вывести сложить(5)
}
`)
	fn := prog.Functions[0]
	decl := fn.Body[1].(*ir.VarDecl)
	lambda, ok := decl.Value.(*ir.LambdaExpr)
	if !ok {
		t.Fatalf("ожидался LambdaExpr, получено %T", decl.Value)
	}
	if len(lambda.Captures) != 1 || lambda.Captures[0].Name != "база" {
		t.Fatalf("ожидался один захват \"база\", получено %+v", lambda.Captures)
	}
	if lambda.Captures[0].Type != ir.TypeInt {
		t.Fatalf("ожидался тип захвата Целое, получено %s", lambda.Captures[0].Type)
	}
}

func TestAnalyzeCallValueExpr(t *testing.T) {
	prog := parseProgram(t, `
функция применить(значение: Целое, ф: Вызываемый<Целое>Целое): Целое
{
    вернуть ф(значение)
}
функция главная { вывести применить(1, функция(x: Целое): Целое { вернуть x }) }
`)
	fn := prog.Functions[0]
	ret := fn.Body[0].(*ir.ReturnStmt)
	call, ok := ret.Value.(*ir.CallValueExpr)
	if !ok {
		t.Fatalf("ожидался CallValueExpr, получено %T", ret.Value)
	}
	if !call.HasReturn || call.Type != ir.TypeInt {
		t.Fatalf("ожидался результат Целое, получено %+v", call)
	}
}

func TestAnalyzeCallableTypeMismatch(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция применить(значение: Целое, ф: Вызываемый<Целое>Целое): Целое
{
    вернуть ф(значение)
}
функция строка_в_строку(s: Строка): Строка { вернуть s }
функция главная { вывести применить(1, &строка_в_строку) }
`)
	if err == nil {
		t.Fatal("ожидалась ошибка несовпадения типов")
	}
}

// TestAnalyzeCallableNestedSignatureOrder - Program.Callables несёт
// вложенную сигнатуру (Callable-параметр другого Callable) РАНЬШЕ той,
// что её использует - иначе cbackend сгенерировал бы typedef, ссылающийся
// на ещё не объявленный C-тип (найдено реальной сборкой cc на
// "Вызываемый<Вызываемый<Список<Целое>>Целое>Целое").
func TestAnalyzeCallableNestedSignatureOrder(t *testing.T) {
	prog := parseProgram(t, `
функция обёртка(значение: Вызываемый<Вызываемый<Целое>Целое>Целое): Целое
{
    вернуть 0
}
функция главная { вывести 1 }
`)
	indexOf := func(name string) int {
		for i, c := range prog.Callables {
			if c.Name == name {
				return i
			}
		}
		return -1
	}
	inner := indexOf("Вызываемый<Целое>Целое")
	outer := indexOf("Вызываемый<Вызываемый<Целое>Целое>Целое")
	if inner == -1 || outer == -1 {
		t.Fatalf("ожидались обе сигнатуры, получено %+v", prog.Callables)
	}
	if inner >= outer {
		t.Fatalf("вложенная сигнатура (%d) должна идти раньше внешней (%d)", inner, outer)
	}
}

func TestAnalyzeCallableProgramCallables(t *testing.T) {
	prog := parseProgram(t, `
функция удвоить(x: Целое): Целое { вернуть x * 2 }
функция главная
{
    переменная ф = &удвоить
    вывести ф(21)
}
`)
	if len(prog.Callables) != 1 {
		t.Fatalf("ожидалась одна сигнатура Callable, получено %+v", prog.Callables)
	}
	sig := prog.Callables[0]
	if len(sig.Params) != 1 || sig.Params[0] != ir.TypeInt || sig.Result == nil || *sig.Result != ir.TypeInt {
		t.Fatalf("неожиданная сигнатура %+v", sig)
	}
}
