// Примитив лёгких задач (корутин) поверх ucontext(3) - фундамент для
// асинхронной модели поверх "асинхронная функция" (docs/ROADMAP.md, Шаг
// 127, инкремент 1 - десугар в "запустить"/"дождаться" на потоках ОС).
// Инкремент 2 (Шаг 128) дал сам примитив переключения (rech_coro_spawn/
// rech_coro_yield/rech_coro_run_all, только очередь готовых, без I/O).
// Инкремент 3 (Шаг 129) добавил ожидание готовности файлового дескриптора
// через epoll (Linux)/kqueue (macOS): rech_coro_wait_readable/
// rech_coro_wait_writable уступают планировщику ТАК ЖЕ, как rech_coro_yield,
// но возобновляются не на следующем же обороте очереди, а когда poll-
// инстанс сообщит о реальной готовности fd - именно этот механизм и даёт
// экономию по сравнению с потоком ОС на соединение: тысячи корутин могут
// ждать разных сокетов одновременно, ничего не блокируя, кроме одного
// вызова epoll_wait/kevent на всех сразу. POSIX-only (ucontext.h/epoll/
// kqueue не существуют на Windows) - согласовано заранее с пользователем:
// первая итерация не включает Windows вовсе (см.
// [[project_language_gaps_priority_plan]] в памяти сессии, не дублируется
// в самом репозитории).
//
// Инкремент 4 (Шаг 130) - ПЕРВОЕ реальное подключение: "дождаться
// <асинхронная функция>(...)" (internal/backend/cbackend/coroutine_task.go,
// ensureCoroTaskWrapper) теперь исполняет НАСТОЯЩЕЕ тело Rech-функции на
// стеке корутины через rech_coro_wait_done - спавнит, крутит планировщик
// ровно до её завершения (не обязательно до полного опустошения очереди -
// см. rech_coro_wait_done) и возвращает результат/пробрасывает исключение,
// тем же контрактом (rech_throw, тот же catch-frame через setjmp), что и
// поток ОС (concurrency.go). Явный "запустить" на асинхронной функции
// по-прежнему идёт через поток ОС - модель под ним не меняется никогда
// (ir.LaunchExpr.Coroutine, package doc там же).
//
// GC-корни стека корутины (открытое ограничение инкрементов 2-3, решено
// здесь): весь стек каждой корутины - дополнительный корень сканирования
// Boehm GC (GC_add_roots) - без этого указатель, живущий только на стеке
// корутины между двумя переключениями (обычный локальный GC_MALLOC'нутый
// объект внутри тела асинхронной функции), был бы невидим сборщику и
// рисковал быть собран как мусор. `<gc.h>` доступен здесь ПОТОМУ ЧТО
// ctx.usesCoroutines входит в общее условие его подключения
// (ir_generator.go) - этот блок эмитится ТОЛЬКО когда хотя бы одна
// "асинхронная функция" реально ожидается через "дождаться", а это уже
// само по себе требует GC (структура задачи - GC_MALLOC, coroutine_task.go).
// ВАЖНО - регистрация НЕ по одной на корутину (см. package doc про арены
// стека ниже, rech_coro_stack_alloc - Boehm GC абортит процесс "Too many
// root sets" уже на паре тысяч отдельных регистраций, живой найденный баг
// инкремента 6, Шаг 132) - по одной на АРЕНУ (256 стеков каждая),
// стеки НАРЕЗАЮТСЯ из арен и переиспользуются через free-list.
//
// Вложенный "дождаться <асинхронная функция>" ИЗНУТРИ уже исполняющейся
// корутины ПОКА НЕ ПОДДЕРЖАН - состояние планировщика (rech_sched_ctx/
// rech_coro_current/очередь) глобальное на процесс, не стек контекстов;
// наивный рекурсивный вызов rech_coro_wait_done corrupted бы внешний
// rech_sched_ctx. rech_coro_wait_done проверяет rech_coro_current != NULL
// на входе и паникует явно (rech_panic), а не тихо портит состояние.
// Условие ДИНАМИЧЕСКОЕ (зависит от того, была ли ТЕКУЩАЯ дин. цепочка
// вызовов на самом деле начата через "дождаться <асинхронная функция>()",
// а не от того, объявлена ли объемлющая функция "асинхронная" - прямой
// вызов такой функции синхронен и НЕ на корутине вовсе) - поэтому НЕТ и
// не может быть чисто статической (компиляционной) проверки на этот
// случай, см. package doc internal/semantic/concurrency.go,
// analyzeAwaitTask. **Осознанное узкое расхождение с интерпретатором**:
// `internal/interp` не имеет этого ограничения вовсе - "запустить"/
// "дождаться" там всегда были обычными Go-горутинами (внутренний планировщик
// Go поддерживает произвольную вложенность нативно, без единой строчки
// собственного кода) - см. package doc internal/interp/concurrency.go.
// Единственный наблюдаемый разрыв правила "оба backend'а дают идентичный
// результат" во всём этом плане - редкий, специально построенный сценарий
// (асинхронная функция, вызванная ЧЕРЕЗ "дождаться", изнутри которой
// (прямо или через обычную функцию-помощника) вызывается ЕЩЁ одно
// "дождаться <асинхронная функция>") - снятие ограничения (стек вложенных
// планировщиков вместо одной глобальной пары переменных) - открытый пункт
// бэклога, не в этом инкременте.
//
// Инкремент 6 (Шаг 132, последний в плане) - "поручить <асинхронная
// функция>(...)": ФОНОВЫЙ запуск, БЕЗ немедленного ожидания (в отличие от
// сахара "дождаться", который спавнит и сразу же зовёт rech_coro_wait_done -
// см. coroutine_task.go). Ради этого rech_coro_await_run (было) разделён
// на rech_coro_spawn (сам по себе, генерик, был и раньше) + rech_coro_
// wait_done (просто ждёт *done, ничего не спавнит) - "поручить" зовёт
// ТОЛЬКО rech_coro_spawn (через per-функциональную rech_coro_launch_<имя>,
// coroutine_task.go) и возвращает управление немедленно; "дождаться" над
// уже полученной так задачей зовёт ТОЛЬКО rech_coro_wait_done (через
// rech_coro_join_<имя>). ВАЖНО: сам rech_coro_spawn безопасно вызывать
// ИЗНУТРИ уже исполняющейся корутины (просто кладёт новую корутину в ту
// же очередь ЭТОГО ЖЕ потока - никакого swapcontext/rech_sched_ctx он не
// трогает) - конфликтует ("вложенный дождаться", выше) ИМЕННО ожидание
// (rech_coro_wait_done), не сам спавн. Именно "поручить" даёт настоящую
// одновременность нескольких соединений на одном планировщике (accept-цикл
// сервера поручает обработку и сразу возвращается к следующему accept) -
// сценарий, ради которого весь этот план и затевался (examples/проекты/
// центр_уведомлений).
//
// Таймауты ожидания (fd так и не стал готов) - по-прежнему сознательно не
// решены, epoll_wait/kevent ждут бесконечно (-1/NULL) - нет живого
// сценария, который бы их потребовал уже сейчас (TCP-builtin'ы ещё не
// переведены на эту модель - открытый пункт следующего инкремента).
package cbackend

// coroutineRuntimeCode - см. package doc выше. RECH_CORO_STACK_SIZE -
// намеренно консервативный дефолт (256 КБ) - фиксированный, не растущий
// стек (в отличие от горутин Go): ucontext не даёт дешёвого способа расти
// постфактум, точный размер имеет смысл калибровать только когда на нём
// реально начнёт исполняться тело Rech-функции (следующие шаги плана), не
// сейчас.
const coroutineRuntimeCode = `#ifndef _WIN32
#define _XOPEN_SOURCE 700
#include <ucontext.h>
#include <stdlib.h>
#include <errno.h>
#if defined(__linux__)
#include <sys/epoll.h>
#elif defined(__APPLE__)
#include <sys/types.h>
#include <sys/event.h>
#endif

#define RECH_CORO_STACK_SIZE (256 * 1024)

// RECH_THREAD_LOCAL - та же роль, что и в ir_generator.go (там определён
// безусловно ОДИН раз для всей сгенерированной программы) - здесь
// ПОДСТРАХОВОЧНО, guard'ом: coroutineRuntimeCode компилируется и САМ ПО
// СЕБЕ, изолированным C-driver'ом (coroutine_test.go, без остального
// рантайма Rech вовсе) - без собственного определения макрос был бы не
// объявлен вовсе в этом сценарии. В полном конвейере (ir_generator.go)
// #ifndef молча пропускает переопределение - идентичное значение,
// объявленное раньше.
#ifndef RECH_THREAD_LOCAL
#if defined(_WIN32)
#define RECH_THREAD_LOCAL __declspec(thread)
#else
#define RECH_THREAD_LOCAL __thread
#endif
#endif

typedef struct rech_coro {
    ucontext_t ctx;
    void *stack;
    void (*fn)(void *);
    void *arg;
    int finished;
    struct rech_coro *next;
} rech_coro_t;

// Всё состояние планировщика ниже - RECH_THREAD_LOCAL: КАЖДЫЙ поток ОС,
// который когда-либо вызывает "дождаться <асинхронная функция>(...)"
// (главный поток программы, ЛЮБОЙ поток из общего пула "запустить" -
// threadpool.go), получает СОБСТВЕННЫЙ, полностью независимый планировщик,
// ни с кем не разделяемый - ровно "несколько независимых event loop'ов,
// как воркеры nginx" (M:N), выбранное пользователем в самом начале этого
// плана. Не подстраховка "на будущее" - НАЙДЕННЫЙ ЖИВОЙ БАГ инкремента 4:
// без RECH_THREAD_LOCAL это были голые process-wide static-переменные -
// главный поток и одновременно исполняющийся "запустить"-поток, оба
// вызвавшие "дождаться <асинхронная функция>()" одновременно, гонялись за
// одним и тем же rech_sched_ctx/rech_coro_current/очередью без единой
// блокировки. Воспроизведено вживую (50 параллельных "запустить"-воркеров,
// каждый "дождаться" свою асинхронную функцию) ДО этого исправления - три
// прогона дали три разных отказа (segfault; ложное "вложенный дождаться"
// из испорченного rech_coro_current; один прогон случайно не столкнулся) -
// классическая гонка, не гипотетический риск.
//
// rech_sched_ctx - контекст самого планировщика ЭТОГО потока
// (rech_coro_run_all) - цель "uc_link" каждой корутины (возврат тела
// функции переключает СЮДА автоматически, без явного swapcontext в
// трамплине) и цель swapcontext из rech_coro_yield.
static RECH_THREAD_LOCAL ucontext_t rech_sched_ctx;
static RECH_THREAD_LOCAL rech_coro_t *rech_coro_queue_head = NULL;
static RECH_THREAD_LOCAL rech_coro_t *rech_coro_queue_tail = NULL;
// rech_coro_current - корутина, исполняющаяся ПРЯМО СЕЙЧАС на этом потоке
// ОС - NULL, когда управление у самого планировщика (между итерациями
// цикла rech_coro_run_all). rech_coro_yield читает её, чтобы знать, КОГО
// поставить обратно в очередь перед уступкой.
static RECH_THREAD_LOCAL rech_coro_t *rech_coro_current = NULL;

static void rech_coro_enqueue(rech_coro_t *co) {
    co->next = NULL;
    if (rech_coro_queue_tail != NULL) {
        rech_coro_queue_tail->next = co;
    } else {
        rech_coro_queue_head = co;
    }
    rech_coro_queue_tail = co;
}

static rech_coro_t *rech_coro_dequeue(void) {
    rech_coro_t *co = rech_coro_queue_head;
    if (co != NULL) {
        rech_coro_queue_head = co->next;
        if (rech_coro_queue_head == NULL) {
            rech_coro_queue_tail = NULL;
        }
    }
    return co;
}

// rech_coro_trampoline - реальная точка входа стека корутины (см.
// makecontext ниже) - "этот" поймать неоткуда (ucontext не передаёт
// аргументы типобезопасно через makecontext на всех платформах), поэтому
// берёт себя из rech_coro_current, который rech_coro_run_all выставляет
// СРАЗУ ПЕРЕД swapcontext, до того как управление реально перейдёт сюда.
// Обычный (не через rech_coro_yield) возврат из fn() переключает контекст
// назад в rech_sched_ctx АВТОМАТИЧЕСКИ, через uc_link - явный swapcontext
// здесь не нужен.
static void rech_coro_trampoline(void) {
    rech_coro_t *co = rech_coro_current;
    co->fn(co->arg);
    co->finished = 1;
}

// Пул стеков (арена) - НАЙДЕННЫЙ ЖИВОЙ БАГ инкремента 6 (Шаг 132):
// GC_add_roots НА КАЖДУЮ корутину отдельно (было в инкрементах 4-5) upasl
// настоящий стресс-тест на 3000 одновременных соединений - "Too many root
// sets" (Boehm GC реально абортит процесс, у таблицы корневых сегментов
// небольшая ФИКСИРОВАННАЯ ёмкость - эмпирически проверено отдельным
// C-репродюсером, падает уже на ~2000-2100 отдельных, не смежных
// регистраций). Отдельная регистрация на каждую из десятков тысяч
// корутин, которые и есть весь смысл этого плана (сценарий 50-100 тыс.
// соединений), несовместима с этим лимитом в принципе - решение
// (стандартный приём для конкурентных рантаймов поверх Boehm GC, тот же
// принцип, что и у арен стека в других зелёно-поточных рантаймах) -
// стеки НЕ malloc'ятся по одному, а НАРЕЗАЮТСЯ из больших блоков-арен
// (RECH_CORO_ARENA_STACKS стеков в каждой), КАЖДАЯ АРЕНА ЦЕЛИКОМ - РОВНО
// ОДНА регистрация GC_add_roots, не одна на стек. Свободные (уже
// завершившиеся) стеки возвращаются в простой односвязный список
// (указатель на следующий свободный хранится В САМИХ первых байтах
// освобождённого стека - классический приём free-list, безопасно: в
// момент возврата стек уже гарантированно не используется как чей-либо
// стек исполнения) и переиспользуются ПЕРВЫМИ, раньше, чем нарезается
// новый слот из арены - число арен (и значит, число GC_add_roots за всё
// время жизни процесса) растёт с ПИКОВЫМ числом ОДНОВРЕМЕННО живых
// корутин на этом потоке, не с суммарным числом когда-либо созданных -
// при 256 стеках на арену пиковые 100 тыс. корутин - это ~390 арен,
// далеко от предела в ~2000-2100. Ничего не отдаётся GC_remove_roots'ом
// НИКОГДА - арены живут до конца процесса, это осознанно (снятие и
// повторная выдача той же области были бы лишним усложнением ради
// экономии, которая здесь не нужна - память самих арен и так уже
// посчитана, попадать в лимит родительских регистраций ей не нужно).
#define RECH_CORO_ARENA_STACKS 256

typedef struct rech_coro_stack_arena {
    char *base;
    long next_free_index;
    struct rech_coro_stack_arena *prev;
} rech_coro_stack_arena_t;

static RECH_THREAD_LOCAL rech_coro_stack_arena_t *rech_coro_cur_arena = NULL;
// rech_coro_free_stack_list - свободные стеки, ГОТОВЫЕ К ПОВТОРНОЙ ВЫДАЧЕ,
// этого же потока - указатель на следующий свободный лежит в первых
// sizeof(void*) байтах самого стека (см. rech_coro_stack_free).
static RECH_THREAD_LOCAL void *rech_coro_free_stack_list = NULL;

static void *rech_coro_stack_alloc(void) {
    if (rech_coro_free_stack_list != NULL) {
        void *s = rech_coro_free_stack_list;
        rech_coro_free_stack_list = *(void **)s;
        return s;
    }
    if (rech_coro_cur_arena == NULL || rech_coro_cur_arena->next_free_index >= RECH_CORO_ARENA_STACKS) {
        rech_coro_stack_arena_t *a = (rech_coro_stack_arena_t *)malloc(sizeof(rech_coro_stack_arena_t));
        size_t arena_bytes = (size_t)RECH_CORO_ARENA_STACKS * RECH_CORO_STACK_SIZE;
        a->base = (char *)malloc(arena_bytes);
        GC_add_roots(a->base, a->base + arena_bytes);
        a->next_free_index = 0;
        a->prev = rech_coro_cur_arena;
        rech_coro_cur_arena = a;
    }
    void *s = rech_coro_cur_arena->base + rech_coro_cur_arena->next_free_index * RECH_CORO_STACK_SIZE;
    rech_coro_cur_arena->next_free_index++;
    return s;
}

static void rech_coro_stack_free(void *stack) {
    *(void **)stack = rech_coro_free_stack_list;
    rech_coro_free_stack_list = stack;
}

// rech_coro_spawn - создаёт корутину (стек - см. арену выше, не отдельный
// malloc/GC_add_roots на каждую) и сразу ставит её в очередь готовых к
// выполнению - не запускает немедленно, реальный первый запуск - только
// внутри rech_coro_step.
static void rech_coro_spawn(void (*fn)(void *), void *arg) {
    rech_coro_t *co = (rech_coro_t *)malloc(sizeof(rech_coro_t));
    co->stack = rech_coro_stack_alloc();
    co->fn = fn;
    co->arg = arg;
    co->finished = 0;
    co->next = NULL;
    getcontext(&co->ctx);
    co->ctx.uc_stack.ss_sp = co->stack;
    co->ctx.uc_stack.ss_size = RECH_CORO_STACK_SIZE;
    co->ctx.uc_link = &rech_sched_ctx;
    makecontext(&co->ctx, rech_coro_trampoline, 0);
    rech_coro_enqueue(co);
}

// rech_coro_yield - вызывается ИЗНУТРИ тела корутины (fn, прямо или через
// цепочку обычных C-вызовов на её же стеке) - кооперативно уступает
// планировщику: ставит себя обратно в конец очереди готовых и
// приостанавливается ровно на этом месте, до следующего раза, когда
// планировщик снова выберет её. Единственная на этом шаге причина уступить -
// следующий шаг плана (epoll/kqueue) добавит уступку по ожиданию
// готовности сокета вместо/вдобавок к этой ручной точке.
static void rech_coro_yield(void) {
    rech_coro_t *co = rech_coro_current;
    rech_coro_enqueue(co);
    swapcontext(&co->ctx, &rech_sched_ctx);
}

// rech_coro_waiter - привязывает fd, ожидаемый ОДНОЙ корутиной, к самой
// этой корутине - epoll/kqueue возвращают указатель на этот же waiter
// внутри самого события (ev.data.ptr/kevent.udata), поэтому отдельная
// таблица "fd -> корутина" не нужна вовсе - демультиплексация бесплатна.
typedef struct rech_coro_waiter {
    rech_coro_t *co;
} rech_coro_waiter_t;

// rech_coro_poll_fd - инстанс epoll/kqueue, ОДИН НА ПОТОК (RECH_THREAD_LOCAL,
// см. общее объяснение выше - тем же принципом, что и остальное состояние
// планировщика: общий на процесс инстанс означал бы, что событие,
// зарегистрированное потоком A, может достаться epoll_wait/kevent потока
// B, который попытался бы поставить ЧУЖУЮ корутину в СВОЮ локальную
// очередь - создаётся лениво при первом вызове ожидания на ЭТОМ потоке
// (поток, не использующий ни одной "асинхронная функция" с сетью, не
// платит за него вовсе).
static RECH_THREAD_LOCAL int rech_coro_poll_fd = -1;
// rech_coro_pending_waiters - сколько корутин ЭТОГО потока сейчас ждут
// готовности какого-то fd (не в очереди готовых) - rech_coro_run_all
// различает по нему "очередь опустела, потому что вся работа сделана"
// (0 - выйти) от "очередь опустела, но кто-то ждёт I/O" (>0 -
// заблокироваться на epoll_wait/kevent, не выходить).
static RECH_THREAD_LOCAL int rech_coro_pending_waiters = 0;

static void rech_coro_poll_init(void) {
    if (rech_coro_poll_fd != -1) {
        return;
    }
#if defined(__linux__)
    rech_coro_poll_fd = epoll_create1(0);
#elif defined(__APPLE__)
    rech_coro_poll_fd = kqueue();
#endif
}

// rech_coro_wait_fd - общая реализация rech_coro_wait_readable/writable -
// НЕ бесхитростный bitmask "читаемость ИЛИ записываемость сразу" (были бы
// на kqueue ДВЕ независимые записи kevent с одним и тем же udata - если бы
// обе стали готовы одновременно, epoll_wait/kevent вернули бы ДВА события
// на один и тот же waiter, дважды поставив одну и ту же корутину в
// очередь). Ровно один интерес на вызов убирает саму возможность такой
// гонки - вызывающему коду (следующий шаг плана), которому одновременно
// нужны оба направления, придётся звать оба ожидания по отдельности.
// EPOLLONESHOT/EV_ONESHOT - однократное срабатывание, без явного снятия
// регистрации самим вызывающим кодом; повторный вызов на тот же fd -
// ADD, который на Linux при уже существующей регистрации возвращает
// EEXIST (сам epoll не разрешает повторный ADD), тогда - MOD; kqueue
// разрешает повторный EV_ADD без ошибки, отдельной ветки не нужно.
static void rech_coro_wait_fd(int fd, int writable) {
    rech_coro_t *co = rech_coro_current;
    rech_coro_poll_init();
    rech_coro_waiter_t *w = (rech_coro_waiter_t *)malloc(sizeof(rech_coro_waiter_t));
    w->co = co;
#if defined(__linux__)
    struct epoll_event ev;
    ev.events = (writable ? EPOLLOUT : EPOLLIN) | EPOLLONESHOT;
    ev.data.ptr = w;
    if (epoll_ctl(rech_coro_poll_fd, EPOLL_CTL_ADD, fd, &ev) == -1 && errno == EEXIST) {
        epoll_ctl(rech_coro_poll_fd, EPOLL_CTL_MOD, fd, &ev);
    }
#elif defined(__APPLE__)
    struct kevent change;
    EV_SET(&change, fd, writable ? EVFILT_WRITE : EVFILT_READ, EV_ADD | EV_ONESHOT, 0, 0, w);
    kevent(rech_coro_poll_fd, &change, 1, NULL, 0, NULL);
#endif
    rech_coro_pending_waiters++;
    swapcontext(&co->ctx, &rech_sched_ctx);
    free(w);
}

// rech_coro_wait_readable/rech_coro_wait_writable - см. rech_coro_wait_fd.
static void rech_coro_wait_readable(int fd) {
    rech_coro_wait_fd(fd, 0);
}
static void rech_coro_wait_writable(int fd) {
    rech_coro_wait_fd(fd, 1);
}

// rech_coro_poll_once - вызывается ТОЛЬКО из rech_coro_run_all, когда
// очередь готовых пуста, но rech_coro_pending_waiters > 0 - единственное
// место, где планировщик реально блокируется (epoll_wait/kevent без
// таймаута - см. package doc про открытый вопрос таймаутов): по одному
// системному вызову обслуживает ЛЮБОЕ число одновременно ожидающих
// корутин разом, в этом и есть экономия по сравнению с потоком ОС на
// каждую.
static void rech_coro_poll_once(void) {
#if defined(__linux__)
    struct epoll_event events[64];
    int n = epoll_wait(rech_coro_poll_fd, events, 64, -1);
    for (int i = 0; i < n; i++) {
        rech_coro_waiter_t *w = (rech_coro_waiter_t *)events[i].data.ptr;
        rech_coro_pending_waiters--;
        rech_coro_enqueue(w->co);
    }
#elif defined(__APPLE__)
    struct kevent events[64];
    int n = kevent(rech_coro_poll_fd, NULL, 0, events, 64, NULL);
    for (int i = 0; i < n; i++) {
        rech_coro_waiter_t *w = (rech_coro_waiter_t *)events[i].udata;
        rech_coro_pending_waiters--;
        rech_coro_enqueue(w->co);
    }
#endif
}

// rech_coro_step - ОДИН оборот планировщика: если очередь готовых пуста,
// но остались корутины, ждущие I/O (rech_coro_pending_waiters > 0) -
// блокируется на rech_coro_poll_once (сделал работу - вернёт 1); если
// пусто совсем - нечего делать, возвращает 0. Иначе выполняет ОДНУ
// корутину до её следующей уступки/завершения. Завершившаяся
// (co->finished) корутина освобождается здесь же - стек возвращается в
// пул (rech_coro_stack_free, НЕ free() - см. package doc про арены выше:
// сама память арены никогда не освобождается процессу, переиспользуется)
// - ей больше некуда вернуться, uc_link уже отработал автоматическим
// переключением назад в rech_sched_ctx к моменту, когда swapcontext ниже
// возвращает управление. Вынесен из rech_coro_run_all отдельной функцией
// ради rech_coro_wait_done ниже - тот крутит планировщик ПО ШАГУ, ровно
// до завершения ОДНОЙ конкретной задачи, не обязательно до полного
// опустошения очереди.
static int rech_coro_step(void) {
    rech_coro_t *co = rech_coro_dequeue();
    if (co == NULL) {
        if (rech_coro_pending_waiters > 0) {
            rech_coro_poll_once();
            return 1;
        }
        return 0;
    }
    rech_coro_current = co;
    swapcontext(&rech_sched_ctx, &co->ctx);
    rech_coro_current = NULL;
    if (co->finished) {
        rech_coro_stack_free(co->stack);
        free(co);
    }
    return 1;
}

// rech_coro_run_all - однопоточный планировщик: крутит очередь готовых
// корутин до полного опустошения (rech_coro_step возвращает 0 - ни
// готовых, ни ожидающих I/O не осталось).
static void rech_coro_run_all(void) {
    while (rech_coro_step()) {
    }
}

// rech_coro_wait_done - крутит планировщик ПОШАГОВО ровно до того, как
// *done станет ненулевым (поле, которое энтри-точка корутины, ранее уже
// запущенной rech_coro_spawn - НЕ здесь, см. package doc coroutine_task.go
// про "поручить"/rech_coro_launch_<имя> - выставляет перед возвратом),
// не обязательно до полного опустошения очереди целиком (в отличие от
// rech_coro_run_all) - на очереди могут стоять и ДРУГИЕ, не связанные с
// этим ожиданием корутины (Шаг 132, "поручить" много раз подряд, "ждаться"
// только одну из них). Сам СПАВН здесь не происходит - разделено намеренно
// (см. package doc coroutine_task.go): "поручить" спавнит немедленно и
// возвращается сразу же, "дождаться" (над уже спавненной или только что
// созданной сахаром задачей) вызывает ИМЕННО эту функцию.
//
// Проверка rech_coro_current != NULL - защита от ВЛОЖЕННОГО "дождаться
// <асинхронная функция>" изнутри уже исполняющейся корутины - см. package
// doc coroutine.go, открытое ограничение. Именно ОЖИДАНИЕ (не спавн - тот
// безопасен и изнутри корутины тоже, просто кладёт в ту же очередь этого
// же потока) конфликтует с уже активным swapcontext на этом потоке.
// rech_coro_step() == 0 внутри цикла (очередь опустела, а конкретная
// задача так и не завершилась) - внутренняя ошибка планировщика, не
// пользовательской программы (при корректной entry невозможно) - явная
// паника вместо тихого зависания/неопределённого результата.
static void rech_coro_wait_done(int *done) {
    if (rech_coro_current != NULL) {
        rech_panic("вложенный \"дождаться\" асинхронной функции изнутри другой асинхронной функции пока не поддерживается");
    }
    while (!*done) {
        if (!rech_coro_step()) {
            rech_panic("внутренняя ошибка: планировщик корутин опустел, не завершив задачу");
        }
    }
}
#endif
`
