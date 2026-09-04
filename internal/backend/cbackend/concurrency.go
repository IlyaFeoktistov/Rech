// Параллелизм ("запустить"/"дождаться", шаг 15) - потоки ОС (pthreads на
// POSIX, CreateThread на Windows), не зелёные потоки/event loop. Boehm GC
// уже используется для ссылочных сущностей (usesGC) - каждый новый поток
// обязан зарегистрироваться в нём (GC_register_my_thread) перед первым
// GC_MALLOC и разрегистрироваться перед завершением, иначе сборщик не
// увидит корни на его стеке - см. ensureTaskWrapper.
//
// Обёртка (C-структура + функция-точка входа потока + rech_launch_.../
// rech_await_...) - ОДНА НА КАЖДУЮ КОГДА-ЛИБО ЗАПУЩЕННУЮ ФУНКЦИЮ, не одна
// на тип результата (как у Список<T> и подобных, generateIRExpr, package
// doc GenericInstantiation): у разных функций разное число/типы
// параметров, единая C-структура "по типу результата" не смогла бы нести
// произвольный набор аргументов. ensureTaskWrapper эмитит её один раз,
// при первой встрече LaunchExpr/AwaitExpr для этой функции (тот же приём,
// что и ctx.usesShow/showFuncs - накопление в отдельный builder по ходу
// генерации тела, вставляемый в файл в нужном текстовом месте в конце,
// см. ctx.taskCode в GenerateFromIR).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func mangleTaskType(funcName string) string  { return "rech_TASK_" + transliterate(funcName) }
func mangleTaskEntry(funcName string) string { return "rech_task_entry_" + transliterate(funcName) }
func mangleTaskLaunch(funcName string) string {
	return "rech_launch_" + transliterate(funcName)
}
func mangleTaskAwait(funcName string) string { return "rech_await_" + transliterate(funcName) }

// ensureTaskWrapper эмитит C-обёртку для funcName в ctx.taskCode, если её
// там ещё нет (ctx.launchedFuncs - дедупликация: одна и та же функция
// может запускаться "запустить" много раз в программе).
func ensureTaskWrapper(ctx *irGenCtx, funcName string) {
	if ctx.launchedFuncs[funcName] {
		return
	}
	ctx.launchedFuncs[funcName] = true
	ctx.usesGC = true
	// usesExceptions - точка входа потока всегда исполняет обёрнутую
	// функцию под кадром "попытаться" (см. ниже), даже если сама программа
	// нигде не пишет "попытаться"/"выбросить" явно - без catch-frame
	// исключение внутри задачи просто уронило бы процесс (rech_throw без
	// rech_catch_top не отличит "некому ловить в этом потоке" от "некому
	// ловить нигде вообще").
	ctx.usesExceptions = true

	fn := ctx.irFuncs[funcName]
	taskType := mangleTaskType(funcName)
	resultC := cReturnTypeFor(ctx, fn.HasReturn, fn.ReturnType)
	argFields := make([]string, len(fn.Params))
	argNames := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		argNames[i] = "arg_" + transliterate(p.Name)
		argFields[i] = fmt.Sprintf("    %s %s;\n", fromIRType(p.Type).cType(ctx), argNames[i])
	}

	var sb strings.Builder

	// Структура задачи: аргументы обёрнутой функции, место под результат
	// (тот же C-тип, что и у самой функции - "int"-заглушка для функции без
	// объявленного типа результата, cReturnTypeFor, ровно как у обычного
	// вызова), исключение (NULL - не было) и сигнал завершения - НЕ
	// дескриптор потока ОС (см. package doc ниже, rech_launch_.../
	// rech_await_... - поток открепляется от задачи сразу при запуске,
	// дескриптор ему не нужен вовсе).
	fmt.Fprintf(&sb, "typedef struct {\n")
	for _, f := range argFields {
		sb.WriteString(f)
	}
	fmt.Fprintf(&sb, "    %s result;\n", resultC)
	sb.WriteString("    void *exc;\n")
	// done/mutex/cond (POSIX) или done_event (Windows) - сигнал завершения
	// потока, НЕ его дескриптор (HANDLE/pthread_t) - "запустить" без
	// единого "дождаться" (осознанный сценарий "фоновой задачи", docs/
	// SPEC.md, раздел 15 - консоль управления/задача на КАЖДОЕ HTTP-
	// соединение http_сервера) иначе никогда не освобождал бы дескриптор
	// потока (pthread_join/CloseHandle раньше вызывались ТОЛЬКО внутри
	// "дождаться") - на POSIX непристёгнутый (non-detached) поток без
	// join держит занятой запись в таблице потоков ОС ДАЖЕ ПОСЛЕ
	// завершения своей функции - при потоке на каждое короткоживущее
	// HTTP-соединение это утечка, которая рано или поздно исчерпывает
	// лимит потоков процесса и роняет pthread_create. Исправлено - поток
	// теперь ВСЕГДА открепляется
	// (pthread_detach/закрытие HANDLE) сразу при запуске, а "дождаться"
	// ждёт этот сигнал вместо потока напрямую - тот же класс "утечки при
	// отсутствии дождаться", что и у самой структуры задачи (GC_MALLOC,
	// никогда не освобождается явно) - маленький кусок памяти/примитив
	// синхронизации, не дефицитный ресурс ядра ОС.
	sb.WriteString("#ifdef _WIN32\n    HANDLE done_event;\n#else\n    pthread_mutex_t mutex;\n    pthread_cond_t cond;\n    int done;\n#endif\n")
	fmt.Fprintf(&sb, "} %s;\n", taskType)

	// Функция-точка входа потока - исполняет тело обёрнутой функции под тем
	// же кадром "попытаться" (rech_catch_frame, exceptions.go), что и
	// обычный вызов - необработанное внутри исключение оседает в t->exc
	// вместо того, чтобы уронить процесс здесь же (см. rech_await_... ниже -
	// оно поднимается заново уже в дожидающемся потоке). Регистрация в
	// Boehm GC - НЕ вручную (GC_register_my_thread): rech_launch_...
	// создаёт поток через pthread_create/CreateThread как есть, а не
	// напрямую - при подключённом GC_THREADS <gc.h> сам подменяет их на
	// GC_pthread_create/GC_CreateThread через макрос (gc_pthread_redirects.h/
	// win32-редирект), которые регистрируют и разрегистрируют поток
	// автоматически - двойная (авто + ручная) регистрация ловилась падением
	// при выходе из потока на практике, не только в теории.
	entry := mangleTaskEntry(funcName)
	fmt.Fprintf(&sb, "#ifdef _WIN32\nstatic DWORD WINAPI %s(LPVOID rech_arg) {\n#else\nstatic void *%s(void *rech_arg) {\n#endif\n", entry, entry)
	fmt.Fprintf(&sb, "    %s *t = (%s *)rech_arg;\n", taskType, taskType)
	fmt.Fprintf(&sb, "    %s rech_frame;\n", catchFrameType)
	fmt.Fprintf(&sb, "    rech_frame.prev = %s;\n", catchTopVar)
	fmt.Fprintf(&sb, "    %s = &rech_frame;\n", catchTopVar)
	sb.WriteString("    if (setjmp(rech_frame.buf) == 0) {\n")
	argRefs := make([]string, len(argNames))
	for i, n := range argNames {
		argRefs[i] = "t->" + n
	}
	fmt.Fprintf(&sb, "        t->result = %s(%s);\n", mangleName(funcName), strings.Join(argRefs, ", "))
	fmt.Fprintf(&sb, "        %s = rech_frame.prev;\n", catchTopVar)
	sb.WriteString("    } else {\n")
	fmt.Fprintf(&sb, "        %s = rech_frame.prev;\n", catchTopVar)
	sb.WriteString("        t->exc = rech_frame.exc;\n")
	sb.WriteString("    }\n")
	// Сигнал завершения - ПОСЛЕ t->result/t->exc (см. package doc выше) -
	// "дождаться" видит их уже готовыми к моменту, когда просыпается по
	// этому сигналу (упорядочивание обеспечено самой блокировкой/событием,
	// не нужен отдельный барьер памяти).
	sb.WriteString("#ifdef _WIN32\n")
	sb.WriteString("    SetEvent(t->done_event);\n")
	sb.WriteString("#else\n")
	sb.WriteString("    pthread_mutex_lock(&t->mutex);\n")
	sb.WriteString("    t->done = 1;\n")
	sb.WriteString("    pthread_cond_signal(&t->cond);\n")
	sb.WriteString("    pthread_mutex_unlock(&t->mutex);\n")
	sb.WriteString("#endif\n")
	// После сигнала завершения - "дождаться_всех_задач" не обязано увидеть
	// оба события синхронно относительно друг друга (это два независимых
	// потребителя: "дождаться" на КОНКРЕТНУЮ задачу читает t->done/t->exc,
	// "дождаться_всех_задач" - только общий счётчик).
	sb.WriteString("    rech_active_tasks_dec();\n")
	sb.WriteString("#ifdef _WIN32\n    return 0;\n#else\n    return NULL;\n#endif\n")
	sb.WriteString("}\n")

	// rech_launch_... - выделяет структуру задачи (GC_MALLOC - переживает
	// возврат из этой функции, время жизни задачи не привязано ни к какому
	// стеку) и запускает поток.
	launch := mangleTaskLaunch(funcName)
	paramList := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		paramList[i] = fmt.Sprintf("%s %s", fromIRType(p.Type).cType(ctx), argNames[i])
	}
	paramListC := "void"
	if len(paramList) > 0 {
		paramListC = strings.Join(paramList, ", ")
	}
	fmt.Fprintf(&sb, "static %s *%s(%s) {\n", taskType, launch, paramListC)
	fmt.Fprintf(&sb, "    %s *t = (%s *)GC_MALLOC(sizeof(%s));\n", taskType, taskType, taskType)
	for _, n := range argNames {
		fmt.Fprintf(&sb, "    t->%s = %s;\n", n, n)
	}
	sb.WriteString("    t->exc = NULL;\n")
	// Считается ДО создания потока (симметрично rech_active_tasks_dec в
	// точке входа потока, ниже) - "дождаться_всех_задач" (active_tasks.go)
	// иначе могла бы застать счётчик ещё не увеличенным для только что
	// запущенной, но фактически ещё не начавшей исполняться задачи.
	sb.WriteString("    rech_active_tasks_inc();\n")
	// Примитив синхронизации - ДО создания потока (сама функция-точка входа
	// signal'ит его по завершении, см. выше - обязан существовать раньше,
	// чем поток вообще может успеть завершиться и попытаться его тронуть).
	// Поток открепляется СРАЗУ (закрытие HANDLE на Windows, pthread_detach
	// на POSIX) - его дескриптор дальше не нужен вообще, "дождаться" ждёт
	// done_event/mutex+cond, не сам поток (см. package doc структуры
	// задачи выше).
	sb.WriteString("#ifdef _WIN32\n")
	sb.WriteString("    t->done_event = CreateEvent(NULL, TRUE, FALSE, NULL);\n")
	fmt.Fprintf(&sb, "    HANDLE rech_thread_handle = CreateThread(NULL, 0, %s, t, 0, NULL);\n", entry)
	sb.WriteString("    CloseHandle(rech_thread_handle);\n")
	sb.WriteString("#else\n")
	sb.WriteString("    pthread_mutex_init(&t->mutex, NULL);\n")
	sb.WriteString("    pthread_cond_init(&t->cond, NULL);\n")
	sb.WriteString("    t->done = 0;\n")
	sb.WriteString("    pthread_t rech_thread_handle;\n")
	fmt.Fprintf(&sb, "    pthread_create(&rech_thread_handle, NULL, %s, t);\n", entry)
	sb.WriteString("    pthread_detach(rech_thread_handle);\n")
	sb.WriteString("#endif\n")
	sb.WriteString("    return t;\n")
	sb.WriteString("}\n")

	// rech_await_... - дожидается завершения потока и либо поднимает
	// исключение заново (rech_throw - тот же кадр "попытаться", что и у
	// обычного вызова, но уже в потоке, вызвавшем "дождаться"), либо
	// возвращает результат.
	await := mangleTaskAwait(funcName)
	fmt.Fprintf(&sb, "static %s %s(%s *t) {\n", resultC, await, taskType)
	// Ждёт СИГНАЛ завершения (done_event/mutex+cond), не сам поток - тот
	// уже откреплён и закрыт в rech_launch_... (см. package doc структуры
	// задачи выше) - освобождает примитив синхронизации здесь же (в
	// отличие от структуры задачи целиком - GC_MALLOC, отдельно не
	// освобождается никогда).
	sb.WriteString("#ifdef _WIN32\n")
	sb.WriteString("    WaitForSingleObject(t->done_event, INFINITE);\n")
	sb.WriteString("    CloseHandle(t->done_event);\n")
	sb.WriteString("#else\n")
	sb.WriteString("    pthread_mutex_lock(&t->mutex);\n")
	sb.WriteString("    while (!t->done) {\n")
	sb.WriteString("        pthread_cond_wait(&t->cond, &t->mutex);\n")
	sb.WriteString("    }\n")
	sb.WriteString("    pthread_mutex_unlock(&t->mutex);\n")
	sb.WriteString("    pthread_mutex_destroy(&t->mutex);\n")
	sb.WriteString("    pthread_cond_destroy(&t->cond);\n")
	sb.WriteString("#endif\n")
	sb.WriteString("    if (t->exc != NULL) {\n")
	fmt.Fprintf(&sb, "        %s(t->exc, %s);\n", throwFunc, throwTypeNameExpr(ctx, "t->exc", systemExceptionEntity))
	sb.WriteString("    }\n")
	sb.WriteString("    return t->result;\n")
	sb.WriteString("}\n\n")

	ctx.taskCode.WriteString(sb.String())
}

// generateIRLaunchExpr - "запустить <имя>(<арг>, ...)".
func generateIRLaunchExpr(ctx *irGenCtx, e *ir.LaunchExpr) (string, valueType, error) {
	ctx.usesThreads = true
	ensureTaskWrapper(ctx, e.Callee)
	argCodes := make([]string, len(e.Args))
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i] = code
	}
	return fmt.Sprintf("%s(%s)", mangleTaskLaunch(e.Callee), strings.Join(argCodes, ", ")), taskType(e.Callee), nil
}

// generateIRAwaitExpr - "дождаться <выражение>".
func generateIRAwaitExpr(ctx *irGenCtx, e *ir.AwaitExpr) (string, valueType, error) {
	ctx.usesThreads = true
	ensureTaskWrapper(ctx, e.Callee)
	code, _, err := generateIRExpr(ctx, e.Task)
	if err != nil {
		return "", typeUnknown, err
	}
	return fmt.Sprintf("%s(%s)", mangleTaskAwait(e.Callee), code), fromIRType(e.Type), nil
}
