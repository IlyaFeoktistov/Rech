// Примитив лёгких задач (корутин) поверх ucontext(3) - фундамент для
// асинхронной модели поверх "асинхронная функция" (Шаг
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
// корутины ПОДДЕРЖАН (Шаг 132, "дожато" по прямой просьбе пользователя
// сразу следом за первой версией этого файла, где было заявлено обратное) -
// см. package doc rech_coro_t.resume_ctx/rech_coro_step: каждый вложенный
// вызов rech_coro_step получает СОБСТВЕННУЮ, лежащую на СВОЁМ месте
// C-стека вызовов точку возврата, не одну общую на процесс/поток
// rech_sched_ctx (которая и была первой, наивной причиной, почему
// вложенность не работала) - никакой рекурсивной порчи состояния,
// проверять/паниковать не на чем: `internal/interp` (Go-горутины) и
// cbackend теперь ведут себя одинаково на этом сценарии, расхождения нет.
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
package concurrency

// CoroutineRuntimeCode - см. package doc выше. RECH_CORO_STACK_SIZE -
// намеренно консервативный дефолт (256 КБ) - фиксированный, не растущий
// стек (в отличие от горутин Go): ucontext не даёт дешёвого способа расти
// постфактум, точный размер имеет смысл калибровать только когда на нём
// реально начнёт исполняться тело Rech-функции (следующие шаги плана), не
// сейчас.
const CoroutineRuntimeCode = `#ifndef _WIN32
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
// ПОДСТРАХОВОЧНО, guard'ом: CoroutineRuntimeCode компилируется и САМ ПО
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
    // resume_ctx - куда переключиться при уступке (rech_coro_yield/
    // rech_coro_wait_fd) или обычном завершении тела (rech_coro_trampoline) -
    // см. package doc про вложенный "дождаться" ниже: ПЕРЕУСТАНАВЛИВАЕТСЯ
    // ЗАНОВО КАЖДЫЙ РАЗ, когда rech_coro_step берёт эту корутину на
    // исполнение (не один раз при спавне) - именно это и делает вложенность
    // корректной: разные вызовы rech_coro_step (в т. ч. вложенные, из тела
    // другой корутины) естественно получают РАЗНЫЕ адреса локальной
    // переменной на СВОЁМ месте C-стека вызовов, поэтому не перетирают
    // друг друга, в отличие от одной глобальной rech_sched_ctx (которая
    // была раньше и не давала вложенности).
    ucontext_t *resume_ctx;
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
// одним и тем же rech_coro_current/очередью без единой блокировки.
// Воспроизведено вживую (50 параллельных "запустить"-воркеров, каждый
// "дождаться" свою асинхронную функцию) ДО этого исправления - три
// прогона дали три разных отказа (segfault; ложное "вложенный дождаться"
// из испорченного rech_coro_current; один прогон случайно не столкнулся) -
// классическая гонка, не гипотетический риск.
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
// берёт себя из rech_coro_current, который rech_coro_step выставляет
// СРАЗУ ПЕРЕД swapcontext, до того как управление реально перейдёт сюда.
// Обычный (не через rech_coro_yield) возврат из fn() переключает контекст
// назад ЯВНЫМ swapcontext на co->resume_ctx - НЕ через "uc_link" (который
// был бы одним и тем же адресом для ЛЮБОГО вызова этой корутины и не
// подходил бы для вложенности, см. package doc rech_coro_t.resume_ctx) -
// co->resume_ctx на этот момент уже указывает ровно туда, откуда её только
// что запустил ТЕКУЩИЙ rech_coro_step (не обязательно тот же самый, что и
// в прошлый раз, если корутина уступала между запусками).
static void rech_coro_trampoline(void) {
    rech_coro_t *co = rech_coro_current;
    co->fn(co->arg);
    co->finished = 1;
    swapcontext(&co->ctx, co->resume_ctx);
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
    co->resume_ctx = NULL;
    co->fn = fn;
    co->arg = arg;
    co->finished = 0;
    co->next = NULL;
    getcontext(&co->ctx);
    co->ctx.uc_stack.ss_sp = co->stack;
    co->ctx.uc_stack.ss_size = RECH_CORO_STACK_SIZE;
    // uc_link - НЕ используется (NULL, "чтобы не забыть" - см. package doc
    // rech_coro_t.resume_ctx): возврат тела - явный swapcontext в
    // rech_coro_trampoline, а не автоматический механизм ucontext.
    co->ctx.uc_link = NULL;
    makecontext(&co->ctx, rech_coro_trampoline, 0);
    rech_coro_enqueue(co);
}

// rech_coro_yield - вызывается ИЗНУТРИ тела корутины (fn, прямо или через
// цепочку обычных C-вызовов на её же стеке) - кооперативно уступает
// планировщику: ставит себя обратно в конец очереди готовых и
// приостанавливается ровно на этом месте, до следующего раза, когда
// планировщик снова выберет её (переключение - на co->resume_ctx, ТЕКУЩИЙ
// на момент этого конкретного запуска, см. package doc rech_coro_t.
// resume_ctx - не обязательно тот же адрес, что при предыдущей уступке).
static void rech_coro_yield(void) {
    rech_coro_t *co = rech_coro_current;
    rech_coro_enqueue(co);
    swapcontext(&co->ctx, co->resume_ctx);
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
    swapcontext(&co->ctx, co->resume_ctx);
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
// корутину до её следующей уступки/завершения.
//
// Вложенность (rech_coro_wait_done, вызванный ИЗНУТРИ уже исполняющейся
// корутины A, сам вызывает эту функцию) - КОРРЕКТНА именно благодаря
// rech_resume_here НИЖЕ: это ЛОКАЛЬНАЯ переменная ДАННОГО КОНКРЕТНОГО
// вызова rech_coro_step - если A вызывает rech_coro_step вложенно (ради
// корутины B), rech_resume_here этого вложенного вызова физически лежит
// на СОБСТВЕННОМ стеке A (эта функция сейчас выполняется КАК ЧАСТЬ тела
// A), не пересекаясь с rech_resume_here внешнего вызова (который лежит
// на стеке ТОГО, кто запустил A - главного потока или ещё одной, более
// внешней корутины). B, уступая/завершаясь, swapcontext'ится РОВНО на
// этот адрес (co->resume_ctx, выставленный чуть ниже) - то есть возврат
// управления идёт туда, откуда B была запущена ИМЕННО в этот раз, а не в
// одно и то же глобальное место для всех - раньше (общая rech_sched_ctx)
// это было ровно тем, что ломало вложенность.
// rech_coro_current сохраняется/восстанавливается (не жёстко в NULL) по
// той же причине: после того, как B закончила один пробег (уступила или
// завершилась) и управление вернулось СЮДА, "текущая корутина" с точки
// зрения ОСТАЛЬНОГО кода на этом потоке снова A (если A её и запустила),
// а не "никто" - иначе A, продолжив работу и, например, сама вызвав
// rech_coro_yield, не смогла бы правильно определить, что уступает именно
// она.
// Завершившаяся (co->finished) корутина освобождается здесь же - стек
// возвращается в пул (rech_coro_stack_free, НЕ free() - см. package doc
// про арены выше).
static int rech_coro_step(void) {
    rech_coro_t *co = rech_coro_dequeue();
    if (co == NULL) {
        if (rech_coro_pending_waiters > 0) {
            rech_coro_poll_once();
            return 1;
        }
        return 0;
    }
    ucontext_t rech_resume_here;
    co->resume_ctx = &rech_resume_here;
    rech_coro_t *rech_prev_current = rech_coro_current;
    rech_coro_current = co;
    swapcontext(&rech_resume_here, &co->ctx);
    rech_coro_current = rech_prev_current;
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
// Безопасно вызывать ВЛОЖЕННО - изнутри уже исполняющейся корутины
// (пример - "асинхронная функция" A, вызванная через "дождаться"/
// "поручить", сама вызывает "дождаться <другая асинхронная функция>()") -
// см. package doc rech_coro_step: каждый вложенный вызов получает
// СОБСТВЕННУЮ точку возврата (локальная переменная на СВОЁМ месте
// C-стека), поэтому не конфликтует с внешним. Раньше (общая на процесс/
// поток rech_sched_ctx) вложенность была не поддержана и рантайм-паниковала -
// снято этим же шагом (Шаг 132, "дожать" по прямой просьбе пользователя).
// rech_coro_step() == 0 внутри цикла (очередь опустела, а конкретная
// задача так и не завершилась) - внутренняя ошибка планировщика, не
// пользовательской программы (при корректной entry невозможно) - явная
// паника вместо тихого зависания/неопределённого результата.
static void rech_coro_wait_done(int *done) {
    while (!*done) {
        if (!rech_coro_step()) {
            rech_panic("внутренняя ошибка: планировщик корутин опустел, не завершив задачу");
        }
    }
}
#endif
`
