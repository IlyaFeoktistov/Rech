// Кодогенерация асинхронных функций на лёгком планировщике корутин -
// "дождаться <асинхронная функция>(...)" (сахар, ir.LaunchExpr.Coroutine
// == true, см. package doc internal/ir/concurrency.go, internal/semantic/
// concurrency.go) и "поручить <асинхронная функция>(...)" (фоновый запуск
// без немедленного ожидания, Шаг 132, ast.CoroLaunchExpr) - обёртка ОДНА
// НА КАЖДУЮ КОГДА-ЛИБО ТАК ЗАПУЩЕННУЮ асинхронную функцию, тем же приёмом,
// что и ensureTaskWrapper (concurrency.go) для потоков ОС - структура
// задачи + точка входа + ДВЕ функции-обёртки (в отличие от потоков),
// накапливаются в ctx.coroTaskCode, вставляются в файл один раз, в конце.
//
// ДВЕ функции, не одна фьюженная - rech_coro_launch_<имя> (спавн, сразу
// возвращает указатель на задачу, НЕ ждёт) и rech_coro_join_<имя> (ждёт
// *done - rech_coro_wait_done, coroutine.go - затем throw/результат) -
// именно это разделение и даёт "поручить" (спавн без ожидания). Сахар
// "дождаться <асинхронная функция>()" (generateIRCoroAwaitExpr) - просто
// КОМПОЗИЦИЯ этих двух: rech_coro_join_<имя>(rech_coro_launch_<имя>(...)) -
// та же форма, что у потоков (rech_await_foo(rech_launch_foo(...))), не
// отдельный третий вариант кода.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func mangleCoroTaskType(funcName string) string { return "rech_CORO_TASK_" + transliterate(funcName) }
func mangleCoroEntry(funcName string) string    { return "rech_coro_entry_" + transliterate(funcName) }
func mangleCoroLaunch(funcName string) string   { return "rech_coro_launch_" + transliterate(funcName) }
func mangleCoroJoin(funcName string) string     { return "rech_coro_join_" + transliterate(funcName) }

// ensureCoroTaskWrapper эмитит C-обёртку для funcName в ctx.coroTaskCode,
// если её там ещё нет (ctx.coroutineFuncs - та же дедупликация, что и
// launchedFuncs у потоков).
func ensureCoroTaskWrapper(ctx *irGenCtx, funcName string) {
	if ctx.coroutineFuncs[funcName] {
		return
	}
	ctx.coroutineFuncs[funcName] = true
	ctx.usesCoroutines = true
	ctx.usesGC = true
	ctx.usesExceptions = true
	// usesPanic - rech_coro_wait_done (coroutine.go) паникует явно на
	// вложенный "дождаться <асинхронная функция>" - см. package doc
	// coroutine.go.
	ctx.usesPanic = true

	fn := ctx.irFuncs[funcName]
	taskType := mangleCoroTaskType(funcName)
	resultC := cReturnTypeFor(ctx, fn.HasReturn, fn.ReturnType)
	argNames := make([]string, len(fn.Params))
	argFields := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		argNames[i] = "arg_" + transliterate(p.Name)
		argFields[i] = fmt.Sprintf("    %s %s;\n", fromIRType(p.Type).cType(ctx), argNames[i])
	}

	var sb strings.Builder

	// Структура задачи - тот же состав полей, что и у потоковой версии
	// (concurrency.go, ensureTaskWrapper), кроме синхронизации: "done" -
	// голый int, не mutex/cond/event - вся работа на ОДНОМ И ТОМ ЖЕ потоке
	// ОС по очереди, а не параллельно - примитив синхронизации между
	// потоками не нужен вовсе.
	fmt.Fprintf(&sb, "typedef struct {\n")
	for _, f := range argFields {
		sb.WriteString(f)
	}
	fmt.Fprintf(&sb, "    %s result;\n", resultC)
	sb.WriteString("    void *exc;\n")
	sb.WriteString("    int done;\n")
	fmt.Fprintf(&sb, "} %s;\n", taskType)

	// Точка входа корутины - тот же catch-frame (setjmp/rech_catch_top),
	// что и у точки входа потока (ensureTaskWrapper) - rech_catch_top живёт
	// на ЭТОМ ЖЕ потоке ОС (корутина не заводит новый), поэтому работает
	// корректно без единой правки: каждая корутина видит ровно тот
	// rech_catch_top, который был активен на этом потоке В МОМЕНТ, когда
	// планировщик в неё переключился (см. rech_frame.prev - восстанавливает
	// ровно то значение, что было ДО входа в эту корутину, а не ДО всей
	// программы).
	entry := mangleCoroEntry(funcName)
	fmt.Fprintf(&sb, "static void %s(void *rech_arg) {\n", entry)
	fmt.Fprintf(&sb, "    %s *t = (%s *)rech_arg;\n", taskType, taskType)
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
	sb.WriteString("    t->done = 1;\n")
	sb.WriteString("}\n")

	// rech_coro_launch_<имя> - выделяет структуру задачи (GC_MALLOC - та же
	// причина, что и у потоковой версии: переживает возврат из этой
	// функции) и спавнит корутину над entry (rech_coro_spawn, coroutine.go -
	// БЕЗОПАСНО вызывать даже изнутри уже исполняющейся корутины, см. её
	// package doc) - возвращает указатель НЕМЕДЛЕННО, не дожидаясь ничего.
	// "поручить" зовёт ровно эту функцию и ничего больше.
	launch := mangleCoroLaunch(funcName)
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
	sb.WriteString("    t->done = 0;\n")
	fmt.Fprintf(&sb, "    rech_coro_spawn(%s, t);\n", entry)
	sb.WriteString("    return t;\n")
	sb.WriteString("}\n")

	// rech_coro_join_<имя> - ждёт (rech_coro_wait_done, coroutine.go) уже
	// СПАВНЕННУЮ (rech_coro_launch_<имя> выше - самой этой функцией или
	// раньше, отдельно, через "поручить") задачу, поднимает исключение
	// заново (тот же rech_throw, что и у потоковой версии) либо возвращает
	// результат. "дождаться" над результатом "поручить" зовёт эту функцию
	// напрямую; сахар "дождаться <асинхронная функция>()" -
	// generateIRCoroAwaitExpr ниже - composes launch+join за один раз.
	join := mangleCoroJoin(funcName)
	fmt.Fprintf(&sb, "static %s %s(%s *t) {\n", resultC, join, taskType)
	fmt.Fprintf(&sb, "    rech_coro_wait_done(&t->done);\n")
	sb.WriteString("    if (t->exc != NULL) {\n")
	fmt.Fprintf(&sb, "        %s(t->exc, %s);\n", throwFunc, throwTypeNameExpr(ctx, "t->exc", systemExceptionEntity))
	sb.WriteString("    }\n")
	sb.WriteString("    return t->result;\n")
	sb.WriteString("}\n\n")

	ctx.coroTaskCode.WriteString(sb.String())
}

// generateIRCoroAwaitExpr - "дождаться <асинхронная функция>(...)"
// (launch.Coroutine == true, прямой вызов, см. package doc выше) - НЕ
// вызывает generateIRLaunchExpr/generateIRExpr(ctx, e.Task) вовсе:
// компонует rech_coro_join_<имя>(rech_coro_launch_<имя>(args...)) сама.
func generateIRCoroAwaitExpr(ctx *irGenCtx, e *ir.AwaitExpr, launch *ir.LaunchExpr) (string, valueType, error) {
	ensureCoroTaskWrapper(ctx, launch.Callee)
	argCodes := make([]string, len(launch.Args))
	for i, a := range launch.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i] = code
	}
	return fmt.Sprintf("%s(%s(%s))", mangleCoroJoin(launch.Callee), mangleCoroLaunch(launch.Callee), strings.Join(argCodes, ", ")), fromIRType(e.Type), nil
}

// generateIRCoroLaunchExpr - "поручить <асинхронная функция>(...)"
// (launch.Coroutine == true, но НЕ немедленно оборачивается в "дождаться" -
// вызывается из generateIRLaunchExpr, concurrency.go, когда LaunchExpr
// встретился как обычное выражение, не внутри AwaitExpr) - ровно
// rech_coro_launch_<имя>(args...), без join.
func generateIRCoroLaunchExpr(ctx *irGenCtx, e *ir.LaunchExpr) (string, valueType, error) {
	ensureCoroTaskWrapper(ctx, e.Callee)
	argCodes := make([]string, len(e.Args))
	for i, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		argCodes[i] = code
	}
	return fmt.Sprintf("%s(%s)", mangleCoroLaunch(e.Callee), strings.Join(argCodes, ", ")), coroTaskType(e.Callee), nil
}

// generateIRCoroJoinFromVar - "дождаться <переменная>" (или любое другое
// выражение, кроме прямого вызова), несущее KindCoroTask (Шаг 132,
// результат ранее исполненного "поручить") - rech_coro_join_<имя>(<код
// выражения задачи>).
func generateIRCoroJoinFromVar(ctx *irGenCtx, e *ir.AwaitExpr) (string, valueType, error) {
	ensureCoroTaskWrapper(ctx, e.Callee)
	code, _, err := generateIRExpr(ctx, e.Task)
	if err != nil {
		return "", typeUnknown, err
	}
	return fmt.Sprintf("%s(%s)", mangleCoroJoin(e.Callee), code), fromIRType(e.Type), nil
}
