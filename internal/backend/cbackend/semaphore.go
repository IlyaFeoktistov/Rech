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
package cbackend

import "fmt"

const semaphoreEntityName = "Семафор"

// semaphoreStructTypedef - C-структура "Семафор": счётчик разрешений +
// нативный мьютекс+condvar, без "vt" (никогда не участвует в
// наследовании/интерфейсах, как и "Блокировка").
func semaphoreStructTypedef() string {
	return fmt.Sprintf(
		"typedef struct {\n    long long count;\n#ifdef _WIN32\n    CRITICAL_SECTION mutex;\n    CONDITION_VARIABLE cond;\n#else\n    pthread_mutex_t mutex;\n    pthread_cond_t cond;\n#endif\n} %s;\n\n",
		mangleType(semaphoreEntityName),
	)
}

// semaphorePrototypes - прототипы конструктора и обоих методов "Семафора" -
// эмитятся в общий "protos" builder (ir_generator.go), тем же принципом,
// что и mutexPrototypes (её package doc - тот же порядковый аргумент:
// "Семафор" может добавиться в prog.Entities поздно относительно кода,
// который её использует).
func semaphorePrototypes() string {
	cType := mangleType(semaphoreEntityName)
	return fmt.Sprintf(
		"static %[1]s *%[2]s(long long начальное_значение);\nstatic void %[3]s(%[1]s *this_);\nstatic void %[4]s(%[1]s *this_);\n",
		cType, mangleCtor(semaphoreEntityName), mangleMethod(semaphoreEntityName, "захватить"), mangleMethod(semaphoreEntityName, "освободить"),
	)
}

// semaphoreRuntimeCode - конструктор ("новый Семафор(N)") и оба метода
// ("захватить"/"освободить") одним куском - эмитится безусловно, ровно один
// раз, только если ctx.usesSemaphore (см. GenerateFromIR). "захватить"
// ждёт, пока count не станет > 0 (pthread_cond_wait/SleepConditionVariableCS
// в цикле, как rech_db_checkout), затем декрементирует; "освободить"
// инкрементирует и будит РОВНО ОДНОГО ожидающего (pthread_cond_signal/
// WakeConditionVariable, не broadcast - "захватить" сам решит, кому из
// нескольких ожидающих в итоге достанется разрешение, обычная семантика
// счётного семафора).
func semaphoreRuntimeCode() string {
	cType := mangleType(semaphoreEntityName)
	ctor := mangleCtor(semaphoreEntityName)
	acquire := mangleMethod(semaphoreEntityName, "захватить")
	release := mangleMethod(semaphoreEntityName, "освободить")
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

static void %[3]s(%[1]s *this_) {
#ifdef _WIN32
    EnterCriticalSection(&this_->mutex);
    while (this_->count == 0) {
        SleepConditionVariableCS(&this_->cond, &this_->mutex, INFINITE);
    }
    this_->count--;
    LeaveCriticalSection(&this_->mutex);
#else
    pthread_mutex_lock(&this_->mutex);
    while (this_->count == 0) {
        pthread_cond_wait(&this_->cond, &this_->mutex);
    }
    this_->count--;
    pthread_mutex_unlock(&this_->mutex);
#endif
}

static void %[4]s(%[1]s *this_) {
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

`, cType, ctor, acquire, release)
}
