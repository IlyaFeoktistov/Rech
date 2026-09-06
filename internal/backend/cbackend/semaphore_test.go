package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRSemaphoreEmitsNativeStructCtorAndMethods - "Семафор"
// (ent.IsSemaphore) должен получить нативную C-структуру (счётчик +
// pthread_mutex_t/pthread_cond_t или CRITICAL_SECTION/CONDITION_VARIABLE,
// без "vt"), конструктор с ОДНИМ параметром (начальное_значение, в отличие
// от "Блокировки") и оба метода - без единого поля/тела на стороне IR
// (Fields/Methods у самого "Семафора" пусты, весь код синтезирован
// semaphore.go). pthread.h должен подключаться уже из одного только
// usesSemaphore, без единого "запустить"/"дождаться"/"Блокировка" в
// программе.
func TestGenerateFromIRSemaphoreEmitsNativeStructCtorAndMethods(t *testing.T) {
	semaphore := &ir.Entity{Name: "Семафор", IsSemaphore: true}
	pool := &ir.Entity{
		Name:   "Пул",
		Fields: []ir.Field{{Name: "счёт", Type: ir.TypeInt}, {Name: "семафор", Type: ir.EntityType("Семафор")}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "пул", Value: &ir.NewExpr{Entity: "Пул", Args: []ir.Expr{&ir.IntLiteral{Value: 0}, &ir.NewExpr{Entity: "Семафор", Args: []ir.Expr{&ir.IntLiteral{Value: 3}}}}}},
		&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{
			Receiver: &ir.FieldAccess{Receiver: &ir.Identifier{Name: "пул", Type: ir.EntityType("Пул")}, Field: "семафор", Type: ir.EntityType("Семафор")},
			Entity:   "Семафор", Method: "захватить",
			// -1 - как реальный вызывающий код после разрешения значения по
			// умолчанию ("сем.захватить()" без аргумента, internal/semantic/
			// semaphore.go, semaphoreWaitForeverDefault) - этот тест строит IR
			// вручную, минуя semantic.Analyze, поэтому подставляет его сам.
			Args: []ir.Expr{&ir.IntLiteral{Value: -1}},
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{semaphore, pool}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"#ifndef _WIN32\n#include <pthread.h>\n#endif\n",
		"long long count;\n#ifdef _WIN32\n    CRITICAL_SECTION mutex;\n    CONDITION_VARIABLE cond;\n#else\n    pthread_mutex_t mutex;\n    pthread_cond_t cond;\n#endif\n} rech_T_semafor;",
		"static rech_T_semafor *rech_new_semafor(long long начальное_значение);",
		"static void rech_M_semafor_zahvatit(rech_T_semafor *this_, long long таймаут_мс);",
		"static void rech_M_semafor_osvobodit(rech_T_semafor *this_);",
		"p->count = начальное_значение;",
		"pthread_mutex_init(&p->mutex, NULL);",
		"InitializeCriticalSection(&p->mutex);",
		"while (this_->count == 0) {",
		"this_->count--;",
		"this_->count++;",
		"pthread_cond_signal(&this_->cond);",
		"WakeConditionVariable(&this_->cond);",
		"rech_M_semafor_zahvatit(((v_pul)->f_semafor), -1LL);",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	if strings.Contains(out, "const rech_vtable *vt;\n} rech_T_semafor;") {
		t.Fatalf("\"Семафор\" не должен участвовать в диспетчеризации (нет vt), получено:\n%s", out)
	}
}

// TestGenerateFromIRWithoutSemaphoreOmitsSemaphoreRuntime - программа без
// единого "Семафора" не должна тянуть за собой pthread.h или синтезированный
// код semaphore.go вовсе.
func TestGenerateFromIRWithoutSemaphoreOmitsSemaphoreRuntime(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "привет"}}}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, unwanted := range []string{"pthread.h", "Семафор", "rech_T_semafor", "rech_new_semafor"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("программа без \"Семафора\" не должна содержать %q, получено:\n%s", unwanted, out)
		}
	}
}
