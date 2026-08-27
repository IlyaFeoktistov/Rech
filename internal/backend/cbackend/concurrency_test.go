package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRLaunchAwaitEmitsThreadWrapper - "запустить"/"дождаться"
// одной функции - структура задачи, точка входа потока и обёртки
// launch/await должны появиться ровно один раз, с чтением/автоматической
// регистрацией потока в Boehm GC через #define GC_THREADS (не ручные
// GC_register_my_thread/GC_unregister_my_thread - см. package doc
// concurrency.go, ensureTaskWrapper).
func TestGenerateFromIRLaunchAwaitEmitsThreadWrapper(t *testing.T) {
	fn := &ir.Function{
		Name: "посчитать", Params: []ir.Param{{Name: "а", Type: ir.TypeInt}, {Name: "б", Type: ir.TypeInt}},
		ReturnType: ir.TypeInt, HasReturn: true,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "+", Left: &ir.Identifier{Name: "а", Type: ir.TypeInt}, Right: &ir.Identifier{Name: "б", Type: ir.TypeInt}, Type: ir.TypeInt}}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "задача", Value: &ir.LaunchExpr{Callee: "посчитать", Args: []ir.Expr{&ir.IntLiteral{Value: 2}, &ir.IntLiteral{Value: 3}}}},
		&ir.VarDecl{Name: "результат", Value: &ir.AwaitExpr{
			Task: &ir.Identifier{Name: "задача", Type: ir.TaskType("посчитать")}, Callee: "посчитать", Type: ir.TypeInt, HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"#define GC_THREADS", "#include <gc.h>",
		"typedef struct {\n    long long arg_a;\n    long long arg_b;\n    long long result;\n    void *exc;\n#ifdef _WIN32\n    HANDLE thread;\n#else\n    pthread_t thread;\n#endif\n} rech_TASK_poschitat;",
		"static rech_TASK_poschitat *rech_launch_poschitat(long long arg_a, long long arg_b) {",
		"static long long rech_await_poschitat(rech_TASK_poschitat *t) {",
		"pthread_create(&t->thread, NULL, rech_task_entry_poschitat, t);",
		"CreateThread(NULL, 0, rech_task_entry_poschitat, t, 0, NULL);",
		"rech_TASK_poschitat * v_zadacha = rech_launch_poschitat(2LL, 3LL);",
		"long long v_rezultat = rech_await_poschitat(v_zadacha);",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	if strings.Contains(out, "GC_register_my_thread") || strings.Contains(out, "GC_unregister_my_thread") {
		t.Fatalf("ручная регистрация потока в GC не нужна (автоматическая через GC_THREADS-редирект) - получено:\n%s", out)
	}
	if strings.Count(out, "rech_TASK_poschitat;") != 1 {
		t.Fatalf("структура задачи должна эмититься ровно один раз, получено:\n%s", out)
	}
}

// TestGenerateFromIRLaunchWithoutAwaitStillCompilesCleanly - программа,
// которая только "запустить", без единого "дождаться" - не должна
// требовать rech_await_... вовсе (никто не ссылается), но launch и
// исключения-инфраструктура (catch-frame вокруг тела потока) - обязаны
// присутствовать.
func TestGenerateFromIRLaunchWithoutAwaitStillCompilesCleanly(t *testing.T) {
	fn := &ir.Function{Name: "поработать", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "работаю"}}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "задача", Value: &ir.LaunchExpr{Callee: "поработать"}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_launch_porabotat(void)") {
		t.Fatalf("ожидался rech_launch_porabotat, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_catch_frame rech_frame;") {
		t.Fatalf("ожидался catch-frame вокруг тела потока, получено:\n%s", out)
	}
}

// TestGenerateFromIRAwaitRethrowsUncaughtExceptionFromTask - исключение,
// не пойманное внутри задачи, должно подниматься заново через rech_throw
// в rech_await_..., с тем же текстом типа, что и обычное "выбросить" (см.
// throwTypeNameExpr).
func TestGenerateFromIRAwaitRethrowsUncaughtExceptionFromTask(t *testing.T) {
	exc := &ir.Entity{Name: systemExceptionEntity, Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}}
	fn := &ir.Function{Name: "сломать", Body: []ir.Stmt{
		&ir.ThrowStmt{Value: &ir.NewExpr{Entity: systemExceptionEntity, Args: []ir.Expr{&ir.StringLiteral{Value: "необработано"}}}},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "задача", Value: &ir.LaunchExpr{Callee: "сломать"}},
		&ir.ExprStmt{Await: &ir.AwaitExpr{Task: &ir.Identifier{Name: "задача", Type: ir.TaskType("сломать")}, Callee: "сломать"}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{exc}, Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `rech_throw(t->exc, "Исключение");`) {
		t.Fatalf("ожидался rech_throw(t->exc, \"Исключение\") (нет наследников - без rech_isa), получено:\n%s", out)
	}
}
