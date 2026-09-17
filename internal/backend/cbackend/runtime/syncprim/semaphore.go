// Синхронизация между потоками - "Семафор" (голый счётный семафор поверх
// нативного мьютекса+condvar ОС - pthread_mutex_t/pthread_cond_t на POSIX,
// CRITICAL_SECTION/CONDITION_VARIABLE на Windows, обобщение "Блокировки",
// mutex.go - без встроенной проверки диапазона начального значения, тот же
// принцип "оставить программисту", что и у неё).
//
// Как и "Блокировка" - не обычная сущность (нет ast.EntityDecl, ent.Methods/
// ent.Ctor пусты, см. package doc ir.Entity.IsSemaphore): C-структура,
// конструктор и оба метода (захватить/освободить) целиком синтезированы
// здесь. Ожидание/сигнал устроены тем же приёмом, что и rech_db_checkout/
// rech_db_checkin (db.go) - мьютекс+condvar вокруг цикла ожидания вместо
// массива слотов, счётчик разрешений вместо массива занятости.
//
// "захватить" принимает необязательный "таймаут_мс" (internal/semantic/
// semaphore.go, значение по умолчанию -1 - ждать бесконечно, тем же
// принципом default-параметров, что и у обычных функций/методов, Шаг 38) -
// HTTP highload пункт 3 (admission control): явный
// неотрицательный аргумент ограничивает ожидание сверху, паникуя
// (rtmsg.SemaphoreAcquireTimeout), если разрешение не появилось вовремя (0
// - не ждать вовсе). POSIX-ветка - абсолютный дедлайн (pthread_cond_
// timedwait), тем же приёмом, что и rech_db_checkout (db.go); Windows-ветка
// - относительный таймаут, переиздаваемый заново на каждой итерации цикла
// (та же осознанная неточность при ложных пробуждениях, что и у
// rech_db_checkout - не новое допущение).
package syncprim

import "fmt"

// SemaphoreEntityName - Rech-имя сущности "Семафор".
const SemaphoreEntityName = "Семафор"

// SemaphoreStructTypedef - C-структура "Семафор": счётчик разрешений +
// нативный мьютекс+condvar, без "vt" (никогда не участвует в
// наследовании/интерфейсах, как и "Блокировка").
func SemaphoreStructTypedef(mangledType string) string {
	return fmt.Sprintf(
		"typedef struct {\n    long long count;\n#ifdef _WIN32\n    CRITICAL_SECTION mutex;\n    CONDITION_VARIABLE cond;\n#else\n    pthread_mutex_t mutex;\n    pthread_cond_t cond;\n#endif\n} %s;\n\n",
		mangledType,
	)
}

// SemaphorePrototypes - прототипы конструктора и обоих методов "Семафора" -
// эмитятся в общий "protos" builder (ir_generator.go), тем же принципом,
// что и MutexPrototypes (её package doc - тот же порядковый аргумент:
// "Семафор" может добавиться в prog.Entities поздно относительно кода,
// который её использует).
func SemaphorePrototypes(mangledType, ctorName, acquireMethod, releaseMethod string) string {
	return fmt.Sprintf(
		"static %[1]s *%[2]s(long long начальное_значение);\nstatic void %[3]s(%[1]s *this_, long long таймаут_мс);\nstatic void %[4]s(%[1]s *this_);\n",
		mangledType, ctorName, acquireMethod, releaseMethod,
	)
}

// SemaphoreRuntimeCode - конструктор ("новый Семафор(N)") и оба метода
// ("захватить"/"освободить") одним куском - эмитится безусловно, ровно один
// раз, только если ctx.usesSemaphore (см. GenerateFromIR). "захватить"
// ждёт, пока count не станет > 0 (pthread_cond_wait/SleepConditionVariableCS
// в цикле, как rech_db_checkout), затем декрементирует; "освободить"
// инкрементирует и будит РОВНО ОДНОГО ожидающего (pthread_cond_signal/
// WakeConditionVariable, не broadcast - "захватить" сам решит, кому из
// нескольких ожидающих в итоге достанется разрешение, обычная семантика
// счётного семафора).
//
// timeoutPanic/deadlineCode - уже готовые куски C (rech_panic(...)
// с экранированным сообщением, вычисление дедлайна pthread_cond_timedwait -
// timespec.DeadlineCode) - вычисляются вызывающей стороной
// (entity_codegen.go), не этим пакетом: экранирование C-строк
// (escapeCString) - внутренняя инфраструктура cbackend, этот пакет от неё
// не зависит.
func SemaphoreRuntimeCode(mangledType, ctorName, acquireMethod, releaseMethod, timeoutPanic, deadlineCode string) string {
	return fmt.Sprintf(`static %[1]s *%[2]s(long long начальное_значение) {
    %[1]s *p = GC_MALLOC(sizeof(%[1]s));
    p->count = начальное_значение;
#ifdef _WIN32
    InitializeCriticalSection(&p->mutex);
    InitializeConditionVariable(&p->cond);
#else
    pthread_mutex_init(&p->mutex, NULL);
    pthread_cond_init(&p->cond, NULL);
#endif
    return p;
}

`, mangledType, ctorName) +
		fmt.Sprintf(`static void %[1]s(%[2]s *this_, long long таймаут_мс) {
#ifdef _WIN32
    EnterCriticalSection(&this_->mutex);
    if (таймаут_мс < 0) {
        while (this_->count == 0) {
            SleepConditionVariableCS(&this_->cond, &this_->mutex, INFINITE);
        }
    } else {
        DWORD ждать_мс = (DWORD)таймаут_мс;
        while (this_->count == 0) {
            if (!SleepConditionVariableCS(&this_->cond, &this_->mutex, ждать_мс)) {
                LeaveCriticalSection(&this_->mutex);
                `, acquireMethod, mangledType) + timeoutPanic + `
            }
        }
    }
    this_->count--;
    LeaveCriticalSection(&this_->mutex);
#else
    pthread_mutex_lock(&this_->mutex);
    if (таймаут_мс < 0) {
        while (this_->count == 0) {
            pthread_cond_wait(&this_->cond, &this_->mutex);
        }
    } else {
        struct timespec rech_sem_deadline;
` + deadlineCode + `        while (this_->count == 0) {
            if (pthread_cond_timedwait(&this_->cond, &this_->mutex, &rech_sem_deadline) != 0) {
                pthread_mutex_unlock(&this_->mutex);
                ` + timeoutPanic + `
            }
        }
    }
    this_->count--;
    pthread_mutex_unlock(&this_->mutex);
#endif
}

` + fmt.Sprintf(`static void %[1]s(%[2]s *this_) {
#ifdef _WIN32
    EnterCriticalSection(&this_->mutex);
    this_->count++;
    LeaveCriticalSection(&this_->mutex);
    WakeConditionVariable(&this_->cond);
#else
    pthread_mutex_lock(&this_->mutex);
    this_->count++;
    pthread_mutex_unlock(&this_->mutex);
    pthread_cond_signal(&this_->cond);
#endif
}

`, releaseMethod, mangledType)
}
