package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRMutexEmitsNativeStructCtorAndMethods - "Блокировка"
// (ent.IsMutex) должна получить нативную C-структуру (pthread_mutex_t/
// CRITICAL_SECTION, без "vt" - никогда не участвует в диспетчеризации),
// конструктор без параметров и оба метода - без единого поля/тела на
// стороне IR (Fields/Methods у самой "Блокировки" пусты, весь код
// синтезирован mutex.go). pthread.h должен подключаться уже из одного
// только usesMutex, без единого "запустить"/"дождаться" в программе.
func TestGenerateFromIRMutexEmitsNativeStructCtorAndMethods(t *testing.T) {
	mutex := &ir.Entity{Name: "Блокировка", IsMutex: true}
	scholar := &ir.Entity{
		Name:   "Счёт",
		Fields: []ir.Field{{Name: "баланс", Type: ir.TypeInt}, {Name: "блокировка", Type: ir.EntityType("Блокировка")}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "счёт", Value: &ir.NewExpr{Entity: "Счёт", Args: []ir.Expr{&ir.IntLiteral{Value: 0}, &ir.NewExpr{Entity: "Блокировка"}}}},
		&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{
			Receiver: &ir.FieldAccess{Receiver: &ir.Identifier{Name: "счёт", Type: ir.EntityType("Счёт")}, Field: "блокировка", Type: ir.EntityType("Блокировка")},
			Entity:   "Блокировка", Method: "захватить",
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{mutex, scholar}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"#ifndef _WIN32\n#include <pthread.h>\n#endif\n",
		"typedef struct {\n#ifdef _WIN32\n    CRITICAL_SECTION native;\n#else\n    pthread_mutex_t native;\n#endif\n} rech_T_blokirovka;",
		"static rech_T_blokirovka *rech_new_blokirovka(void);",
		"static void rech_M_blokirovka_zahvatit(rech_T_blokirovka *this_);",
		"static void rech_M_blokirovka_osvobodit(rech_T_blokirovka *this_);",
		"pthread_mutex_init(&p->native, NULL);",
		"InitializeCriticalSection(&p->native);",
		"pthread_mutex_lock(&this_->native);",
		"EnterCriticalSection(&this_->native);",
		"pthread_mutex_unlock(&this_->native);",
		"LeaveCriticalSection(&this_->native);",
		"rech_M_blokirovka_zahvatit(((v_schyot)->f_blokirovka));",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	if strings.Contains(out, "const rech_vtable *vt;\n} rech_T_blokirovka;") {
		t.Fatalf("\"Блокировка\" не должна участвовать в диспетчеризации (нет vt), получено:\n%s", out)
	}
	if strings.Contains(out, "#define GC_THREADS") {
		t.Fatalf("GC_THREADS - только для \"запустить\"/\"дождаться\" (usesThreads), \"Блокировка\" сама по себе потоков не создаёт, получено:\n%s", out)
	}
}

// TestGenerateFromIRWithoutMutexOmitsMutexRuntime - программа без единой
// "Блокировки" не должна тянуть за собой pthread.h или синтезированный код
// mutex.go вовсе.
func TestGenerateFromIRWithoutMutexOmitsMutexRuntime(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "привет"}}}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, unwanted := range []string{"pthread.h", "Блокировка", "rech_T_blokirovka", "rech_new_Блокировка"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("программа без \"Блокировки\" не должна содержать %q, получено:\n%s", unwanted, out)
		}
	}
}
