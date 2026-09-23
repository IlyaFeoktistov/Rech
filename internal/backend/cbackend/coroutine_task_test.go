package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRCoroAwaitEmitsCoroutineWrapper - "дождаться
// <асинхронная функция>(...)" (ir.LaunchExpr.Coroutine == true) должен
// эмитить структуру задачи/точку входа/пару обёрток "rech_coro_launch_.../
// rech_coro_join_..." и сам ucontext/epoll/kqueue-рантайм
// (concurrency.CoroutineRuntimeCode), а не rech_launch_.../rech_await_... (поток ОС) -
// см. package doc coroutine_task.go.
func TestGenerateFromIRCoroAwaitEmitsCoroutineWrapper(t *testing.T) {
	fn := &ir.Function{
		Name: "посчитать", Params: []ir.Param{{Name: "а", Type: ir.TypeInt}, {Name: "б", Type: ir.TypeInt}},
		ReturnType: ir.TypeInt, HasReturn: true,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "+", Left: &ir.Identifier{Name: "а", Type: ir.TypeInt}, Right: &ir.Identifier{Name: "б", Type: ir.TypeInt}, Type: ir.TypeInt}}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "результат", Value: &ir.AwaitExpr{
			Task:      &ir.LaunchExpr{Callee: "посчитать", Args: []ir.Expr{&ir.IntLiteral{Value: 2}, &ir.IntLiteral{Value: 3}}, Coroutine: true},
			Callee:    "посчитать",
			Type:      ir.TypeInt,
			HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"#include <gc.h>",
		"static RECH_THREAD_LOCAL rech_coro_t *rech_coro_queue_head = NULL;",
		"static void rech_coro_spawn(void (*fn)(void *), void *arg) {",
		"GC_add_roots(a->base, a->base + arena_bytes);",
		"static void rech_coro_wait_done(int *done) {",
		"typedef struct rech_CORO_TASK_poschitat rech_CORO_TASK_poschitat;",
		"typedef struct rech_CORO_TASK_poschitat {\n    long long arg_a;\n    long long arg_b;\n    long long result;\n    void *exc;\n    int done;\n} rech_CORO_TASK_poschitat;",
		"static void rech_coro_entry_poschitat(void *rech_arg) {",
		"static rech_CORO_TASK_poschitat *rech_coro_launch_poschitat(long long arg_a, long long arg_b) {",
		"static long long rech_coro_join_poschitat(rech_CORO_TASK_poschitat *t) {",
		"rech_coro_spawn(rech_coro_entry_poschitat, t);",
		"long long v_rezultat_INIT = rech_coro_join_poschitat(rech_coro_launch_poschitat(2LL, 3LL));",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	if strings.Contains(out, "rech_launch_poschitat") || strings.Contains(out, "rech_await_poschitat(") {
		t.Fatalf("сахар \"дождаться <асинхронная функция>()\" не должен эмитить обёртку потока ОС, получено:\n%s", out)
	}
	if strings.Count(out, "rech_CORO_TASK_poschitat;") != 2 {
		t.Fatalf("структура задачи корутины должна иметь ровно один полный typedef и один forward declaration, получено:\n%s", out)
	}
}

// TestGenerateFromIRExplicitLaunchOfAsyncFunctionStaysOnThread - явный
// "запустить" на той же функции, что где-то в программе ОТДЕЛЬНО вызвана
// через сахар "дождаться <асинхронная функция>()" - должны сосуществовать,
// каждый своей обёрткой (поток ОС / корутина), не взаимоисключающе (см.
// package doc ir_generator.go, coroutineFuncs/launchedFuncs).
func TestGenerateFromIRExplicitLaunchOfAsyncFunctionStaysOnThread(t *testing.T) {
	fn := &ir.Function{Name: "удвоить", Params: []ir.Param{{Name: "х", Type: ir.TypeInt}}, ReturnType: ir.TypeInt, HasReturn: true,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "*", Left: &ir.Identifier{Name: "х", Type: ir.TypeInt}, Right: &ir.IntLiteral{Value: 2}, Type: ir.TypeInt}}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "задача", Value: &ir.LaunchExpr{Callee: "удвоить", Args: []ir.Expr{&ir.IntLiteral{Value: 1}}}},
		&ir.VarDecl{Name: "результат", Value: &ir.AwaitExpr{
			Task:      &ir.LaunchExpr{Callee: "удвоить", Args: []ir.Expr{&ir.IntLiteral{Value: 2}}, Coroutine: true},
			Callee:    "удвоить",
			Type:      ir.TypeInt,
			HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static rech_TASK_udvoit *rech_launch_udvoit(long long arg_h) {") {
		t.Fatalf("явный \"запустить\" на асинхронной функции обязан оставить обёртку потока ОС, получено:\n%s", out)
	}
	if !strings.Contains(out, "static long long rech_coro_join_udvoit(rech_CORO_TASK_udvoit *t) {") {
		t.Fatalf("сахар \"дождаться удвоить(...)\" обязан дать СВОЮ отдельную обёртку-корутину, получено:\n%s", out)
	}
}

// TestGenerateFromIRCoroLaunchThenAwaitOverVariable - "поручить" (Шаг 132,
// ir.LaunchExpr.Coroutine == true, Task ЧЕРЕЗ ПЕРЕМЕННУЮ, не прямой вызов) -
// должен эмитить рознящиеся launch/join-вызовы (не фьюженный
// rech_coro_join_foo(rech_coro_launch_foo(...)) в одну строку, как у сахара
// "дождаться <вызов>()") - "поручить" немедленно спавнит и присваивает
// переменной, "дождаться" отдельно джойнит уже готовое значение.
func TestGenerateFromIRCoroLaunchThenAwaitOverVariable(t *testing.T) {
	fn := &ir.Function{Name: "удвоить", Params: []ir.Param{{Name: "х", Type: ir.TypeInt}}, ReturnType: ir.TypeInt, HasReturn: true,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "*", Left: &ir.Identifier{Name: "х", Type: ir.TypeInt}, Right: &ir.IntLiteral{Value: 2}, Type: ir.TypeInt}}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "задача", Value: &ir.LaunchExpr{Callee: "удвоить", Args: []ir.Expr{&ir.IntLiteral{Value: 21}}, Coroutine: true}},
		&ir.VarDecl{Name: "результат", Value: &ir.AwaitExpr{
			Task:      &ir.Identifier{Name: "задача", Type: ir.CoroTaskType("удвоить")},
			Callee:    "удвоить",
			Type:      ir.TypeInt,
			HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_coro_launch_udvoit(21LL);") {
		t.Fatalf("\"поручить\" обязан немедленно спавнить (rech_coro_launch_udvoit), получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_coro_join_udvoit(v_zadacha)") {
		t.Fatalf("\"дождаться\" над переменной от \"поручить\" обязан звать rech_coro_join_udvoit, получено:\n%s", out)
	}
	if strings.Contains(out, "rech_coro_join_udvoit(rech_coro_launch_udvoit(") {
		t.Fatalf("launch и join над переменной не должны быть фьюженными в один вызов, получено:\n%s", out)
	}
}

// TestGenerateFromIRCoroAwaitRethrowsUncaughtException - тот же контракт
// исключений, что и у потоковой версии (concurrency_test.go,
// TestGenerateFromIRAwaitRethrowsUncaughtExceptionFromTask), только через
// rech_coro_await_... вместо rech_await_....
func TestGenerateFromIRCoroAwaitRethrowsUncaughtException(t *testing.T) {
	exc := &ir.Entity{Name: systemExceptionEntity, Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}}
	fn := &ir.Function{Name: "сломать", Body: []ir.Stmt{
		&ir.ThrowStmt{Value: &ir.NewExpr{Entity: systemExceptionEntity, Args: []ir.Expr{&ir.StringLiteral{Value: "необработано"}}}},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.ExprStmt{Await: &ir.AwaitExpr{Task: &ir.LaunchExpr{Callee: "сломать", Coroutine: true}, Callee: "сломать"}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{exc}, Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `rech_throw(t->exc, "Исключение");`) {
		t.Fatalf("ожидался rech_throw(t->exc, \"Исключение\") внутри rech_coro_join_slomat, получено:\n%s", out)
	}
}
