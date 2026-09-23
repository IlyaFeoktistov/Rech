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
//
// САМ ПОТОК ОС, на котором исполняется точка входа (rech_task_entry_...),
// НЕ создаётся заново на каждый "запустить" - rech_launch_... ставит
// задачу в очередь общего для всего процесса пула воркеров
// (rech_pool_submit, threadpool.go), новый поток ОС там заводится, только
// когда простаивающих воркеров в моменте нет ("что ещё
// нужно для настоящего HTTP highload", пункт 1). Обёртка каждой обёрнутой
// функции (структура задачи + точка входа) от этого не меняется вовсе -
// пул воркеров не знает НИЧЕГО о конкретных обёрнутых функциях, только
// зовёт то, что ему передали (указатель на entry + указатель на структуру
// задачи).
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
	// дескриптор потока ОС (задача не привязана к КОНКРЕТНОМУ потоку -
	// см. package doc выше, её обслуживает пул воркеров, дескриптор
	// исполнившего её потока структуре задачи не нужен вовсе).
	fmt.Fprintf(&sb, "typedef struct %s {\n", taskType)
	for _, f := range argFields {
		sb.WriteString(f)
	}
	fmt.Fprintf(&sb, "    %s result;\n", resultC)
	sb.WriteString("    void *exc;\n")
	// done/mutex/cond (POSIX) или done_event (Windows) - сигнал завершения
	// ЗАДАЧИ, НЕ дескриптор потока, который её исполнил - "запустить" без
	// единого "дождаться" (осознанный сценарий "фоновой задачи", docs/
	// SPEC.md, раздел 15 - консоль управления/задача на КАЖДОЕ HTTP-
	// соединение http_сервера) не должен требовать, чтобы кто-то когда-либо
	// дождался именно этого потока - воркеры пула (threadpool.go) все
	// открепляются (pthread_detach/закрытие HANDLE) сразу при создании и
	// живут ДОЛЬШЕ одной задачи (переиспользуются), "дождаться" ждёт этот
	// сигнал, не поток напрямую - тот же класс "утечки при отсутствии
	// дождаться", что и у самой структуры задачи (GC_MALLOC, никогда не
	// освобождается явно) - маленький кусок памяти/примитив синхронизации,
	// не дефицитный ресурс ядра ОС.
	sb.WriteString("#ifdef _WIN32\n    HANDLE done_event;\n#else\n    pthread_mutex_t mutex;\n    pthread_cond_t cond;\n    int done;\n#endif\n")
	fmt.Fprintf(&sb, "} %s;\n", taskType)

	// Функция-точка входа потока - исполняет тело обёрнутой функции под тем
	// же кадром "попытаться" (rech_catch_frame, exceptions.go), что и
	// обычный вызов - необработанное внутри исключение оседает в t->exc
	// вместо того, чтобы уронить процесс здесь же (см. rech_await_... ниже -
	// оно поднимается заново уже в дожидающемся потоке); rech_catch_top
	// (exceptions.go) - потокоуказательная (RECH_THREAD_LOCAL) переменная,
	// поэтому вызов entry ДРУГОЙ, уже переиспользуемой задачей на том же
	// самом воркере пула (threadpool.go) корректно начинает СВОЙ собственный
	// кадр, не путается с предыдущей задачей, когда-либо исполнявшейся этим
	// же потоком. Регистрация в Boehm GC - НЕ вручную (GC_register_my_thread):
	// поток ОС создаётся через pthread_create/CreateThread как есть только
	// ВНУТРИ пула (rech_pool_submit, threadpool.go, когда простаивающих
	// воркеров нет), не здесь - при подключённом GC_THREADS <gc.h> сам
	// подменяет их на GC_pthread_create/GC_CreateThread через макрос
	// (gc_pthread_redirects.h/win32-редирект), которые регистрируют поток
	// автоматически один раз на каждый воркер (не на каждую задачу, как
	// было бы при создании потока на каждый "запустить") - двойная
	// (авто + ручная) регистрация ловилась падением при выходе из потока на
	// практике, не только в теории.
	entry := mangleTaskEntry(funcName)
	fmt.Fprintf(&sb, "#ifdef _WIN32\nstatic DWORD WINAPI %s(LPVOID rech_arg) {\n#else\nstatic void *%s(void *rech_arg) {\n#endif\n", entry, entry)
	fmt.Fprintf(&sb, "    %s *t = (%s *)rech_arg;\n", taskType, taskType)
	// "= {0}" - НАЙДЕННЫЙ И ИСПРАВЛЕННЫЙ баг (проявился только при вводе
	// пула воркеров, threadpool.go, живым прогоном examples/параллелизм/
	// задачи.rech: третья задача, которая должна была быть поймана
	// "поймать (иск: ОшибкаВвода)", улетала как необработанное исключение) -
	// catch_panics обязан быть гарантированным false у ЭТОГО кадра (тот же
	// принцип, что и у обычного "попытаться", см. generateIRTryStmt,
	// exceptions.go, "= {0}") - без явного нулевого инициализатора
	// оставался неопределённым мусором стека. На ОТДЕЛЬНОМ, ни разу не
	// использованном потоке ОС (было раньше - "запустить" создавал новый
	// поток на каждую задачу) свежая страница стека почти всегда нулевая
	// (деталь распределения стека потока в конкретной ОС, не гарантия языка
	// C), поэтому мусор случайно читался как false и баг не проявлялся;
	// воркер пула переиспользует ОДИН И ТОТ ЖЕ стек между разными задачами -
	// "мусор" на месте catch_panics оказывается вполне реальным значением,
	// оставшимся от чужого предыдущего кадра, когда-либо жившего на этом же
	// участке стека.
	fmt.Fprintf(&sb, "    %s rech_frame = {0};\n", catchFrameType)
	fmt.Fprintf(&sb, "    rech_frame.prev = %s;\n", catchTopVar)
	fmt.Fprintf(&sb, "    %s = &rech_frame;\n", catchTopVar)
	sb.WriteString("    if (setjmp(rech_frame.buf) == 0) {\n")
	argRefs := make([]string, len(argNames))
	for i, n := range argNames {
		argRefs[i] = "t->" + n
	}
	fmt.Fprintf(&sb, "        t->result = %s(%s);\n", funcCName(ctx, funcName), strings.Join(argRefs, ", "))
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
	// Примитив синхронизации - ДО постановки задачи в очередь пула (сама
	// функция-точка входа signal'ит его по завершении, см. выше - обязан
	// существовать раньше, чем задача вообще может успеть исполниться и
	// попытаться его тронуть - воркер пула может подхватить задачу СРАЗУ
	// же, если в пуле уже есть простаивающий поток). rech_pool_submit
	// (threadpool.go) - вместо прямого pthread_create/CreateThread на
	// каждую задачу здесь же: воркер, который исполнит entry, может быть
	// уже существующим (переиспользуемым), не обязательно новым - "поток
	// открепляется/закрывается" (было раньше) теперь целиком забота пула,
	// не этой функции.
	sb.WriteString("#ifdef _WIN32\n")
	sb.WriteString("    t->done_event = CreateEvent(NULL, TRUE, FALSE, NULL);\n")
	sb.WriteString("#else\n")
	sb.WriteString("    pthread_mutex_init(&t->mutex, NULL);\n")
	sb.WriteString("    pthread_cond_init(&t->cond, NULL);\n")
	sb.WriteString("    t->done = 0;\n")
	sb.WriteString("#endif\n")
	fmt.Fprintf(&sb, "    rech_pool_submit(%s, t);\n", entry)
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

// generateIRLaunchExpr - "запустить <имя>(<арг>, ...)" ИЛИ "поручить
// <асинхронная функция>(...)" (e.Coroutine == true, Шаг 132 - см. package
// doc coroutine_task.go, generateIRCoroLaunchExpr) - LaunchExpr как
// САМОСТОЯТЕЛЬНОЕ выражение (не внутри AwaitExpr, тот перехватывает
// коротинный случай раньше, см. generateIRAwaitExpr ниже) - именно этот
// путь и достигается для "поручить" (сахар "дождаться <асинхронная
// функция>()" никогда не доходит сюда вовсе, см. package doc
// generateIRAwaitExpr).
func generateIRLaunchExpr(ctx *irGenCtx, e *ir.LaunchExpr) (string, valueType, error) {
	if e.Coroutine {
		return generateIRCoroLaunchExpr(ctx, e)
	}
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

// generateIRAwaitExpr - "дождаться <выражение>". Два коротинных случая
// (см. package doc coroutine_task.go) - launch.Coroutine == true И Task
// синтаксически прямой LaunchExpr (сахар "дождаться <асинхронная
// функция>(...)", internal/semantic/concurrency.go, analyzeAwaitTask) -
// composes launch+join сама, НЕ вызывая generateIRExpr(ctx, e.Task) вовсе
// (который эмитил бы отдельный rech_coro_launch_.../нарушил бы
// "запустить+дождаться в один присест"); ЛИБО Task - произвольное
// выражение (обычно переменная), чей ТИП - KindCoroTask (результат
// РАНЕЕ исполненного "поручить", Шаг 132) - тогда просто join над уже
// готовым значением. Иначе (KindTask, поток ОС) - прежний путь.
func generateIRAwaitExpr(ctx *irGenCtx, e *ir.AwaitExpr) (string, valueType, error) {
	if launch, ok := e.Task.(*ir.LaunchExpr); ok && launch.Coroutine {
		return generateIRCoroAwaitExpr(ctx, e, launch)
	}
	if ir.TypeOf(e.Task).Kind == ir.KindCoroTask {
		return generateIRCoroJoinFromVar(ctx, e)
	}
	ctx.usesThreads = true
	ensureTaskWrapper(ctx, e.Callee)
	code, _, err := generateIRExpr(ctx, e.Task)
	if err != nil {
		return "", typeUnknown, err
	}
	return fmt.Sprintf("%s(%s)", mangleTaskAwait(e.Callee), code), fromIRType(e.Type), nil
}
