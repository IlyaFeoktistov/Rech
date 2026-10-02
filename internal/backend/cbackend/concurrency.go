// Параллелизм ("запустить"/"поручить"/"дождаться") - задача запускает
// значение типа Вызываемый (&функция, лямбда, обёртка метода) с
// аргументами. Обёртка - ОДНА НА СИГНАТУРУ, не на функцию: задачи одной
// сигнатуры - один тип (ir.TaskType), кто бы их ни запустил - поток ОС или
// корутина.
//
// Структура задачи rech_TASK_<сигнатура> несёт результат, исключение,
// сам Вызываемый с аргументами и указатель wait - функцию ожидания того
// вида исполнения, которым задача запущена. "дождаться" (rech_await_...)
// зовёт t->wait и не знает, поток это или корутина, - поэтому задачи обоих
// видов живут в одном Список<Задача<...>>.
//
// Поток ОС (ensureThreadLaunch): задача ставится в очередь общего пула
// воркеров (rech_pool_submit, threadpool.go), синхронизация завершения -
// rech_task_sync (mutex+cond / event) по указателю t->sync, чтобы сама
// структура задачи не зависела от pthread и в программе только с
// корутинами. Корутина (ensureCoroLaunch): rech_coro_spawn на лёгком
// планировщике (coroutine.go), завершение - t->coro_done.
//
// Boehm GC: воркеры пула регистрируются в GC автоматически (GC_THREADS
// подменяет pthread_create/CreateThread), вручную точка входа ничего не
// регистрирует - двойная регистрация роняла процесс при выходе потока.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func mangleTaskType(sig string) string   { return "rech_TASK_" + transliterate(sig) }
func mangleTaskAwait(sig string) string  { return "rech_await_" + transliterate(sig) }
func mangleTaskLaunch(sig string) string { return "rech_launch_" + transliterate(sig) }
func mangleCoroLaunch(sig string) string { return "rech_coro_launch_" + transliterate(sig) }

// taskSig - разобранная сигнатура задачи (ctx.callableSigs) и C-типы её
// частей.
type taskSig struct {
	name     string
	sig      ir.CallableSig
	taskType string
	resultC  string // "" - без результата
	argTypes []string
}

func newTaskSig(ctx *irGenCtx, name string) (taskSig, error) {
	sig, ok := ctx.callableSigs[name]
	if !ok {
		return taskSig{}, fmt.Errorf("внутренняя ошибка: сигнатура задачи %q не зарегистрирована", name)
	}
	ts := taskSig{name: name, sig: sig, taskType: mangleTaskType(name)}
	if sig.Result != nil {
		ts.resultC = fromIRType(*sig.Result).cType(ctx)
	}
	for _, p := range sig.Params {
		ts.argTypes = append(ts.argTypes, fromIRType(p).cType(ctx))
	}
	return ts, nil
}

// paramList - "rech_C_... fn, T0 a0, T1 a1" - параметры rech_launch_...
func (ts taskSig) paramList() string {
	parts := []string{mangleCallableType(ts.name) + " fn"}
	for i, t := range ts.argTypes {
		parts = append(parts, fmt.Sprintf("%s a%d", t, i))
	}
	return strings.Join(parts, ", ")
}

// fillTask - выделение структуры задачи и копирование в неё Вызываемого и
// аргументов (общее для потока и корутины).
func (ts taskSig) fillTask(sb *strings.Builder) {
	fmt.Fprintf(sb, "    %s *t = (%s *)GC_MALLOC(sizeof(%s));\n", ts.taskType, ts.taskType, ts.taskType)
	sb.WriteString("    t->fn = fn;\n")
	for i := range ts.argTypes {
		fmt.Fprintf(sb, "    t->a%d = a%d;\n", i, i)
	}
	sb.WriteString("    t->exc = NULL;\n")
}

// runUnderCatch - тело точки входа: вызов t->fn под кадром "попытаться"
// (исключение или паника оседает в t->exc и поднимается заново в
// "дождаться"). "= {0}" - воркер пула переиспользует стек, мусор прежнего
// кадра не должен читаться как значение.
func (ts taskSig) runUnderCatch(sb *strings.Builder) {
	fmt.Fprintf(sb, "    %s rech_frame = {0};\n", catchFrameType)
	fmt.Fprintf(sb, "    rech_frame.prev = %s;\n", catchTopVar)
	fmt.Fprintf(sb, "    %s = &rech_frame;\n", catchTopVar)
	sb.WriteString("    if (setjmp(rech_frame.buf) == 0) {\n")
	args := []string{"t->fn.env"}
	for i := range ts.argTypes {
		args = append(args, fmt.Sprintf("t->a%d", i))
	}
	call := fmt.Sprintf("t->fn.fn(%s)", strings.Join(args, ", "))
	if ts.resultC != "" {
		fmt.Fprintf(sb, "        t->result = %s;\n", call)
	} else {
		fmt.Fprintf(sb, "        %s;\n", call)
	}
	fmt.Fprintf(sb, "        %s = rech_frame.prev;\n", catchTopVar)
	sb.WriteString("    } else {\n")
	fmt.Fprintf(sb, "        %s = rech_frame.prev;\n", catchTopVar)
	sb.WriteString("        t->exc = rech_frame.exc;\n")
	sb.WriteString("    }\n")
}

// ensureTaskType эмитит структуру задачи и rech_await_... для сигнатуры
// (один раз на сигнатуру, ctx.taskSigsEmitted).
func ensureTaskType(ctx *irGenCtx, name string) (taskSig, error) {
	ts, err := newTaskSig(ctx, name)
	if err != nil {
		return ts, err
	}
	if ctx.taskSigsEmitted[name] {
		return ts, nil
	}
	ctx.taskSigsEmitted[name] = true
	ctx.usesGC = true
	ctx.usesExceptions = true

	var sb strings.Builder
	fmt.Fprintf(&sb, "struct %s {\n", ts.taskType)
	if ts.resultC != "" {
		fmt.Fprintf(&sb, "    %s result;\n", ts.resultC)
	}
	sb.WriteString("    void *exc;\n")
	fmt.Fprintf(&sb, "    void (*wait)(struct %s *t);\n", ts.taskType)
	fmt.Fprintf(&sb, "    %s fn;\n", mangleCallableType(name))
	for i, t := range ts.argTypes {
		fmt.Fprintf(&sb, "    %s a%d;\n", t, i)
	}
	sb.WriteString("    void *sync;\n")
	sb.WriteString("    int coro_done;\n")
	sb.WriteString("};\n")

	resultC := ts.resultC
	if resultC == "" {
		resultC = "void"
	}
	fmt.Fprintf(&sb, "static %s %s(%s *t) {\n", resultC, mangleTaskAwait(name), ts.taskType)
	sb.WriteString("    t->wait(t);\n")
	sb.WriteString("    if (t->exc != NULL) {\n")
	fmt.Fprintf(&sb, "        %s(t->exc, %s);\n", throwFunc, thrownTypeNameExpr("t->exc"))
	sb.WriteString("    }\n")
	if ts.resultC != "" {
		sb.WriteString("    return t->result;\n")
	}
	sb.WriteString("}\n\n")
	ctx.taskCode.WriteString(sb.String())
	return ts, nil
}

// taskSyncCode - синхронизация завершения задачи на потоке ОС, общая для
// всех сигнатур (эмитится один раз, при первом потоковом запуске).
const taskSyncCode = `typedef struct {
#ifdef _WIN32
    HANDLE done_event;
#else
    pthread_mutex_t mutex;
    pthread_cond_t cond;
    int done;
#endif
} rech_task_sync;

static rech_task_sync *rech_task_sync_new(void) {
    rech_task_sync *s = (rech_task_sync *)GC_MALLOC(sizeof(rech_task_sync));
#ifdef _WIN32
    s->done_event = CreateEvent(NULL, TRUE, FALSE, NULL);
#else
    pthread_mutex_init(&s->mutex, NULL);
    pthread_cond_init(&s->cond, NULL);
    s->done = 0;
#endif
    return s;
}

static void rech_task_sync_signal(rech_task_sync *s) {
#ifdef _WIN32
    SetEvent(s->done_event);
#else
    pthread_mutex_lock(&s->mutex);
    s->done = 1;
    pthread_cond_signal(&s->cond);
    pthread_mutex_unlock(&s->mutex);
#endif
}

static void rech_task_sync_wait(rech_task_sync *s) {
#ifdef _WIN32
    WaitForSingleObject(s->done_event, INFINITE);
#else
    pthread_mutex_lock(&s->mutex);
    while (!s->done) {
        pthread_cond_wait(&s->cond, &s->mutex);
    }
    pthread_mutex_unlock(&s->mutex);
#endif
}

`

// ensureThreadLaunch эмитит запуск задачи сигнатуры на потоке ОС
// (rech_launch_...): точка входа потока и функция ожидания.
func ensureThreadLaunch(ctx *irGenCtx, name string) (taskSig, error) {
	ts, err := ensureTaskType(ctx, name)
	if err != nil {
		return ts, err
	}
	ctx.usesThreads = true
	if ctx.threadLaunchEmitted[name] {
		return ts, nil
	}
	ctx.threadLaunchEmitted[name] = true
	var sb strings.Builder
	if !ctx.taskSyncEmitted {
		ctx.taskSyncEmitted = true
		sb.WriteString(taskSyncCode)
	}
	id := transliterate(name)

	fmt.Fprintf(&sb, "static void rech_task_wait_thread_%s(%s *t) {\n", id, ts.taskType)
	sb.WriteString("    rech_task_sync_wait((rech_task_sync *)t->sync);\n")
	sb.WriteString("}\n")

	entry := "rech_task_entry_" + id
	fmt.Fprintf(&sb, "#ifdef _WIN32\nstatic DWORD WINAPI %s(LPVOID rech_arg) {\n#else\nstatic void *%s(void *rech_arg) {\n#endif\n", entry, entry)
	fmt.Fprintf(&sb, "    %s *t = (%s *)rech_arg;\n", ts.taskType, ts.taskType)
	ts.runUnderCatch(&sb)
	// Сигнал - после t->result/t->exc: "дождаться" видит их готовыми.
	sb.WriteString("    rech_task_sync_signal((rech_task_sync *)t->sync);\n")
	sb.WriteString("    rech_active_tasks_dec();\n")
	sb.WriteString("#ifdef _WIN32\n    return 0;\n#else\n    return NULL;\n#endif\n")
	sb.WriteString("}\n")

	fmt.Fprintf(&sb, "static %s *%s(%s) {\n", ts.taskType, mangleTaskLaunch(name), ts.paramList())
	ts.fillTask(&sb)
	fmt.Fprintf(&sb, "    t->wait = rech_task_wait_thread_%s;\n", id)
	sb.WriteString("    t->sync = rech_task_sync_new();\n")
	// Счётчик - до постановки в очередь: "дождаться_всех_задач" не должна
	// застать его ещё не увеличенным.
	sb.WriteString("    rech_active_tasks_inc();\n")
	fmt.Fprintf(&sb, "    rech_pool_submit(%s, t);\n", entry)
	sb.WriteString("    return t;\n")
	sb.WriteString("}\n\n")
	ctx.taskCode.WriteString(sb.String())
	return ts, nil
}

// ensureCoroLaunch эмитит запуск задачи сигнатуры на планировщике корутин
// (rech_coro_launch_...): точка входа корутины и функция ожидания.
func ensureCoroLaunch(ctx *irGenCtx, name string) (taskSig, error) {
	ts, err := ensureTaskType(ctx, name)
	if err != nil {
		return ts, err
	}
	ctx.usesCoroutines = true
	// rech_coro_wait_done паникует на недопустимом ожидании (coroutine.go).
	ctx.usesPanic = true
	if ctx.coroLaunchEmitted[name] {
		return ts, nil
	}
	ctx.coroLaunchEmitted[name] = true
	id := transliterate(name)
	var sb strings.Builder

	fmt.Fprintf(&sb, "static void rech_task_wait_coro_%s(%s *t) {\n", id, ts.taskType)
	sb.WriteString("    rech_coro_wait_done(&t->coro_done);\n")
	sb.WriteString("}\n")

	entry := "rech_coro_entry_" + id
	fmt.Fprintf(&sb, "static void %s(void *rech_arg) {\n", entry)
	fmt.Fprintf(&sb, "    %s *t = (%s *)rech_arg;\n", ts.taskType, ts.taskType)
	ts.runUnderCatch(&sb)
	sb.WriteString("    t->coro_done = 1;\n")
	sb.WriteString("}\n")

	fmt.Fprintf(&sb, "static %s *%s(%s) {\n", ts.taskType, mangleCoroLaunch(name), ts.paramList())
	ts.fillTask(&sb)
	fmt.Fprintf(&sb, "    t->wait = rech_task_wait_coro_%s;\n", id)
	sb.WriteString("    t->coro_done = 0;\n")
	fmt.Fprintf(&sb, "    rech_coro_spawn(%s, t);\n", entry)
	sb.WriteString("    return t;\n")
	sb.WriteString("}\n\n")
	ctx.taskCode.WriteString(sb.String())
	return ts, nil
}

// generateIRLaunchExpr - "запустить ..."/"поручить ..." и сахар
// "дождаться <асинхронная функция>(...)" (тогда внутри AwaitExpr).
func generateIRLaunchExpr(ctx *irGenCtx, e *ir.LaunchExpr) (string, valueType, error) {
	sigName := ir.TypeOf(e.Value).Entity
	launch := mangleTaskLaunch(sigName)
	if e.Coroutine {
		if _, err := ensureCoroLaunch(ctx, sigName); err != nil {
			return "", typeUnknown, err
		}
		launch = mangleCoroLaunch(sigName)
	} else if _, err := ensureThreadLaunch(ctx, sigName); err != nil {
		return "", typeUnknown, err
	}
	fnCode, _, err := generateIRExpr(ctx, e.Value)
	if err != nil {
		return "", typeUnknown, err
	}
	codes := []string{fnCode}
	for _, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		codes = append(codes, code)
	}
	decls, codes := sequenceArgs(ctx, append([]ir.Expr{e.Value}, e.Args...), codes)
	return sequenced(decls, fmt.Sprintf("%s(%s)", launch, strings.Join(codes, ", "))), taskType(sigName), nil
}

// generateIRAwaitExpr - "дождаться <выражение>": rech_await_...(задача) -
// вид исполнения задача знает сама (t->wait).
func generateIRAwaitExpr(ctx *irGenCtx, e *ir.AwaitExpr) (string, valueType, error) {
	sigName := ir.TypeOf(e.Task).Entity
	if _, err := ensureTaskType(ctx, sigName); err != nil {
		return "", typeUnknown, err
	}
	code, _, err := generateIRExpr(ctx, e.Task)
	if err != nil {
		return "", typeUnknown, err
	}
	return fmt.Sprintf("%s(%s)", mangleTaskAwait(sigName), code), fromIRType(e.Type), nil
}
