// Синхронизация между потоками - "Семафор" (см. package doc
// internal/backend/cbackend/semaphore.go для полного описания примитива и
// решения дизайна - здесь только то, что специфично интерпретатору).
// sync.Cond - естественный аналог нативного condvar ОС, тем же принципом,
// что sync.Mutex у internal/interp/mutex.go: evalNewExpr перехватывает
// "новый Семафор(N)" раньше обычного пути (in.entities[e.Entity]) и
// заводит semaphoreState напрямую.
package interp

import (
	"fmt"
	"sync"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

const semaphoreEntityName = "Семафор"

// semaphoreState - count защищён mu, cond будит ровно одного ожидающего в
// "освободить" (Signal, не Broadcast - обычная семантика счётного
// семафора, как и WakeConditionVariable/pthread_cond_signal в cbackend).
type semaphoreState struct {
	mu    sync.Mutex
	cond  *sync.Cond
	count int64
}

// newSemaphoreValue - "новый Семафор(начальное_значение)".
func newSemaphoreValue(n int64) Value {
	s := &semaphoreState{count: n}
	s.cond = sync.NewCond(&s.mu)
	return Value{Type: ir.EntityType(semaphoreEntityName), Obj: &entityObj{typeName: semaphoreEntityName, semaphore: s}}
}

// evalSemaphoreMethod - "захватить"/"освободить" на значении "Семафор" -
// recv.Obj.semaphore != nil отличает эту ветку от обычного метода/generic-
// коллекции/"Блокировки" (см. evalMethodCallExpr). Оба метода без
// результата, как и в cbackend (funcSig без ReturnType, internal/semantic/
// semaphore.go).
func evalSemaphoreMethod(obj *entityObj, e *ir.MethodCallExpr) (Value, error) {
	s := obj.semaphore
	switch e.Method {
	case "захватить":
		s.mu.Lock()
		for s.count == 0 {
			s.cond.Wait()
		}
		s.count--
		s.mu.Unlock()
	case "освободить":
		s.mu.Lock()
		s.count++
		s.mu.Unlock()
		s.cond.Signal()
	default:
		return Value{}, fmt.Errorf("у сущности %q нет метода %q", semaphoreEntityName, e.Method)
	}
	return Value{}, nil
}
