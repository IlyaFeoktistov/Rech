package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/concurrency"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestThreadPoolCodeDisabledByDefault - PoolShrinkEnabled=false запекает
// RECH_POOL_SHRINK_ENABLED 0 - препроцессор сворачивает воркер к прежнему
// безусловному ожиданию (pthread_cond_wait/SleepConditionVariableCS с
// INFINITE), без единой ссылки на таймаут.
func TestThreadPoolCodeDisabledByDefault(t *testing.T) {
	code := concurrency.ThreadPoolCode(false, 30000)
	if !strings.Contains(code, "#define RECH_POOL_SHRINK_ENABLED 0") {
		t.Fatalf("ожидался RECH_POOL_SHRINK_ENABLED 0, получено:\n%s", code)
	}
	if !strings.Contains(code, "pthread_cond_wait(&rech_pool_cond, &rech_pool_mutex);") {
		t.Fatalf("ожидалось безусловное ожидание (без таймаута) в отключённом режиме, получено:\n%s", code)
	}
}

// TestThreadPoolCodeShrinkEnabled - PoolShrinkEnabled=true запекает
// RECH_POOL_SHRINK_ENABLED 1 и RECH_POOL_IDLE_TIMEOUT_MS - выбранное
// значение, плюс ветки pthread_cond_timedwait/SleepConditionVariableCS с
// перепроверкой очереди перед завершением воркера.
func TestThreadPoolCodeShrinkEnabled(t *testing.T) {
	code := concurrency.ThreadPoolCode(true, 12345)
	if !strings.Contains(code, "#define RECH_POOL_SHRINK_ENABLED 1") {
		t.Fatalf("ожидался RECH_POOL_SHRINK_ENABLED 1, получено:\n%s", code)
	}
	if !strings.Contains(code, "#define RECH_POOL_IDLE_TIMEOUT_MS 12345") {
		t.Fatalf("ожидался RECH_POOL_IDLE_TIMEOUT_MS 12345, получено:\n%s", code)
	}
	if !strings.Contains(code, "pthread_cond_timedwait(&rech_pool_cond, &rech_pool_mutex, &rech_pool_deadline) != 0 && rech_pool_head == NULL") {
		t.Fatalf("ожидалась перепроверка очереди после таймаута (POSIX), получено:\n%s", code)
	}
	if !strings.Contains(code, "SleepConditionVariableCS(&rech_pool_cond, &rech_pool_mutex, RECH_POOL_IDLE_TIMEOUT_MS) && rech_pool_head == NULL") {
		t.Fatalf("ожидалась перепроверка очереди после таймаута (Windows), получено:\n%s", code)
	}
}

// TestGenerateFromIRWiresPoolShrinkSettings - "запустить" в программе,
// prog.PoolShrinkEnabled/PoolIdleTimeoutMs проброшены в сгенерированный C
// (internal/compiler подставляет их из ir.Program перед кодогенерацией -
// см. package doc ir.Program.PoolShrinkEnabled).
func TestGenerateFromIRWiresPoolShrinkSettings(t *testing.T) {
	fn := &ir.Function{Name: "поработать", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "работаю"}}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "задача", Value: &ir.LaunchExpr{Callee: "поработать"}},
	}}

	outDisabled, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{fn, main}, PoolIdleTimeoutMs: 30000})
	if err != nil {
		t.Fatalf("неожиданная ошибка (выключено): %v", err)
	}
	if !strings.Contains(outDisabled, "#define RECH_POOL_SHRINK_ENABLED 0") {
		t.Fatalf("ожидался RECH_POOL_SHRINK_ENABLED 0 по умолчанию, получено:\n%s", outDisabled)
	}

	outEnabled, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{fn, main}, PoolShrinkEnabled: true, PoolIdleTimeoutMs: 777})
	if err != nil {
		t.Fatalf("неожиданная ошибка (включено): %v", err)
	}
	if !strings.Contains(outEnabled, "#define RECH_POOL_SHRINK_ENABLED 1") {
		t.Fatalf("ожидался RECH_POOL_SHRINK_ENABLED 1, получено:\n%s", outEnabled)
	}
	if !strings.Contains(outEnabled, "#define RECH_POOL_IDLE_TIMEOUT_MS 777") {
		t.Fatalf("ожидался RECH_POOL_IDLE_TIMEOUT_MS 777, получено:\n%s", outEnabled)
	}
}
