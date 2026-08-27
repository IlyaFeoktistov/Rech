package interp

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// taskValue - состояние задачи, запущенной "запустить <имя>(...)" -
// исполняется в отдельной Go-горутине (модель - потоки ОС, шаг 15,
// docs/ROADMAP.md; горутина - естественное соответствие потоку ОС в этом
// backend'е, интерпретатору не нужен собственный планировщик). done
// закрывается по завершении - "дождаться" просто читает из него.
type taskValue struct {
	result Value
	// err - исключение (thrownException), не пойманное внутри запущенной
	// функции - "дождаться" возвращает его как обычную error, тем же
	// каналом, которым execTryStmt и так пробрасывает любую ошибку (см.
	// package doc exception.go) - без этого исключение из фоновой задачи
	// осталось бы незамеченным.
	err error
	// panicVal - паника Go (обычно *runtimeError, panicRuntime) внутри
	// горутины, перехваченная recover() здесь же, чтобы не уронить весь
	// процесс немедленно (паника одной горутины, не пойманная НИГДЕ,
	// завершает всю программу, а не только эту горутину) - "дождаться"
	// поднимает её заново (panic(t.panicVal)) в потоке, который дожидается,
	// сохраняя ровно то же поведение, что было бы у паники на месте, где
	// её не поймали - только отложенное до самого "дождаться".
	panicVal any
	done     chan struct{}
}

// evalLaunchExpr - "запустить <имя>(<арг>, ...)": аргументы вычисляются и
// клонируются (та же значение-копирующая семантика "структура", что и у
// обычного вызова) ДО запуска горутины - сама горутина не имеет доступа к
// env вызывающей стороны, только к своим собственным args/callCtx (новый,
// без получателя - "запустить" не поддерживает методы).
func (in *Interp) evalLaunchExpr(e *ir.LaunchExpr, env *Env, cc *callCtx) (Value, error) {
	fn := in.funcs[e.Callee]
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return Value{}, err
		}
		args[i] = in.cloneValue(v)
	}
	t := &taskValue{done: make(chan struct{})}
	// thread - под отладчиком (in.debugger != nil, "rech debug") "запустить"
	// заводит НОВЫЙ поток DAP (см. internal/interp/debugger.go) - новая
	// горутина Go ведёт собственный, независимый стек кадров/собственные
	// точки останова/шаги, а не продолжает cc.dbg вызывающей стороны (та
	// исполняется в другой горутине). nil в обычном Run/rech test -
	// callFunction/panicRuntime дальше с ним ничего не делают.
	var thread *dbgThread
	if in.debugger != nil {
		thread = in.debugger.newThread("запустить " + e.Callee)
	}
	go func() {
		defer close(t.done)
		if thread != nil {
			defer in.debugger.threadExited(thread)
		}
		defer func() {
			if r := recover(); r != nil {
				t.panicVal = r
			}
		}()
		v, err := in.callFunction(fn, args, thread)
		if err != nil {
			t.err = err
			return
		}
		t.result = v
	}()
	return Value{Type: ir.TaskType(e.Callee), Task: t}, nil
}

// evalAwaitExpr - "дождаться <выражение>" - см. package doc taskValue.
func (in *Interp) evalAwaitExpr(e *ir.AwaitExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Task, env, cc)
	if err != nil {
		return Value{}, err
	}
	t := v.Task
	<-t.done
	if t.panicVal != nil {
		panic(t.panicVal)
	}
	if t.err != nil {
		return Value{}, t.err
	}
	return t.result, nil
}
