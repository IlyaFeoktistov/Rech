package concurrency

// CoroutineRuntimeWindowsCode - тот же интерфейс лёгких задач, что и
// CoroutineRuntimeCode (rech_coro_spawn/yield/step/run_all/wait_done,
// rech_coro_wait_readable/writable), для Windows: вместо ucontext - волокна
// (CreateFiberEx/SwitchToFiber), вместо epoll/kqueue - WSAPoll по списку
// ожидающих сокетов.
//
// Boehm GC видит у потока только один стек - от текущего sp до
// "stackbottom" потока. Поэтому каждый контекст исполнения (волокно
// корутины и исходный контекст потока) - запись rech_coro_frame_t в общем
// реестре: у приостановленного записаны sp и вершина стека, их диапазоны
// отдаёт сборщику колбэк GC_set_push_other_roots, а для исполняемого
// контекста GC_set_stackbottom указывает вершину его собственного стека.
// Регистры приостанавливаемого контекста сохраняются на его же стеке
// (setjmp в rech_coro_switch_to) - SwitchToFiber хранит их вне стека, где
// сборщик их не видит.
//
// Переключение - под блокировкой GC (GC_alloc_lock): переключающий берёт
// её, меняет stackbottom и реестр, переключается; получатель отпускает её
// сразу после возврата из SwitchToFiber (или в начале трамплина новой
// корутины). Так сборщик никогда не застаёт поток посередине
// переключения. Стек волокна закоммичен целиком (commit = reserve), без
// guard-страниц: сборщик читает его без исключений.
const CoroutineRuntimeWindowsCode = `#ifdef _WIN32
#include <winsock2.h>
#include <setjmp.h>
#include <gc_mark.h>

#define RECH_CORO_STACK_SIZE (256 * 1024)

#ifndef RECH_THREAD_LOCAL
#define RECH_THREAD_LOCAL __declspec(thread)
#endif

typedef struct rech_coro_frame {
    char *sp;
    char *stack_hi;
    int suspended;
    struct rech_coro_frame *reg_next;
    struct rech_coro_frame *reg_prev;
} rech_coro_frame_t;

typedef struct rech_coro {
    rech_coro_frame_t frame;
    LPVOID fiber;
    LPVOID resume_fiber;
    rech_coro_frame_t *resume_frame;
    void (*fn)(void *);
    void *arg;
    int finished;
    struct rech_coro *next;
} rech_coro_t;

static rech_coro_frame_t *volatile rech_coro_frames = NULL;
static SRWLOCK rech_coro_frames_lock = SRWLOCK_INIT;
static int rech_coro_roots_hooked = 0;
static GC_push_other_roots_proc rech_coro_prev_push_roots = 0;

static void GC_CALLBACK rech_coro_push_roots(void) {
    for (rech_coro_frame_t *f = rech_coro_frames; f != NULL; f = f->reg_next) {
        if (f->suspended && f->sp != NULL && f->stack_hi != NULL) {
            GC_push_all_eager(f->sp, f->stack_hi);
        }
    }
    if (rech_coro_prev_push_roots != 0) {
        rech_coro_prev_push_roots();
    }
}

static void rech_coro_register(rech_coro_frame_t *f) {
    AcquireSRWLockExclusive(&rech_coro_frames_lock);
    if (!rech_coro_roots_hooked) {
        rech_coro_prev_push_roots = GC_get_push_other_roots();
        GC_set_push_other_roots(rech_coro_push_roots);
        rech_coro_roots_hooked = 1;
    }
    f->reg_prev = NULL;
    f->reg_next = rech_coro_frames;
    if (rech_coro_frames != NULL) {
        rech_coro_frames->reg_prev = f;
    }
    rech_coro_frames = f;
    ReleaseSRWLockExclusive(&rech_coro_frames_lock);
}

static void rech_coro_unregister(rech_coro_frame_t *f) {
    AcquireSRWLockExclusive(&rech_coro_frames_lock);
    if (f->reg_prev != NULL) {
        f->reg_prev->reg_next = f->reg_next;
    } else {
        rech_coro_frames = f->reg_next;
    }
    if (f->reg_next != NULL) {
        f->reg_next->reg_prev = f->reg_prev;
    }
    ReleaseSRWLockExclusive(&rech_coro_frames_lock);
}

static RECH_THREAD_LOCAL rech_coro_t *rech_coro_queue_head = NULL;
static RECH_THREAD_LOCAL rech_coro_t *rech_coro_queue_tail = NULL;
static RECH_THREAD_LOCAL rech_coro_t *rech_coro_current = NULL;
static RECH_THREAD_LOCAL rech_coro_frame_t *rech_coro_thread_frame = NULL;
static RECH_THREAD_LOCAL rech_coro_frame_t *rech_coro_active_frame = NULL;
static RECH_THREAD_LOCAL void *rech_coro_gc_thread = NULL;

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

// Исходный контекст потока - волокно (ConvertThreadToFiber) и запись
// реестра с настоящим дном стека потока, которое знает сборщик.
static void rech_coro_thread_init(void) {
    if (rech_coro_thread_frame != NULL) {
        return;
    }
    if (!IsThreadAFiber()) {
        ConvertThreadToFiber(NULL);
    }
    struct GC_stack_base sb;
    rech_coro_gc_thread = GC_get_my_stackbottom(&sb);
    rech_coro_frame_t *f = (rech_coro_frame_t *)GC_MALLOC_UNCOLLECTABLE(sizeof(rech_coro_frame_t));
    f->sp = NULL;
    f->stack_hi = (char *)sb.mem_base;
    f->suspended = 0;
    rech_coro_register(f);
    rech_coro_thread_frame = f;
    rech_coro_active_frame = f;
}

// rech_coro_switch_to - приостанавливает текущий контекст и переключается
// на to/to_fiber (см. package doc про блокировку GC). noinline: кадр этой
// функции с сохранёнными регистрами (regs) остаётся на стеке
// приостановленного контекста, sp записи указывает ниже него.
static __attribute__((noinline)) void rech_coro_switch_to(rech_coro_frame_t *to, LPVOID to_fiber) {
    jmp_buf regs;
    setjmp(regs);
    rech_coro_frame_t *from = rech_coro_active_frame;
    GC_alloc_lock();
    from->sp = (char *)&regs;
    from->suspended = 1;
    to->suspended = 0;
    rech_coro_active_frame = to;
    if (to->stack_hi != NULL) {
        struct GC_stack_base sb;
        sb.mem_base = to->stack_hi;
        GC_set_stackbottom(rech_coro_gc_thread, &sb);
    }
    SwitchToFiber(to_fiber);
    GC_alloc_unlock();
}

// Первое исполнение корутины: блокировку GC держит переключивший сюда
// контекст - вершина стека волокна (NT_TIB.StackBase) становится
// stackbottom потока до её освобождения.
static void WINAPI rech_coro_trampoline(LPVOID param) {
    rech_coro_t *co = (rech_coro_t *)param;
    co->frame.stack_hi = (char *)((NT_TIB *)NtCurrentTeb())->StackBase;
    struct GC_stack_base sb;
    sb.mem_base = co->frame.stack_hi;
    GC_set_stackbottom(rech_coro_gc_thread, &sb);
    GC_alloc_unlock();
    co->fn(co->arg);
    co->finished = 1;
    rech_coro_switch_to(co->resume_frame, co->resume_fiber);
}

// Структура корутины - в памяти, которую сборщик сканирует, но не
// освобождает сам: arg (задача, GC_MALLOC) живёт, даже если результат
// "поручить" никто не сохранил.
static void rech_coro_spawn(void (*fn)(void *), void *arg) {
    rech_coro_thread_init();
    rech_coro_t *co = (rech_coro_t *)GC_MALLOC_UNCOLLECTABLE(sizeof(rech_coro_t));
    co->fn = fn;
    co->arg = arg;
    co->finished = 0;
    co->next = NULL;
    co->resume_fiber = NULL;
    co->resume_frame = NULL;
    co->frame.sp = NULL;
    co->frame.stack_hi = NULL;
    co->frame.suspended = 1;
    co->fiber = CreateFiberEx(RECH_CORO_STACK_SIZE, RECH_CORO_STACK_SIZE, FIBER_FLAG_FLOAT_SWITCH, rech_coro_trampoline, co);
    if (co->fiber == NULL) {
        rech_panic("не удалось создать корутину (CreateFiberEx)");
    }
    rech_coro_register(&co->frame);
    rech_coro_enqueue(co);
}

static void rech_coro_yield(void) {
    rech_coro_t *co = rech_coro_current;
    rech_coro_enqueue(co);
    rech_coro_switch_to(co->resume_frame, co->resume_fiber);
}

typedef struct rech_coro_waiter {
    SOCKET fd;
    short events;
    rech_coro_t *co;
} rech_coro_waiter_t;

static RECH_THREAD_LOCAL rech_coro_waiter_t *rech_coro_waiters = NULL;
static RECH_THREAD_LOCAL int rech_coro_pending_waiters = 0;
static RECH_THREAD_LOCAL int rech_coro_waiters_cap = 0;

static void rech_coro_wait_fd(SOCKET fd, int writable) {
    rech_coro_t *co = rech_coro_current;
    if (rech_coro_pending_waiters == rech_coro_waiters_cap) {
        rech_coro_waiters_cap = rech_coro_waiters_cap == 0 ? 16 : rech_coro_waiters_cap * 2;
        rech_coro_waiters = (rech_coro_waiter_t *)realloc(rech_coro_waiters, sizeof(rech_coro_waiter_t) * (size_t)rech_coro_waiters_cap);
    }
    rech_coro_waiter_t *w = &rech_coro_waiters[rech_coro_pending_waiters++];
    w->fd = fd;
    w->events = writable ? POLLWRNORM : POLLRDNORM;
    w->co = co;
    rech_coro_switch_to(co->resume_frame, co->resume_fiber);
}

static void rech_coro_wait_readable(SOCKET fd) {
    rech_coro_wait_fd(fd, 0);
}

static void rech_coro_wait_writable(SOCKET fd) {
    rech_coro_wait_fd(fd, 1);
}

// Одно ожидание готовности всех сокетов сразу; готовые корутины - в
// очередь, их записи удаляются (последняя встаёт на место удалённой).
static void rech_coro_poll_once(void) {
    int n = rech_coro_pending_waiters;
    WSAPOLLFD *fds = (WSAPOLLFD *)malloc(sizeof(WSAPOLLFD) * (size_t)n);
    for (int i = 0; i < n; i++) {
        fds[i].fd = rech_coro_waiters[i].fd;
        fds[i].events = rech_coro_waiters[i].events;
        fds[i].revents = 0;
    }
    if (WSAPoll(fds, (ULONG)n, -1) == SOCKET_ERROR) {
        free(fds);
        rech_panic("ошибка ожидания сокетов планировщиком корутин (WSAPoll)");
    }
    for (int i = n - 1; i >= 0; i--) {
        if (fds[i].revents != 0) {
            rech_coro_enqueue(rech_coro_waiters[i].co);
            rech_coro_waiters[i] = rech_coro_waiters[--rech_coro_pending_waiters];
        }
    }
    free(fds);
}

static int rech_coro_step(void) {
    rech_coro_t *co = rech_coro_dequeue();
    if (co == NULL) {
        if (rech_coro_pending_waiters > 0) {
            rech_coro_poll_once();
            return 1;
        }
        return 0;
    }
    rech_coro_thread_init();
    co->resume_fiber = GetCurrentFiber();
    co->resume_frame = rech_coro_active_frame;
    rech_coro_t *rech_prev_current = rech_coro_current;
    rech_coro_current = co;
    rech_coro_switch_to(&co->frame, co->fiber);
    rech_coro_current = rech_prev_current;
    if (co->finished) {
        rech_coro_unregister(&co->frame);
        DeleteFiber(co->fiber);
        GC_FREE(co);
    }
    return 1;
}

static void rech_coro_run_all(void) {
    while (rech_coro_step()) {
    }
}

static void rech_coro_wait_done(int *done) {
    while (!*done) {
        if (!rech_coro_step()) {
            rech_panic("` + schedulerEmptyMsg + `");
        }
    }
}
#endif
`
