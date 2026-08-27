package interp

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// callableValue - значение типа "Вызываемый" (&имя/лямбда): call принимает
// уже вычисленные аргументы (тем же порядком, что и Function.Params) и
// исполняет тело - ссылку на функцию (evalFuncRefExpr) и лямбду
// (evalLambdaExpr) роднит один и тот же интерфейс вызова, дальше
// evalCallValueExpr не различает их вообще.
type callableValue struct {
	call func(args []Value) (Value, error)
}

func callableTypeValue(t ir.Type, call func(args []Value) (Value, error)) Value {
	return Value{Type: t, Callable: &callableValue{call: call}}
}

// evalFuncRefExpr - "&имя": оборачивает уже существующую функцию программы
// в callableValue, которая просто зовёт callFunction - то же исполнение,
// что и у обычного вызова по имени (ir.CallExpr), но полученное как
// значение и вызываемое позже, возможно многократно.
func (in *Interp) evalFuncRefExpr(e *ir.FuncRefExpr) (Value, error) {
	fn, ok := in.funcs[e.FuncName]
	if !ok {
		return Value{}, fmt.Errorf("функция %q не найдена во время исполнения", e.FuncName)
	}
	return callableTypeValue(e.Type, func(args []Value) (Value, error) {
		return in.callFunction(fn, args)
	}), nil
}

// evalLambdaExpr вычисляет "функция(...) { ... }" - снимок текущих значений
// захваченных переменных (e.Captures, в порядке, определённом семантическим
// анализом) копируется в момент создания лямбды (cloneValue - то же
// правило копирования, что и у обычного присваивания/аргумента, см. package
// doc ir.CaptureVar), не в момент вызова: последующая мутация переменной
// снаружи (или повторный вызов той же лямбды после того, как внешняя
// переменная изменилась) не видна внутри тела - копия уже независима.
// Сам снимок (capturedEnv) - ОДИН на значение лямбды, не создаётся заново
// на каждый вызов: каждый вызов исполняется в собственном Env, дочернем от
// capturedEnv (Env.set идёт вверх по цепочке к месту объявления имени) -
// мутация захваченной переменной внутри тела копится МЕЖДУ вызовами ОДНОГО
// И ТОГО ЖЕ значения лямбды (типичный сценарий - счётчик, растущий от
// вызова к вызову), а не теряется к следующему вызову.
func (in *Interp) evalLambdaExpr(e *ir.LambdaExpr, env *Env) (Value, error) {
	capturedEnv := newEnv(nil)
	for _, c := range e.Captures {
		v, ok := env.get(c.Name)
		if !ok {
			return Value{}, fmt.Errorf("захваченная переменная %q не найдена во время исполнения", c.Name)
		}
		capturedEnv.declare(c.Name, in.cloneValue(v))
	}
	return callableTypeValue(e.Type, func(args []Value) (Value, error) {
		callEnv := newEnv(capturedEnv)
		for i, p := range e.Params {
			callEnv.declare(p.Name, in.cloneValue(args[i]))
		}
		// curLine - собственная ячейка тела лямбды, не связанная с cc
		// вызывающей стороны: паника внутри не заводит отдельный кадр стека
		// вызовов (см. package doc internal/interp/trace.go) - execStmt всё
		// равно обязан иметь куда писать текущую строку, не читает её здесь
		// никто.
		ctrl, err := in.execBlock(e.Body, callEnv, &callCtx{curLine: new(int)})
		if err != nil {
			return Value{}, err
		}
		if ctrl.kind == ctrlReturn {
			return ctrl.value, nil
		}
		return Value{}, nil
	}), nil
}

// evalCallValueExpr - "ф(<арг>, ...)", где ф - значение типа "Вызываемый"
// (переменная/поле, не имя объявленной функции) - в отличие от ir.CallExpr
// (диспетчеризация по статически известному имени через in.funcs), здесь
// сам вызов уже несёт свою реализацию (Value.Callable.call), evalExpr
// только вычисляет получатель и аргументы и передаёт их дальше.
func (in *Interp) evalCallValueExpr(e *ir.CallValueExpr, env *Env, cc *callCtx) (Value, error) {
	fv, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	if fv.Callable == nil {
		return Value{}, fmt.Errorf("значение типа %s не является вызываемым во время исполнения", fv.Type)
	}
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return Value{}, err
		}
		args[i] = v
	}
	return fv.Callable.call(args)
}
