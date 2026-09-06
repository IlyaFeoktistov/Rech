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
	"time"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
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

// evalSemaphoreMethod - "захватить(таймаут_мс)"/"освободить" на значении
// "Семафор" - recv.Obj.semaphore != nil отличает эту ветку от обычного
// метода/generic-коллекции/"Блокировки" (см. evalMethodCallExpr). Оба
// метода без результата, как и в cbackend (funcSig без ReturnType,
// internal/semantic/semaphore.go). "захватить" - таймаут_мс < 0 (умолчание
// -1, см. semaphoreWaitForeverDefault) ждёт бесконечно (s.cond.Wait() в
// цикле, как раньше); неотрицательный - ограничивает ожидание таймаутом
// через time.AfterFunc + s.cond.Broadcast (sync.Cond не умеет ждать с
// таймаутом сам по себе, в отличие от pthread_cond_timedwait/
// SleepConditionVariableCS в cbackend) - будит ВСЕХ ожидающих по истечении
// (не только этого одного - Broadcast, не Signal), каждый сам перепроверяет
// своё условие/дедлайн в цикле, ложные пробуждения остальных безвредны
// (тот же цикл "for count == 0"). Не дождался - паника (rtmsg.
// SemaphoreAcquireTimeout), тем же принципом, что и cbackend.
func (in *Interp) evalSemaphoreMethod(obj *entityObj, e *ir.MethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	s := obj.semaphore
	switch e.Method {
	case "захватить":
		timeoutArg, err := in.evalExpr(e.Args[0], env, cc)
		if err != nil {
			return Value{}, err
		}
		timeoutMs := timeoutArg.I
		s.mu.Lock()
		if timeoutMs < 0 {
			for s.count == 0 {
				s.cond.Wait()
			}
			s.count--
			s.mu.Unlock()
			return Value{}, nil
		}
		deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
		timer := time.AfterFunc(time.Duration(timeoutMs)*time.Millisecond, s.cond.Broadcast)
		for s.count == 0 && time.Now().Before(deadline) {
			s.cond.Wait()
		}
		timer.Stop()
		if s.count == 0 {
			s.mu.Unlock()
			panicRuntime("%s", rtmsg.SemaphoreAcquireTimeout)
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
