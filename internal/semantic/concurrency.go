// Параллелизм ("запустить"/"дождаться") - см. docs/SPEC.md. Модель -
// потоки ОС (шаг 15, docs/ROADMAP.md): каждый "запустить" - настоящий
// поток, без собственного планировщика/event loop. Никакой flow-sensitive
// проверки гонок/захвата изменяемого состояния между потоками нет -
// открытый вопрос ROADMAP.md решён в пользу "оставить программисту", тем
// же принципом, что и обычные потоки ОС в других языках без встроенного
// borrow checker.
package semantic

import (
	"fmt"

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
		return nil, fmt.Errorf("строка %d: \"запустить\" не поддерживает встроенные функции - только обычные функции программы", e.Line)
	}
	sig, ok := sc.funcs[name]
	if !ok {
		return nil, fmt.Errorf("строка %d: функция %q не объявлена", e.Line, name)
	}
	if sig.IsExtern {
		return nil, fmt.Errorf("строка %d: \"запустить\" не поддерживает внешние функции (\"внешняя функция\") - нет гарантии потокобезопасности произвольного C-символа", e.Line)
	}
	args, err := checkCallArgs(sc, e.Call.Args, sig, e.Line, fmt.Sprintf("функция %q", name), fmt.Sprintf("вызова %q", name))
	if err != nil {
		return nil, err
	}
	return &ir.LaunchExpr{Callee: name, Args: args}, nil
}

// analyzeAwaitExpr - "дождаться <выражение>": Task обязан быть значением
// задачи (TypeOf.Kind == KindTask, т. е. результатом LaunchExpr, включая
// проведённое через переменную/поле). Callee уже несёт имя обёрнутой
// функции (LaunchExpr.Callee, ir.TaskType) - сигнатура берётся из sc.funcs
// заново, а не переносится откуда-то ещё, тем же принципом, что и везде в
// этом пакете (символьная таблица одна на всю программу).
func analyzeAwaitExpr(sc *ctx, e *ast.AwaitExpr) (*ir.AwaitExpr, error) {
	task, err := analyzeExpr(sc, e.Task)
	if err != nil {
		return nil, err
	}
	t := ir.TypeOf(task)
	if t.Kind != ir.KindTask {
		return nil, fmt.Errorf("строка %d: \"дождаться\" ожидает значение задачи (результат \"запустить\"), получено %s", e.Line, t)
	}
	sig := sc.funcs[t.Entity]
	return &ir.AwaitExpr{Task: task, Callee: t.Entity, Type: sig.ReturnType, HasReturn: sig.HasReturn}, nil
}
