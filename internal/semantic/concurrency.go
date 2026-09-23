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
	"regexp"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
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

// analyzeLaunchExpr - "запустить <имя>(<арг>, ...)": Callee обязан быть
// обычной, объявленной в программе функцией - не встроенной (нет гарантии
// потокобезопасности библиотечного C-кода) и не "внешняя функция" (та же
// причина - произвольный C-символ). Метод сюда попасть не может вовсе -
// парсер разбирает операнд "запустить" ровно как строгий вызов функции
// (IDENT "(" ... ")"), синтаксически той же формы, что и MethodCallExpr
// (".") достичь не может (см. ast.LaunchExpr).
func analyzeLaunchExpr(sc *ctx, e *ast.LaunchExpr) (ir.Expr, error) {
	name := e.Call.Callee
	if builtins.IsReservedName(name) {
		return nil, errAtLine(e.Line, CodeSemanticOther, "\"запустить\" не поддерживает встроенные функции - только обычные функции программы")
	}
	// callee - имя, которое реально попадёт в ir.LaunchExpr.Callee (и, через
	// ir.TaskType(Callee), в тип результата "запустить") - мангл-имя у
	// перегруженной функции, голое имя иначе (см. package doc
	// funcSig.MangledName, resolveFuncSigForCall). analyzeAwaitExpr находит
	// сигнатуру заново по этому же имени (sc.funcs[t.Entity]) - оба места
	// автоматически согласованы.
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
	return &ir.LaunchExpr{Callee: callee, Args: args}, nil
}

// analyzeCoroLaunchExpr - "поручить <асинхронная функция>(<арг>, ...)"
// (см. package doc ast.CoroLaunchExpr) - резолюция вызова та же самая, что
// у "запустить" (analyzeLaunchExpr, не копия её кода), ДОПОЛНИТЕЛЬНО
// требует funcSig.IsAsync - "поручить" (в отличие от "запустить")
// поддерживает ТОЛЬКО функции, объявленные "асинхронная" (фоновый запуск
// на планировщике корутин имеет смысл только для них - обычная функция
// исполнялась бы синхронно на СВОЁМ ЖЕ вызывающем контексте, никакого
// "фона" не было бы вовсе). Результат - тот же ir.LaunchExpr, что и у
// сахара "дождаться <асинхронная функция>()" (analyzeAwaitTask), с
// Coroutine == true - разница только в том, что "поручить" НЕ оборачивает
// результат в AwaitExpr сразу же (не блокирует), "дождаться" делает это
// потом, отдельно, над уже сохранённым в переменную значением.
func analyzeCoroLaunchExpr(sc *ctx, e *ast.CoroLaunchExpr) (ir.Expr, error) {
	launch, err := analyzeLaunchExpr(sc, &ast.LaunchExpr{Call: e.Call, Line: e.Line})
	if err != nil {
		return nil, err
	}
	l := launch.(*ir.LaunchExpr)
	if sig, ok := sc.funcs[l.Callee]; !ok || !sig.IsAsync {
		return nil, errAtLine(e.Line, CodeNameResolution, "\"поручить\" поддерживает только \"асинхронная функция\" - %q не объявлена асинхронной", e.Call.Callee)
	}
	l.Coroutine = true
	return l, nil
}

// analyzeAwaitExpr - "дождаться <выражение>": Task обязан быть значением
// задачи (TypeOf.Kind == KindTask/KindCoroTask, т. е. результатом
// LaunchExpr/CoroLaunchExpr, включая проведённое через переменную/поле,
// или прямым вызовом "асинхронная функция" - см. analyzeAwaitTask).
// Callee уже несёт имя обёрнутой функции (LaunchExpr.Callee, ir.TaskType/
// CoroTaskType) - сигнатура берётся из sc.funcs заново, а не переносится
// откуда-то ещё, тем же принципом, что и везде в этом пакете (символьная
// таблица одна на всю программу).
func analyzeAwaitExpr(sc *ctx, e *ast.AwaitExpr) (*ir.AwaitExpr, error) {
	task, err := analyzeAwaitTask(sc, e)
	if err != nil {
		return nil, err
	}
	t := ir.TypeOf(task)
	if t.Kind != ir.KindTask && t.Kind != ir.KindCoroTask {
		return nil, errAtLine(e.Line, CodeSemanticOther, "\"дождаться\" ожидает значение задачи (результат \"запустить\"/\"поручить\" или вызов \"асинхронная функция\"), получено %s", t)
	}
	if t.Entity == "" {
		return nil, errAtLine(e.Line, CodeSemanticOther, "\"дождаться\" ожидает конкретное значение задачи, получен абстрактный тип \"Задача\" - дождаться можно только результат \"запустить\"/\"поручить\" или вызов \"асинхронная функция\"; значение-список или nullable-\"?Задача\" нужно развернуть до конкретной задачи до ожидания")
	}
	sig, ok := sc.funcs[t.Entity]
	if !ok {
		return nil, errAtLine(e.Line, CodeNameResolution, "функция %q не объявлена", t.Entity)
	}
	return &ir.AwaitExpr{Task: task, Callee: t.Entity, Type: sig.ReturnType, HasReturn: sig.HasReturn}, nil
}

// analyzeAwaitTask разбирает операнд "дождаться". Обычный случай -
// произвольное выражение задачи (переменная/поле, несущие результат
// "запустить"), анализируется как есть - идёт на поток ОС из общего пула,
// как и раньше (ir.LaunchExpr.Coroutine остаётся false по умолчанию).
// Особый случай - Task синтаксически ПРЯМОЙ вызов функции (не через
// "запустить"), объявленной "асинхронная" (funcSig.IsAsync): "дождаться
// получить_данные()" тогда десугарится в LaunchExpr здесь же, через
// analyzeLaunchExpr (единый путь резолюции перегрузки/проверки аргументов
// с обычным "запустить", не копия его кода), но с Coroutine == true -
// backend (internal/backend/cbackend/coroutine.go, Шаги 128-130) эмитит
// вызов на лёгком планировщике вместо потока ОС. Явный "запустить
// получить_данные()" (не через эту функцию) по-прежнему даёт
// Coroutine == false - модель под ним не меняется никогда (docs/SPEC.md,
// раздел 15).
//
// НЕТ статической проверки на вложенный сахар (асинхронная функция,
// изнутри вызывающая "дождаться <другая асинхронная функция>()") - НЕ
// НУЖНА (Шаг 132/133 - вложенность ПОДДЕРЖАНА на уровне планировщика,
// cbackend/coroutine.go, rech_coro_step - каждый вложенный вызов получает
// собственную точку возврата). Раньше здесь была бы уместна заметка о
// том, что статическая проверка в принципе невозможна (реальное условие
// "сейчас исполняется код на корутине" динамическое, не от объявления
// "асинхронная" - см. package doc ast.FunctionDecl.IsAsync) - теперь
// не актуально вовсе, проверять нечего.
func analyzeAwaitTask(sc *ctx, e *ast.AwaitExpr) (ir.Expr, error) {
	if call, ok := e.Task.(*ast.CallExpr); ok {
		if sig, exists := sc.funcs[call.Callee]; exists && sig.IsAsync {
			launch, err := analyzeLaunchExpr(sc, &ast.LaunchExpr{Call: call, Line: e.Line})
			if err != nil {
				return nil, err
			}
			launch.(*ir.LaunchExpr).Coroutine = true
			return launch, nil
		}
	}
	return analyzeExpr(sc, e.Task)
}

// taskAnnotationRef - "Задача<T>" внутри произвольной строки type_name
// (голая "Задача<T>", "Список<Задача<T>>", "?Задача<T>", ...) - "T" внутри
// НИКОГДА не несёт собственных "<...>" (resolveGenericTypeName берёт его
// как голое имя функции, не парсит рекурсивно как тип), поэтому регулярка
// без вложенности достаточна и не путается с ЛЮБЫМ другим generic-типом.
var taskAnnotationRef = regexp.MustCompile(`Задача<([^<>,\s]+)>`)

// overloadedTaskAnnotationCallee ищет в typeName ПЕРВОЕ вхождение
// "Задача<T>", чей T - имя функции, перегруженной (2+ объявления) в этой
// программе (funcOverloads), "" если такого нет.
//
// Нужна, потому что "Задача<T>"/"Список<Задача<T>>" в АННОТАЦИИ типа несёт
// ГОЛОЕ имя T как есть (resolveGenericTypeName, case "Задача" - без
// разрешения перегрузки вовсе), а настоящий "запустить T(...)"/"поручить
// T(...)" (analyzeLaunchExpr) сам резолвит перегрузку по типам аргументов
// вызова и несёт в ir.LaunchExpr.Callee/ir.TaskType УЖЕ манглированное имя
// ("T(Целое,Целое)", package doc funcSig.MangledName) - для перегруженного
// T аннотация и настоящее значение никогда не совпадут (isAssignable
// сравнивает Entity как есть), а спеллинг манглированного имени как типа
// ("Задача<T(Целое,Целое)>") не входит в грамматику type_name вовсе
// (скобки внутри "<...>" не парсятся). Явная, ранняя ошибка здесь - вместо
// путающего "объявлена типом Задача<T>, но инициализирована значением типа
// Задача<T(Целое,Целое)>" ниже по течению (isAssignable).
func overloadedTaskAnnotationCallee(typeName string, funcOverloads map[string][]funcSig) string {
	for _, m := range taskAnnotationRef.FindAllStringSubmatch(typeName, -1) {
		if len(funcOverloads[m[1]]) > 0 {
			return m[1]
		}
	}
	return ""
}
