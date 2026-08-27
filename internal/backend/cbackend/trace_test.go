package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRTracePushPopFunctionsAndMethods - каждая функция/метод
// заводит свой кадр при входе (rech_trace_push с Rech-именем, "Тип.метод" у
// метода) и снимает его перед выходом (rech_trace_pop, включая неявный
// "return 0" функции без объявленного результата).
func TestGenerateFromIRTracePushPopFunctionsAndMethods(t *testing.T) {
	entity := &ir.Entity{
		Name:   "Счётчик",
		Fields: []ir.Field{{Name: "значение", Type: ir.TypeInt}},
		Methods: []*ir.Method{
			{Name: "увеличить", Body: []ir.Stmt{
				&ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: "Счётчик"}, Field: "значение", Value: &ir.IntLiteral{Value: 1}, Line: 5},
			}},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.StringLiteral{Value: "ок"}, Line: 1},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `rech_trace_push("главная");`) {
		t.Fatalf("ожидался push кадра главная, получено:\n%s", out)
	}
	if !strings.Contains(out, `rech_trace_push("Счётчик.увеличить");`) {
		t.Fatalf("ожидался push кадра метода с именем \"Тип.метод\", получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_trace_line(5);") {
		t.Fatalf("ожидался rech_trace_line на присваивании полю, получено:\n%s", out)
	}
	if strings.Count(out, "rech_trace_pop();") < 2 {
		t.Fatalf("ожидался pop кадра и у главная, и у метода, получено:\n%s", out)
	}
}

// TestGenerateFromIRTraceReturnEvaluatesBeforePop - "вернуть <выражение>"
// вычисляет значение ДО rech_trace_pop() (во временную переменную) - если
// само выражение паникует (вложенный вызов), кадр этой функции ещё обязан
// быть на стеке в момент паники.
func TestGenerateFromIRTraceReturnEvaluatesBeforePop(t *testing.T) {
	fn := &ir.Function{
		Name: "сорок_два", HasReturn: true, ReturnType: ir.TypeInt,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.IntLiteral{Value: 42}, Line: 3}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CallExpr{Callee: "сорок_два", Type: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	tmpIdx := strings.Index(out, "rech_ret_tmp = 42LL;")
	popIdx := strings.Index(out, "rech_trace_pop();")
	retIdx := strings.Index(out, "return rech_ret_tmp;")
	if tmpIdx == -1 || popIdx == -1 || retIdx == -1 || !(tmpIdx < popIdx && popIdx < retIdx) {
		t.Fatalf("ожидался порядок: значение во временную переменную, затем rech_trace_pop(), затем return, получено:\n%s", out)
	}
}

// TestGenerateFromIRTraceRuntimeAlwaysEmitted - traceHelper (стек вызовов)
// эмитится безусловно, тем же принципом, что и consoleInitHelper/
// argsGlobals - дешёвая проверка на каждый вызов, без дополнительных
// заголовков/библиотек (см. package doc trace.go).
func TestGenerateFromIRTraceRuntimeAlwaysEmitted(t *testing.T) {
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{mainFuncIR()}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{"rech_trace_push", "rech_trace_pop", "rech_trace_line", "rech_trace_print", "RECH_THREAD_LOCAL"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в рантайме стека вызовов, получено:\n%s", want, out)
		}
	}
}
