package cbackend

import "github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/timespec"

// activeTasksCode - счётчик КОГДА-ЛИБО ЗАПУЩЕННЫХ, ЕЩЁ НЕ ЗАВЕРШИВШИХСЯ
// задач ("запустить") - один общий на весь процесс, не по функции (в
// отличие от per-функции обёртки в concurrency.go) - "дождаться_всех_задач"
// не знает заранее, какие именно функции были запущены за время работы
// программы. rech_active_tasks_inc/_dec зовутся из rech_launch_.../
// точки входа потока (concurrency.go, ensureTaskWrapper) для КАЖДОЙ
// когда-либо обёрнутой функции - без этого счётчика ждать было бы вообще
// нечего: обёрнутая задача, запущенная без сохранения дескриптора для
// "дождаться" (осознанный сценарий "фоновой задачи" - поток на HTTP-
// соединение http_сервера), иначе никак не наблюдаема со стороны.
//
// Мьютекс - CRITICAL_SECTION/pthread_mutex_t с ленивой инициализацией на
// Windows (InterlockedCompareExchange), тем же приёмом, что и rech_tls_lock
// (tls.go) - CONDITION_VARIABLE, в отличие от CRITICAL_SECTION, имеет
// статический инициализатор (CONDITION_VARIABLE_INIT), отдельная ленивая
// инициализация ей не нужна. pthread_cond_t/CONDITION_VARIABLE, не
// busy-poll - "дождаться_всех_задач" просыпается СРАЗУ, как только счётчик
// досчитывается до нуля, не по истечении таймаута впустую, если все задачи
// управились раньше.
var activeTasksCode = `static long long rech_active_tasks_count = 0;
#ifdef _WIN32
static CRITICAL_SECTION rech_active_tasks_mutex;
static volatile LONG rech_active_tasks_mutex_ready = 0;
static void rech_active_tasks_mutex_init(void) {
    if (InterlockedCompareExchange(&rech_active_tasks_mutex_ready, 1, 0) == 0) {
        InitializeCriticalSection(&rech_active_tasks_mutex);
    }
}
static CONDITION_VARIABLE rech_active_tasks_cond = CONDITION_VARIABLE_INIT;
#define RECH_ACTIVE_TASKS_LOCK() (rech_active_tasks_mutex_init(), EnterCriticalSection(&rech_active_tasks_mutex))
#define RECH_ACTIVE_TASKS_UNLOCK() LeaveCriticalSection(&rech_active_tasks_mutex)
#else
static pthread_mutex_t rech_active_tasks_mutex = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t rech_active_tasks_cond = PTHREAD_COND_INITIALIZER;
#define RECH_ACTIVE_TASKS_LOCK() pthread_mutex_lock(&rech_active_tasks_mutex)
#define RECH_ACTIVE_TASKS_UNLOCK() pthread_mutex_unlock(&rech_active_tasks_mutex)
#endif

static void rech_active_tasks_inc(void) {
    RECH_ACTIVE_TASKS_LOCK();
    rech_active_tasks_count++;
    RECH_ACTIVE_TASKS_UNLOCK();
}

static void rech_active_tasks_dec(void) {
    RECH_ACTIVE_TASKS_LOCK();
    rech_active_tasks_count--;
    if (rech_active_tasks_count == 0) {
#ifdef _WIN32
        WakeAllConditionVariable(&rech_active_tasks_cond);
#else
        pthread_cond_broadcast(&rech_active_tasks_cond);
#endif
    }
    RECH_ACTIVE_TASKS_UNLOCK();
}

// rech_wait_all_tasks - "дождаться_всех_задач(миллисекунды): Логическое" -
// истина, если счётчик дошёл до нуля ДО истечения таймаута (ложь - таймаут
// истёк, что-то ещё работает). ms <= 0 - вернуть немедленно (истина только
// если счётчик и так уже 0) - тот же смысл нулевого/отрицательного
// таймаута, что и у rech_set_timeout (builtins_shared.go).
static int rech_wait_all_tasks(long long ms) {
    RECH_ACTIVE_TASKS_LOCK();
#ifdef _WIN32
    DWORD deadline_ms = ms > 0 ? (DWORD)ms : 0;
    while (rech_active_tasks_count > 0) {
        if (!SleepConditionVariableCS(&rech_active_tasks_cond, &rech_active_tasks_mutex, deadline_ms)) {
            break;
        }
    }
#else
    if (rech_active_tasks_count > 0 && ms > 0) {
        struct timespec ts;
` + timespec.DeadlineCode("ts", "ms") + `
        while (rech_active_tasks_count > 0) {
            if (pthread_cond_timedwait(&rech_active_tasks_cond, &rech_active_tasks_mutex, &ts) != 0) {
                break;
            }
        }
    }
#endif
    int ok = (rech_active_tasks_count == 0);
    RECH_ACTIVE_TASKS_UNLOCK();
    return ok;
}

`
