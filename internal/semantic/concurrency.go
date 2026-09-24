// Параллелизм ("запустить"/"дождаться") - см. docs/SPEC.md. Модель -
// потоки ОС (шаг 15): каждый "запустить" - настоящий
// поток, без собственного планировщика/event loop. Никакой flow-sensitive
// проверки гонок/захвата изменяемого состояния между потоками нет -
// открытый вопрос решён в пользу "оставить программисту", тем
// же принципом, что и обычные потоки ОС в других языках без встроенного
// borrow checker.
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// DefaultPoolIdleTimeoutMs - сколько миллисекунд воркер пула
// (internal/backend/cbackend/threadpool.go) простаивает без работы, прежде
// чем завершиться, если файл проекта включает сжатие ("потоки.сжатие_пула:
// истина") без явного "потоки.таймаут_простоя_мс" - см. internal/
// project.File.PoolIdleTimeoutMs/ir.Program.PoolIdleTimeoutMs. Значение "не
// слишком агрессивно, не слишком лениво" - тот же порядок величины, что и
// типичный keep-alive таймаут HTTP-сервера (http_сервер.rechproj), не
// привязано к нему напрямую.
const DefaultPoolIdleTimeoutMs = 30000

// analyzeLaunchExpr - "запустить <вызов>": три формы операнда (см. package
// doc ast.LaunchExpr), все сводятся к одному ir.LaunchExpr - запуску
// значения типа Вызываемый с аргументами:
//   - функция программы -> &функция (перегрузка выбирается по аргументам);
//   - переменная/параметр типа Вызываемый -> само значение;
//   - метод -> синтезированная лямбда, вызывающая метод на получателе,
//     захваченном в момент запуска (launchMethodLambda).
//
// Встроенные и внешние функции не запускаются - нет гарантии
// потокобезопасности библиотечного C-кода.
func analyzeLaunchExpr(sc *ctx, e *ast.LaunchExpr) (*ir.LaunchExpr, error) {
	if e.MethodCall != nil {
		return analyzeLaunchMethod(sc, e)
	}
	name := e.Call.Callee
	if typ, ok := sc.lookup(name); ok && typ.Kind == ir.KindCallable {
		call, err := analyzeCallValueExpr(sc, &ir.Identifier{Name: name, Type: typ}, e.Call)
		if err != nil {
			return nil, err
		}
		return newLaunch(sc, call.Value, call.Args, e.Line)
	}
	if builtins.IsReservedName(name) {
		return nil, errAtLine(e.Line, CodeSemanticOther, "\"запустить\" не поддерживает встроенные функции - только функции программы, значения типа \"Вызываемый\" и методы")
	}
	callee := name
	sig, mangled, overloaded, err := resolveFuncSigForCall(sc, name, sc.a.funcOverloads[name], e.Call.Args, e.Line)
	if overloaded {
		if err != nil {
			return nil, err
		}
		callee = mangled
	} else {
		var ok bool
		sig, ok = sc.funcs[name]
		if !ok {
			return nil, errAtLine(e.Line, CodeNameResolution, "функция %q не объявлена", name)
		}
	}
	if sig.IsExtern {
		return nil, errAtLine(e.Line, CodeSemanticOther, "\"запустить\" не поддерживает внешние функции (\"внешняя функция\") - нет гарантии потокобезопасности произвольного C-символа")
	}
	args, err := checkCallArgs(sc, e.Call.Args, sig, e.Line, fmt.Sprintf("функция %q", name), fmt.Sprintf("вызова %q", name))
	if err != nil {
		return nil, err
	}
	ref, err := funcRefFromSig(sc, callee, sig, e.Line)
	if err != nil {
		return nil, err
	}
	return newLaunch(sc, ref, args, e.Line)
}

// newLaunch - ir.LaunchExpr над value с уже проверенными args; тип задачи
// (сигнатура value) регистрируется для backend'ов тем же registerCallable,
// что и сама сигнатура.
func newLaunch(sc *ctx, value ir.Expr, args []ir.Expr, line int) (*ir.LaunchExpr, error) {
	if err := sc.a.registerCallable(ir.TypeOf(value)); err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: line})
	}
	return &ir.LaunchExpr{Value: value, Args: args}, nil
}

// launchReceiverCapture - имя захвата получателя в лямбде, синтезированной
// для "запустить объект.метод(...)" - не идентификатор Rech (пробел), так
// что не совпадёт ни с одним параметром/переменной.
const launchReceiverCapture = "получатель задачи"

// analyzeLaunchMethod - "запустить <получатель>.<метод>(<арг>, ...)":
// вызов метода анализируется как обычный, затем оборачивается в лямбду
//
//	функция(а1: Т1, ...): Р { вернуть <получатель>.<метод>(а1, ...) }
//
// где получатель - инициализирующий захват (ir.CaptureVar.Init) значения
// получателя в момент запуска: сущность разделяется с потоком по ссылке,
// структура копируется - то же правило, что у передачи аргумента.
func analyzeLaunchMethod(sc *ctx, e *ast.LaunchExpr) (*ir.LaunchExpr, error) {
	if e.MethodCall.Static {
		return nil, errAtLine(e.Line, CodeSemanticOther, "\"запустить\" поддерживает только методы экземпляра (\"объект.метод(...)\"), не статические")
	}
	analyzed, err := analyzeMethodCallExpr(sc, e.MethodCall)
	if err != nil {
		return nil, err
	}
	if cv, isField := analyzed.(*ir.CallValueExpr); isField {
		// "объект.поле(...)", где поле - Вызываемый: запускается само поле.
		return newLaunch(sc, cv.Value, cv.Args, e.Line)
	}
	mc, ok := analyzed.(*ir.MethodCallExpr)
	if !ok {
		return nil, errAtLine(e.Line, CodeSemanticOther, "\"запустить\" поддерживает только методы экземпляра (\"объект.метод(...)\")")
	}
	recvType := ir.TypeOf(mc.Receiver)
	params := make([]ir.Param, len(mc.Args))
	paramTypes := make([]ir.Type, len(mc.Args))
	callArgs := make([]ir.Expr, len(mc.Args))
	for i, a := range mc.Args {
		t := ir.TypeOf(a)
		name := fmt.Sprintf("аргумент задачи %d", i+1)
		params[i] = ir.Param{Name: name, Type: t}
		paramTypes[i] = t
		callArgs[i] = &ir.Identifier{Name: name, Type: t}
	}
	call := *mc
	call.Receiver = &ir.Identifier{Name: launchReceiverCapture, Type: recvType}
	call.Args = callArgs
	var result *ir.Type
	var body ir.Stmt
	if mc.HasReturn {
		rt := mc.Type
		result = &rt
		body = &ir.ReturnStmt{Value: &call, Line: e.Line}
	} else {
		body = &ir.ExprStmt{MethodCall: &call, Line: e.Line}
	}
	sc.a.lambdaCounter++
	lambda := &ir.LambdaExpr{
		Name:       fmt.Sprintf("__лямбда_%d", sc.a.lambdaCounter),
		Params:     params,
		ReturnType: mc.Type,
		HasReturn:  mc.HasReturn,
		Captures:   []ir.CaptureVar{{Name: launchReceiverCapture, Type: recvType, Init: mc.Receiver}},
		Body:       []ir.Stmt{body},
		Type:       ir.CallableType(paramTypes, result),
	}
	return newLaunch(sc, lambda, mc.Args, e.Line)
}

// analyzeCoroLaunchExpr - "поручить <асинхронная функция>(<арг>, ...)"
// (см. package doc ast.CoroLaunchExpr) - резолюция вызова та же, что у
// "запустить" (analyzeLaunchExpr), дополнительно требует функцию,
// объявленную "асинхронная": лямбды/методы/переменные на планировщик
// корутин не поручаются. Результат - тот же ir.LaunchExpr с
// Coroutine == true и тот же тип задачи, что у "запустить".
func analyzeCoroLaunchExpr(sc *ctx, e *ast.CoroLaunchExpr) (ir.Expr, error) {
	launch, err := analyzeLaunchExpr(sc, &ast.LaunchExpr{Call: e.Call, Line: e.Line})
	if err != nil {
		return nil, err
	}
	ref, ok := launch.Value.(*ir.FuncRefExpr)
	if sig, exists := sc.funcs[refName(ref, ok)]; !ok || !exists || !sig.IsAsync {
		return nil, errAtLine(e.Line, CodeNameResolution, "\"поручить\" поддерживает только \"асинхронная функция\" - %q не объявлена асинхронной", e.Call.Callee)
	}
	launch.Coroutine = true
	return launch, nil
}

func refName(ref *ir.FuncRefExpr, ok bool) string {
	if !ok {
		return ""
	}
	return ref.FuncName
}

// analyzeAwaitExpr - "дождаться <выражение>": Task обязан быть значением
// задачи конкретной сигнатуры (не абстрактной "Задача") - результат
// дождаться - результат этой сигнатуры.
func analyzeAwaitExpr(sc *ctx, e *ast.AwaitExpr) (*ir.AwaitExpr, error) {
	task, err := analyzeAwaitTask(sc, e)
	if err != nil {
		return nil, err
	}
	t := ir.TypeOf(task)
	if t.Kind != ir.KindTask {
		return nil, errAtLine(e.Line, CodeSemanticOther, "\"дождаться\" ожидает значение задачи (результат \"запустить\"/\"поручить\" или вызов \"асинхронная функция\"), получено %s", t)
	}
	if t.Entity == "" {
		return nil, errAtLine(e.Line, CodeSemanticOther, "\"дождаться\" ожидает конкретное значение задачи, получен абстрактный тип \"Задача\" - дождаться можно только задачу с известной сигнатурой (\"Задача<Вызываемый(...): Тип>\"); значение-список или nullable-\"?Задача\" нужно развернуть до конкретной задачи до ожидания")
	}
	sig, err := sc.a.resolveCallableSig(t.Entity)
	if err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: e.Line})
	}
	await := &ir.AwaitExpr{Task: task}
	if sig.Result != nil {
		await.Type = *sig.Result
		await.HasReturn = true
	}
	return await, nil
}

// analyzeAwaitTask разбирает операнд "дождаться". Обычный случай -
// произвольное выражение задачи (переменная/поле). Особый случай -
// ПРЯМОЙ вызов функции, объявленной "асинхронная": "дождаться
// получить_данные()" - сахар, запуск на планировщике корутин и сразу
// ожидание (тот же путь резолюции, что у "запустить", с Coroutine ==
// true). Явный "запустить получить_данные()" остаётся потоком ОС.
func analyzeAwaitTask(sc *ctx, e *ast.AwaitExpr) (ir.Expr, error) {
	if call, ok := e.Task.(*ast.CallExpr); ok {
		if _, isVar := sc.lookup(call.Callee); !isVar {
			if sig, exists := sc.funcs[call.Callee]; exists && sig.IsAsync {
				launch, err := analyzeLaunchExpr(sc, &ast.LaunchExpr{Call: call, Line: e.Line})
				if err != nil {
					return nil, err
				}
				launch.Coroutine = true
				return launch, nil
			}
		}
	}
	return analyzeExpr(sc, e.Task)
}
