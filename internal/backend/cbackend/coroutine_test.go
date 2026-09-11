package cbackend

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// coroutineDriverTimeout - предохранитель на запуск собранного driver'а: у
// корректного планировщика все тесты этого файла отрабатывают за доли
// секунды, а сломанное ожидание fd (например, спутанный waiter или
// забытое rech_coro_pending_waiters--) обычно означает НАСТОЯЩИЙ deadlock
// (epoll_wait/kevent блокируется вечно) - без таймаута такой баг подвесил
// бы весь `go test`, а не просто провалил один тест.
const coroutineDriverTimeout = 5 * time.Second

// findTestCCompiler - тот же поиск, что internal/toolchain.findCCompiler
// (CC, затем cc/clang/gcc в PATH) - не переиспользован напрямую: cbackend
// не может импортировать internal/toolchain (тот импортирует
// internal/compiler, а internal/compiler импортирует cbackend - цикл).
// Небольшое дублирование трёх строк поиска компилятора ради теста, не
// production-логики.
func findTestCCompiler(t *testing.T) string {
	t.Helper()
	if cc := os.Getenv("CC"); cc != "" {
		if path, err := exec.LookPath(cc); err == nil {
			return path
		}
	}
	for _, candidate := range []string{"cc", "clang", "gcc"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	t.Skip("системный C-компилятор не найден (проверялись $CC, cc, clang, gcc)")
	return ""
}

// coroutineDriverPrelude - GC_THREADS (ДО <gc.h> - перехватывает
// pthread_create/GC_pthread_create, нужно тестам с несколькими потоками
// ОС, например TestCoroutineSchedulerIsolatedPerThread, безвредно для
// однопоточных тестов) + <gc.h> (rech_coro_spawn с Шага 130 сам зовёт
// GC_add_roots, package doc coroutine.go) + минимальный rech_panic
// (настоящий - exceptions.go/panicHelper, недоступен изолированному
// driver'у без остального рантайма Rech) - каждый driver обязан сам
// вызвать "GC_INIT();" самой первой строкой main() (тот же порядок, что и
// в настоящем сгенерированном коде, ir_generator.go) ДО первого
// rech_coro_spawn.
const coroutineDriverPrelude = `#define GC_THREADS
#include <gc.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
static void rech_panic(const char *msg) {
    fprintf(stderr, "panic: %s\n", msg);
    exit(1);
}
`

// compileAndRunCoroutineDriver - компилирует coroutineRuntimeCode + driver
// (произвольный C-код, использующий rech_coro_spawn/rech_coro_yield/
// rech_coro_run_all/...) НАСТОЯЩИМ cc и возвращает stdout запущенного
// бинарника - без остального рантайма Rech (ни IR, ни исключения) вообще,
// см. package doc coroutine.go. GC (-lgc) - настоящий, не заглушка: GC-
// корни стека корутины (Шаг 130) - часть того, что эти тесты проверяют.
func compileAndRunCoroutineDriver(t *testing.T, driver string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("coroutineRuntimeCode - POSIX-only (ucontext.h), на Windows не собирается")
	}
	ccPath := findTestCCompiler(t)
	dir := t.TempDir()
	cPath := filepath.Join(dir, "driver.c")
	// "#define _XOPEN_SOURCE" - обязан идти ДО первого системного заголовка
	// вообще (glibc feature-test macro), включая <gc.h> из прелюдии -
	// coroutineRuntimeCode ниже определяет её же повторно (то же
	// значение - легальный повторный #define), но здесь нужно раньше.
	src := "#define _XOPEN_SOURCE 700\n" + coroutineDriverPrelude + coroutineRuntimeCode + "\n" + driver
	if err := os.WriteFile(cPath, []byte(src), 0o644); err != nil {
		t.Fatalf("запись driver.c: %v", err)
	}
	binPath := filepath.Join(dir, "driver")
	// #nosec G204 -- ccPath найден локальным поиском (CC/cc/clang/gcc), cPath - файл, только что написанный этим же тестом.
	cmd := exec.Command(ccPath, cPath, "-o", binPath, "-lgc", "-pthread")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("сборка driver.c: %v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), coroutineDriverTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, binPath).Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("driver не завершился за %s - похоже на deadlock планировщика (fd так и не стал готов, или перепутан waiter)", coroutineDriverTimeout)
	}
	if err != nil {
		t.Fatalf("запуск собранного driver: %v", err)
	}
	return string(out)
}

// TestCoroutineYieldInterleaves - две корутины, каждая печатает три
// маркера с "rech_coro_yield" между ними - если бы переключение контекста
// на самом деле НЕ приостанавливало/не возобновляло стек (например, тело
// просто исполнялось от начала до конца при первом же swapcontext), вывод
// был бы "A1\nA2\nA3\nB1\nB2\nB3\n" (последовательно), а не настоящим
// чередованием - см. package doc coroutine.go, arithmetic ожидаемого
// порядка расписан там же.
func TestCoroutineYieldInterleaves(t *testing.T) {
	driver := `
static void coro_a(void *arg) {
    (void)arg;
    printf("A1\n");
    rech_coro_yield();
    printf("A2\n");
    rech_coro_yield();
    printf("A3\n");
}
static void coro_b(void *arg) {
    (void)arg;
    printf("B1\n");
    rech_coro_yield();
    printf("B2\n");
    rech_coro_yield();
    printf("B3\n");
}
int main(void) {
    GC_INIT();
    rech_coro_spawn(coro_a, NULL);
    rech_coro_spawn(coro_b, NULL);
    rech_coro_run_all();
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	want := "A1\nB1\nA2\nB2\nA3\nB3\n"
	if out != want {
		t.Fatalf("ожидался чередующийся вывод %q, получено %q", want, out)
	}
}

// TestCoroutineWithoutYieldRunsToCompletion - корутина без единого
// rech_coro_yield должна просто отработать целиком за один
// swapcontext (обычный возврат из fn() -> автоматическое переключение
// назад через uc_link, без явного участия) - минимальный случай, отдельный
// от чередования выше.
func TestCoroutineWithoutYieldRunsToCompletion(t *testing.T) {
	driver := `
static void coro(void *arg) {
    (void)arg;
    printf("привет из корутины\n");
}
int main(void) {
    GC_INIT();
    rech_coro_spawn(coro, NULL);
    rech_coro_run_all();
    printf("после run_all\n");
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	want := "привет из корутины\nпосле run_all\n"
	if out != want {
		t.Fatalf("ожидалось %q, получено %q", want, out)
	}
}

// TestCoroutineArgIsPerInstanceNotShared - несколько корутин с одной и той
// же функцией-телом, каждая со СВОИМ arg - проверяет, что rech_coro_spawn
// не путает/не разделяет arg между экземплярами (частая ошибка при ручном
// приведении void* - если бы arg хранился в общей статической переменной
// вместо поля rech_coro_t, все три напечатали бы одно и то же число).
func TestCoroutineArgIsPerInstanceNotShared(t *testing.T) {
	driver := `
static void coro(void *arg) {
    long n = (long)arg;
    printf("%ld\n", n);
    rech_coro_yield();
    printf("%ld снова\n", n);
}
int main(void) {
    GC_INIT();
    rech_coro_spawn(coro, (void *)(long)1);
    rech_coro_spawn(coro, (void *)(long)2);
    rech_coro_spawn(coro, (void *)(long)3);
    rech_coro_run_all();
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	want := "1\n2\n3\n1 снова\n2 снова\n3 снова\n"
	if out != want {
		t.Fatalf("ожидалось %q, получено %q", want, out)
	}
}

// TestCoroutineManyDoNotCrash - 1000 корутин, часть без единого yield,
// часть с несколькими - грубая проверка, что malloc/free стеков (256 КБ
// каждый, RECH_CORO_STACK_SIZE) на реалистичном количестве не падает и не
// течёт настолько, чтобы завершиться ошибкой в пределах теста (реальную
// экономию памяти по сравнению с потоками ОС из Шага 127 этот шаг ещё не
// измеряет - у него нет I/O, значит и сценария "тысячи одновременно
// простаивающих" тоже нет, см. package doc coroutine.go).
func TestCoroutineManyDoNotCrash(t *testing.T) {
	driver := `
static void coro(void *arg) {
    long n = (long)arg;
    if (n % 2 == 0) {
        rech_coro_yield();
    }
    printf("%ld\n", n);
}
int main(void) {
    GC_INIT();
    long i;
    for (i = 0; i < 1000; i++) {
        rech_coro_spawn(coro, (void *)i);
    }
    rech_coro_run_all();
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	lines := 0
	for _, c := range out {
		if c == '\n' {
			lines++
		}
	}
	if lines != 1000 {
		t.Fatalf("ожидалось 1000 строк вывода (по одной на корутину), получено %d, вывод: %q", lines, out)
	}
}

// TestCoroutineWaitReadableSuspendsUntilDataArrives - решающий тест
// инкремента 3 (epoll/kqueue): читающая корутина стартует ПЕРВОЙ (её
// pipe(2) в этот момент ещё пуст) и вызывает rech_coro_wait_readable - если
// бы ожидание НЕ приостанавливало стек по-настоящему (например, тихо
// возвращалось сразу или блокировало ВЕСЬ процесс через обычный read()),
// вывод получился бы либо неверного порядка ("reader done" раньше "writer
// running"), либо тест зависнет вовсе (реальный deadlock однопоточного
// процесса на пустом pipe) - см. coroutineDriverTimeout. Пишущая корутина
// стартует ВТОРОЙ, из очереди готовых (не через epoll вовсе) - её запуск и
// есть событие, которое делает fd читающей корутины готовым уже ПОСЛЕ
// того, как та приостановилась.
func TestCoroutineWaitReadableSuspendsUntilDataArrives(t *testing.T) {
	driver := `
#include <unistd.h>
static int rech_test_pipe[2];
static void coro_reader(void *arg) {
    (void)arg;
    printf("reader waiting\n");
    rech_coro_wait_readable(rech_test_pipe[0]);
    char buf[16];
    ssize_t n = read(rech_test_pipe[0], buf, sizeof(buf) - 1);
    buf[n] = 0;
    printf("reader got: %s\n", buf);
}
static void coro_writer(void *arg) {
    (void)arg;
    printf("writer running\n");
    write(rech_test_pipe[1], "hi", 2);
}
int main(void) {
    GC_INIT();
    if (pipe(rech_test_pipe) != 0) {
        return 1;
    }
    rech_coro_spawn(coro_reader, NULL);
    rech_coro_spawn(coro_writer, NULL);
    rech_coro_run_all();
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	want := "reader waiting\nwriter running\nreader got: hi\n"
	if out != want {
		t.Fatalf("ожидался порядок %q (ожидание -> уступка -> запись -> возобновление), получено %q", want, out)
	}
}

// TestCoroutineWaitWritableFiresOnFreshPipe - минимальная проверка ветки
// "запись" (EPOLLOUT/EVFILT_WRITE, не только чтение выше) - у свежего
// pipe(2) буфер записи пуст, значит он читается написанным сразу же, без
// реального ожидания: проверяет, что сам код регистрации/срабатывания для
// записываемости не падает и не виснет, не порядок приостановки/
// возобновления (та проверена для чтения выше).
func TestCoroutineWaitWritableFiresOnFreshPipe(t *testing.T) {
	driver := `
#include <unistd.h>
static int rech_test_pipe[2];
static void coro_writer(void *arg) {
    (void)arg;
    rech_coro_wait_writable(rech_test_pipe[1]);
    write(rech_test_pipe[1], "ok", 2);
    printf("wrote\n");
}
int main(void) {
    GC_INIT();
    if (pipe(rech_test_pipe) != 0) {
        return 1;
    }
    rech_coro_spawn(coro_writer, NULL);
    rech_coro_run_all();
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	want := "wrote\n"
	if out != want {
		t.Fatalf("ожидалось %q, получено %q", want, out)
	}
}

// TestCoroutineWaitDemuxesToCorrectCoroutine - две корутины ждут ДВА
// РАЗНЫХ pipe(2) одновременно - проверяет, что epoll/kqueue-событие
// возвращает планировщику ПРАВИЛЬНОГО waiter'а (rech_coro_waiter_t.co) для
// каждого fd, а не перепутанного/общего - баг здесь (например, общая
// статическая переменная вместо поля в waiter) проявился бы как обмен
// данными местами (R1 получил бы "two", R2 - "one"). Оба pipe уже
// заполнены ДО rech_coro_run_all - порядок между R1/R2 в выводе поэтому не
// гарантирован (один epoll_wait/kevent может вернуть оба события сразу, в
// любом порядке) - сравнивается набор строк, не точная строка.
func TestCoroutineWaitDemuxesToCorrectCoroutine(t *testing.T) {
	driver := `
#include <unistd.h>
typedef struct { int fd; const char *label; } reader_arg_t;
static void coro_reader(void *arg) {
    reader_arg_t *ra = (reader_arg_t *)arg;
    rech_coro_wait_readable(ra->fd);
    char buf[16];
    ssize_t n = read(ra->fd, buf, sizeof(buf) - 1);
    buf[n] = 0;
    printf("%s:%s\n", ra->label, buf);
}
int main(void) {
    GC_INIT();
    int fds1[2], fds2[2];
    if (pipe(fds1) != 0 || pipe(fds2) != 0) {
        return 1;
    }
    write(fds1[1], "one", 3);
    write(fds2[1], "two", 3);
    static reader_arg_t r1;
    static reader_arg_t r2;
    r1.fd = fds1[0];
    r1.label = "R1";
    r2.fd = fds2[0];
    r2.label = "R2";
    rech_coro_spawn(coro_reader, &r1);
    rech_coro_spawn(coro_reader, &r2);
    rech_coro_run_all();
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	got := map[string]bool{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		got[line] = true
	}
	want := []string{"R1:one", "R2:two"}
	for _, w := range want {
		if !got[w] {
			t.Fatalf("ожидалась строка %q в выводе, получено %q", w, out)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("ожидалось ровно %d строк (%v), получено %d: %q", len(want), want, len(got), out)
	}
}

// TestCoroutineStackIsGCRoot - решающий тест инкремента 4 (Шаг 130) для
// GC-корней (package doc coroutine.go): значение, GC_MALLOC'нутое внутри
// тела корутины и живущее ТОЛЬКО в локальной C-переменной на её
// собственном стеке (нигде больше не достижимо - ни из глобалов, ни из
// стека планировщика/main), не должно быть собрано настоящим
// GC_gcollect(), вызванным РОВНО в момент, когда корутина приостановлена
// (rech_coro_yield). Управляет планировщиком вручную по шагам
// (rech_coro_step, не rech_coro_run_all) именно ради того, чтобы вставить
// GC_gcollect() между "корутина уступила" и "корутина возобновлена" - без
// GC_add_roots в rech_coro_spawn этот тест почти наверняка увидел бы
// испорченную/перезаписанную память вместо "живо" (сборщик рассматривает
// только зарегистрированные корни - malloc'нутый стек корутины ему
// неизвестен, пока не зарегистрирован явно).
func TestCoroutineStackIsGCRoot(t *testing.T) {
	driver := `
#include <string.h>
static void coro_alloc_holder(void *arg) {
    (void)arg;
    char *p = (char *)GC_MALLOC(64);
    strcpy(p, "живо");
    rech_coro_yield();
    printf("%s\n", p);
}
int main(void) {
    GC_INIT();
    rech_coro_spawn(coro_alloc_holder, NULL);
    rech_coro_step();
    GC_gcollect();
    // Давление на аллокатор ПОСЛЕ первого GC_gcollect() - если бы p не был
    // на самом деле зарегистрированным корнем, GC_gcollect() выше уже
    // пометил бы его память как мусор, а один из этих 200000 GC_MALLOC(64)
    // (тот же класс размера) почти наверняка переиспользовал бы РОВНО этот
    // адрес и обнулил бы его (GC_MALLOC возвращает обнулённую память) -
    // без этой петли тест не был бы решающим: сама по себе непереписанная
    // память могла случайно уцелеть и до, и после сборки, не доказывая
    // ничего (проверено вручную - без GC_add_roots в rech_coro_spawn этот
    // тест с петлёй давления реально проваливается, без нёе - нет).
    for (long i = 0; i < 200000; i++) {
        GC_MALLOC(64);
    }
    GC_gcollect();
    rech_coro_step();
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	want := "живо\n"
	if out != want {
		t.Fatalf("указатель, живущий только на стеке приостановленной корутины, не пережил GC_gcollect() - GC-корень (GC_add_roots в rech_coro_spawn) не сработал: ожидалось %q, получено %q", want, out)
	}
}

// TestCoroutineSchedulerIsolatedPerThread - РЕШАЮЩИЙ тест инкремента 5
// (Шаг 131, M:N): 8 настоящих потоков ОС (pthread_create), каждый крутит
// СВОЙ планировщик 500 раз подряд (spawn + run_all на каждой итерации),
// без единой блокировки между ними - если бы состояние планировщика
// (rech_sched_ctx/rech_coro_queue_*/rech_coro_current, coroutine.go) НЕ
// было RECH_THREAD_LOCAL, потоки гонялись бы за одними и теми же
// глобальными переменными: ОДИН И ТОТ ЖЕ swapcontext на общем
// rech_sched_ctx из двух потоков ОДНОВРЕМЕННО - классический рецепт
// сегфолта/испорченной памяти. Живьём воспроизведено (,
// Шаг 131) - через полный конвейер Rech (50-300 параллельных
// "запустить"-воркеров, каждый "дождаться <асинхронная функция>") ДО
// исправления: сегфолт, ложная паника "вложенный дождаться" (испорченное
// чтение rech_coro_current), один прогон случайно уцелел - три разных
// исхода на три прогона, классическая гонка. Каждый поток пишет результаты
// в СВОЙ собственный буфер (malloc, передан как результат pthread_join) -
// сверяется главным потоком ПОСЛЕ join, а не текстом stdout (не нужно
// синхронизировать printf между потоками, проверка детерминирована).
func TestCoroutineSchedulerIsolatedPerThread(t *testing.T) {
	driver := `
#define RECH_TEST_NTHREADS 8
#define RECH_TEST_ITERS 500

typedef struct {
    long thread_id;
    long results[RECH_TEST_ITERS];
} rech_test_thread_result_t;

typedef struct {
    long value;
    long *out;
} rech_test_coro_arg_t;

static void coro_double(void *arg) {
    rech_test_coro_arg_t *a = (rech_test_coro_arg_t *)arg;
    rech_coro_yield();
    *a->out = a->value * 2;
}

static void *thread_main(void *arg) {
    rech_test_thread_result_t *tr = (rech_test_thread_result_t *)arg;
    for (int i = 0; i < RECH_TEST_ITERS; i++) {
        long value = tr->thread_id * 1000000 + i;
        rech_test_coro_arg_t ca;
        ca.value = value;
        ca.out = &tr->results[i];
        rech_coro_spawn(coro_double, &ca);
        rech_coro_run_all();
    }
    return NULL;
}

int main(void) {
    GC_INIT();
    pthread_t threads[RECH_TEST_NTHREADS];
    rech_test_thread_result_t *results[RECH_TEST_NTHREADS];
    int t;
    for (t = 0; t < RECH_TEST_NTHREADS; t++) {
        results[t] = (rech_test_thread_result_t *)malloc(sizeof(rech_test_thread_result_t));
        results[t]->thread_id = t;
        pthread_create(&threads[t], NULL, thread_main, results[t]);
    }
    for (t = 0; t < RECH_TEST_NTHREADS; t++) {
        pthread_join(threads[t], NULL);
    }
    int ok = 1;
    for (t = 0; t < RECH_TEST_NTHREADS; t++) {
        int i;
        for (i = 0; i < RECH_TEST_ITERS; i++) {
            long value = results[t]->thread_id * 1000000 + i;
            if (results[t]->results[i] != value * 2) {
                ok = 0;
            }
        }
    }
    printf(ok ? "OK\n" : "FAIL\n");
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	if out != "OK\n" {
		t.Fatalf("несколько потоков ОС с независимыми планировщиками дали испорченный результат (гонка на разделяемом состоянии планировщика) - получено %q", out)
	}
}

// TestCoroutineNestedWaitDone - РЕШАЮЩИЙ тест "дожатого" по прямой
// просьбе пользователя снятия ограничения (Шаг 132, coroutine.go, package
// doc про rech_coro_t.resume_ctx/rech_coro_step): корутина "outer",
// исполняющаяся НА планировщике (запущена и ожидаема "root"'ом через
// rech_coro_wait_done), сама спавнит "inner" и САМА вызывает
// rech_coro_wait_done ВЛОЖЕННО, изнутри своего же тела - раньше это было
// невозможно (общая на процесс/поток rech_sched_ctx), теперь безопасно
// благодаря resume_ctx, лежащему на СВОЁМ месте C-стека для каждого
// вложенного вызова rech_coro_step. "inner" ещё и уступает
// (rech_coro_yield) ПЕРЕД тем, как завершиться - без корректного
// сохранения/восстановления rech_coro_current вокруг вложенного
// swapcontext это тоже было бы почвой для порчи состояния. Порядок вывода
// доказывает, что вложенность работает, а не просто "не падает":
// "outer running" -> "inner running" -> "outer got inner value: 42" ->
// "root got outer value: 43".
func TestCoroutineNestedWaitDone(t *testing.T) {
	driver := `
typedef struct { int done; int value; } coro_result_t;

static void coro_inner(void *arg) {
    coro_result_t *r = (coro_result_t *)arg;
    printf("inner running\n");
    rech_coro_yield();
    r->value = 42;
    r->done = 1;
}

static void coro_outer(void *arg) {
    coro_result_t *r = (coro_result_t *)arg;
    printf("outer running\n");
    coro_result_t inner_r;
    inner_r.done = 0;
    inner_r.value = 0;
    rech_coro_spawn(coro_inner, &inner_r);
    rech_coro_wait_done(&inner_r.done);
    printf("outer got inner value: %d\n", inner_r.value);
    r->value = inner_r.value + 1;
    r->done = 1;
}

int main(void) {
    GC_INIT();
    coro_result_t outer_r;
    outer_r.done = 0;
    outer_r.value = 0;
    rech_coro_spawn(coro_outer, &outer_r);
    rech_coro_wait_done(&outer_r.done);
    printf("root got outer value: %d\n", outer_r.value);
    return 0;
}
`
	out := compileAndRunCoroutineDriver(t, driver)
	want := "outer running\ninner running\nouter got inner value: 42\nroot got outer value: 43\n"
	if out != want {
		t.Fatalf("вложенный rech_coro_wait_done дал неверный результат/порядок - ожидалось %q, получено %q", want, out)
	}
}
