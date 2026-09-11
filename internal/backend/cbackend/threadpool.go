// Пул воркер-потоков, обслуживающий "запустить" (concurrency.go) - вместо
// pthread_create/CreateThread НА КАЖДЫЙ вызов "запустить", каждая задача
// теперь встаёт в очередь (rech_pool_submit), которую разбирают уже
// существующие потоки-воркеры; новый воркер создаётся только когда СВОБОДНЫХ
// (простаивающих) воркеров в моменте нет. Пул растёт до пикового числа
// когда-либо ОДНОВРЕМЕННО выполнявшихся задач - пиковая конкурентность
// программы не меняется (столько же потоков может работать разом, сколько и
// раньше), но повторная нагрузка (после разгона) переиспользует уже
// созданные потоки вместо create/destroy на каждое соединение - именно этот
// накладной расход и был найденным узким местом ("что ещё
// нужно для настоящего HTTP highload", пункт 1).
//
// По умолчанию пул НИКОГДА не сжимается (см. Шаг 80) - "потоки.сжатие_пула:
// истина" файла проекта (internal/project.File.PoolShrinkEnabled,
// ir.Program.PoolShrinkEnabled) включает сжатие: воркер, простоявший без
// работы дольше "потоки.таймаут_простоя_мс" (ir.Program.PoolIdleTimeoutMs,
// по умолчанию internal/semantic.DefaultPoolIdleTimeoutMs), сам завершается.
// Оба параметра запекаются в код препроцессорными макросами
// (RECH_POOL_SHRINK_ENABLED/RECH_POOL_IDLE_TIMEOUT_MS) - тот же приём, что и
// RECH_DB_POOL_TIMEOUT_MS у rech_db_checkout (db.go): выбор веток решается
// компилятором C (`#if`), не рантайм-условием, поэтому "сжатие выключено" -
// БУКВАЛЬНО тот же код, что был до появления этой настройки, без единого
// лишнего сравнения на горячем пути.
//
// НЕ параметризован по конкретной запущенной функции (в отличие от
// per-функции rech_task_entry_<имя>, ensureTaskWrapper) - один общий пул на
// весь процесс, независимо от того, сколько РАЗНЫХ функций когда-либо
// запущено "запустить": элемент очереди несёт голый указатель на функцию
// (та же сигнатура, что и у точки входа потока, см. ниже) плюс аргумент
// (указатель на структуру задачи, GC_MALLOC'нутую в rech_launch_...,
// concurrency.go) - воркеру не нужно ничего знать про КОНКРЕТНУЮ обёрнутую
// функцию программы, он просто зовёт то, что ему передали.
//
// Сигнатура элемента очереди - LPTHREAD_START_ROUTINE (DWORD WINAPI
// (*)(LPVOID)) на Windows, void *(*)(void *) на POSIX - РОВНО та же, что и у
// rech_task_entry_<имя> (concurrency.go, entry) - тело воркера просто зовёт
// то, что стоит в очереди, результат (само значение задачи, исключение)
// точка входа кладёт в структуру задачи и сигналит done САМА, воркеру
// возвращаемое значение entry не нужно вовсе.
//
// Мьютекс/condvar - тот же приём ленивой инициализации, что и
// rech_active_tasks_mutex (active_tasks.go)/rech_tls_lock (tls.go) -
// CRITICAL_SECTION нуждается в явном InitializeCriticalSection (в отличие от
// CONDITION_VARIABLE, у которой есть статический инициализатор).
//
// Гонка "воркер как раз решил завершиться по таймауту, а submit() в этот же
// момент увидел его как простаивающего" НЕ теряет задачу: pthread_cond_
// timedwait/SleepConditionVariableCS ВСЕГДА реацквайрят мьютекс перед
// возвратом (даже при ETIMEDOUT/FALSE) - воркер перепроверяет
// "rech_pool_head == NULL" уже ПОСЛЕ этого, под тем же мьютексом, что и
// submit() использовал для постановки элемента в очередь. Если элемент
// успел встать в очередь ДО того, как воркер перепроверил условие (а он
// обязан был встать под тем же мьютексом раньше, чем submit() принял
// решение не создавать новый поток) - воркер увидит его и продолжит
// работать, а не завершится - ложного "потерянного" пробуждения нет.
package cbackend

import "fmt"

// threadPoolCode - см. package doc выше.
func threadPoolCode(shrinkEnabled bool, idleTimeoutMs int) string {
	shrinkFlag := 0
	if shrinkEnabled {
		shrinkFlag = 1
	}
	deadlineCode := timespecDeadlineCode("rech_pool_deadline", "RECH_POOL_IDLE_TIMEOUT_MS")
	return fmt.Sprintf(`typedef struct rech_pool_item {
#ifdef _WIN32
    LPTHREAD_START_ROUTINE fn;
#else
    void *(*fn)(void *);
#endif
    void *arg;
    struct rech_pool_item *next;
} rech_pool_item_t;

static rech_pool_item_t *rech_pool_head = NULL;
static rech_pool_item_t *rech_pool_tail = NULL;
static int rech_pool_idle = 0;

#define RECH_POOL_SHRINK_ENABLED %[1]d
#define RECH_POOL_IDLE_TIMEOUT_MS %[2]d

#ifdef _WIN32
static CRITICAL_SECTION rech_pool_mutex;
static volatile LONG rech_pool_mutex_ready = 0;
static void rech_pool_mutex_init(void) {
    if (InterlockedCompareExchange(&rech_pool_mutex_ready, 1, 0) == 0) {
        InitializeCriticalSection(&rech_pool_mutex);
    }
}
static CONDITION_VARIABLE rech_pool_cond = CONDITION_VARIABLE_INIT;
#define RECH_POOL_LOCK() (rech_pool_mutex_init(), EnterCriticalSection(&rech_pool_mutex))
#define RECH_POOL_UNLOCK() LeaveCriticalSection(&rech_pool_mutex)
#else
static pthread_mutex_t rech_pool_mutex = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t rech_pool_cond = PTHREAD_COND_INITIALIZER;
#define RECH_POOL_LOCK() pthread_mutex_lock(&rech_pool_mutex)
#define RECH_POOL_UNLOCK() pthread_mutex_unlock(&rech_pool_mutex)
#endif

#ifdef _WIN32
static DWORD WINAPI rech_pool_worker(LPVOID rech_unused) {
#else
static void *rech_pool_worker(void *rech_unused) {
#endif
    (void)rech_unused;
    for (;;) {
        RECH_POOL_LOCK();
        rech_pool_idle++;
#if !defined(_WIN32) && RECH_POOL_SHRINK_ENABLED
        struct timespec rech_pool_deadline;
%[3]s#endif
        while (rech_pool_head == NULL) {
#if defined(_WIN32) && RECH_POOL_SHRINK_ENABLED
            if (!SleepConditionVariableCS(&rech_pool_cond, &rech_pool_mutex, RECH_POOL_IDLE_TIMEOUT_MS) && rech_pool_head == NULL) {
                rech_pool_idle--;
                RECH_POOL_UNLOCK();
                return 0;
            }
#elif defined(_WIN32)
            SleepConditionVariableCS(&rech_pool_cond, &rech_pool_mutex, INFINITE);
#elif RECH_POOL_SHRINK_ENABLED
            if (pthread_cond_timedwait(&rech_pool_cond, &rech_pool_mutex, &rech_pool_deadline) != 0 && rech_pool_head == NULL) {
                rech_pool_idle--;
                RECH_POOL_UNLOCK();
                return NULL;
            }
#else
            pthread_cond_wait(&rech_pool_cond, &rech_pool_mutex);
#endif
        }
        rech_pool_idle--;
        rech_pool_item_t *rech_item = rech_pool_head;
        rech_pool_head = rech_item->next;
        if (rech_pool_head == NULL) {
            rech_pool_tail = NULL;
        }
        RECH_POOL_UNLOCK();
        rech_item->fn(rech_item->arg);
    }
#ifdef _WIN32
    return 0;
#else
    return NULL;
#endif
}

#ifdef _WIN32
static void rech_pool_submit(LPTHREAD_START_ROUTINE fn, void *arg) {
#else
static void rech_pool_submit(void *(*fn)(void *), void *arg) {
#endif
    rech_pool_item_t *item = (rech_pool_item_t *)GC_MALLOC(sizeof(rech_pool_item_t));
    item->fn = fn;
    item->arg = arg;
    item->next = NULL;
    RECH_POOL_LOCK();
    if (rech_pool_tail != NULL) {
        rech_pool_tail->next = item;
    } else {
        rech_pool_head = item;
    }
    rech_pool_tail = item;
    int rech_need_worker = (rech_pool_idle == 0);
    RECH_POOL_UNLOCK();
    if (rech_need_worker) {
#ifdef _WIN32
        HANDLE rech_worker_handle = CreateThread(NULL, 0, rech_pool_worker, NULL, 0, NULL);
        CloseHandle(rech_worker_handle);
#else
        pthread_t rech_worker_handle;
        pthread_create(&rech_worker_handle, NULL, rech_pool_worker, NULL);
        pthread_detach(rech_worker_handle);
#endif
    } else {
#ifdef _WIN32
        WakeConditionVariable(&rech_pool_cond);
#else
        pthread_cond_signal(&rech_pool_cond);
#endif
    }
}

`, shrinkFlag, idleTimeoutMs, deadlineCode)
}
