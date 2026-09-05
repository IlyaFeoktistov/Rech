package interp

import (
	"slices"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// thrownException - сигнал "выбросить": в отличие от panicRuntime
// (невосстановимая ошибка рантайма, panic/recover Go), это ОБЫЧНАЯ error,
// пробрасываемая вверх штатным каналом, которым execStmt/execBlock/
// callMethod и так уже пробрасывают любую ошибку - "попытаться" просто
// проверяет тип err (execTryStmt) вместо того, чтобы завершать исполнение;
// ничего в остальном коде не нужно менять, чтобы исключение размотало
// стек до ближайшего "попытаться" или до самого верха, если такого нет.
type thrownException struct {
	value Value
	// trace - стек вызовов (internal/interp/trace.go), заполняется
	// callFunction/callMethod по мере того, как исключение (обычная error,
	// не Go panic) проходит через них необработанным - см. package doc там
	// же. Пуст, если исключение поймано раньше, чем дошло до границы
	// какого-либо вызова (execTryStmt внутри той же активации) - в этом
	// случае .Error() у него никто не спрашивает.
	trace []traceFrame
}

// divByZeroExceptionEntity - см. internal/semantic/exception.go, тот же
// принцип общего идентификатора константы, что и parseErrorEntity (json.go).
const divByZeroExceptionEntity = "ОшибкаДеленияНаНоль"

// newSystemException строит thrownException для системного исключения
// (typeName - "ОшибкаРазбора"/"ОшибкаДеленияНаНоль"/...), выброшенного
// напрямую рантаймом интерпретатора, не пользовательским "выбросить" -
// общий для json.go/xml.go/eval.go (деление на ноль), чтобы поля не
// расходились: "код" всегда явно "Ничто" (zeroValue нужного nullable-типа),
// а не отсутствующий ключ map - getField на отсутствующем ключе тоже
// вернул бы нулевое Value, которое СЛУЧАЙНО читается как "Ничто", но
// полагаться на это совпадение опаснее, чем задать его явно.
func newSystemException(entityName, msg string) *thrownException {
	return &thrownException{value: Value{
		Type: ir.EntityType(entityName),
		Obj: &entityObj{typeName: entityName, fields: map[string]Value{
			"сообщение": stringValue(msg),
			"код":       zeroValue(ir.NullableType(ir.TypeInt)),
		}},
	}}
}

// Error - текст, который увидит вызывающий код (rech run/rech test), если
// исключение дошло до самого верха необработанным - см.
// rtmsg.UncaughtExceptionFormat (тот же формат, что и у cbackend.rech_throw).
func (e *thrownException) Error() string {
	return rtmsg.PanicPrefix + rtmsg.UncaughtExceptionFormat(e.value.Obj.typeName, e.value.Obj.getField("сообщение").S) + traceSuffix(e.trace)
}

// isA - "относится к": typeName == target, или один из предков typeName по
// цепочке BaseName == target, или typeName (либо любой предок) реализует
// интерфейс target напрямую ("реализует", Entity.Interfaces - собственные
// для каждого уровня, см. package doc ir.Entity.Interfaces). typeName == ""
// (значение сущностного/интерфейсного типа ещё не присвоено, Obj == nil
// -см. zeroValue) - ничему не соответствует.
func (in *Interp) isA(typeName, target string) bool {
	for typeName != "" {
		if typeName == target {
			return true
		}
		ent, ok := in.entities[typeName]
		if !ok {
			return false
		}
		if slices.Contains(ent.Interfaces, target) {
			return true
		}
		typeName = ent.BaseName
	}
	return false
}

// execThrowStmt - "выбросить <выражение>": вычисляет значение и оборачивает
// его в thrownException - execTryStmt/execBlock/callMethod дальше несут
// его как обычную error, ничего специального в остальном коде не нужно.
func (in *Interp) execThrowStmt(s *ir.ThrowStmt, env *Env, cc *callCtx) (ctrl, error) {
	v, err := in.evalExpr(s.Value, env, cc)
	if err != nil {
		return ctrl{}, err
	}
	return ctrl{}, &thrownException{value: v}
}

// execTryStmt - "попытаться { ... } поймать(<имя>: <Тип>[|<Тип>...]) { ... }".
// Тело исполняется как обычный блок; если оно вернуло thrownException,
// фактический тип сравнивается с каждым перечисленным в CatchTypes (isA) -
// первое совпадение запускает CatchBody с CatchVar, привязанным к самому
// значению (не к копии заголовка - "этот" внутри полиморфного вызова
// метода на CatchVar должен видеть тот же объект). Ни одно совпадение,
// как и любая ДРУГАЯ ошибка (runtimeError и т. п.) - err пробрасывается
// как есть дальше, до ближайшего охватывающего "попытаться" или до самого
// верха.
func (in *Interp) execTryStmt(s *ir.TryStmt, env *Env, cc *callCtx) (ctrl, error) {
	if s.NoCatch || len(s.Finally) > 0 {
		return in.execTryWithFinally(s, env, cc)
	}
	if s.CatchPanic {
		if len(s.CatchTypes) > 0 {
			return in.execTryCatchMixed(s, env, cc)
		}
		return in.execTryCatchPanic(s, env, cc)
	}
	return in.execTryCatchTypes(s, env, cc)
}

// execTryCatchTypes - обычный "поймать(Тип[|Тип...])" без "Паника" в
// списке - вынесено из execTryStmt в отдельную функцию, чтобы
// execTryWithFinally ниже могла переиспользовать её целиком (см. package
// doc там же), не копируя диспетчеризацию по CatchTypes ещё раз.
func (in *Interp) execTryCatchTypes(s *ir.TryStmt, env *Env, cc *callCtx) (ctrl, error) {
	c, err := in.execBlock(s.Body, newEnv(env), cc)
	if err == nil {
		return c, nil
	}
	thrown, ok := err.(*thrownException)
	if !ok {
		return ctrl{}, err
	}
	matched := false
	for _, t := range s.CatchTypes {
		if in.isA(thrown.value.Obj.typeName, t) {
			matched = true
			break
		}
	}
	if !matched {
		return ctrl{}, err
	}
	catchEnv := newEnv(env)
	catchEnv.declare(s.CatchVar, in.cloneValue(thrown.value))
	return in.execBlock(s.CatchBody, catchEnv, cc)
}

// execTryWithFinally - узел цепочки с "окончательно" (package doc
// ast.TryStmt.Finally) - либо вовсе без "поймать" (s.NoCatch), либо один
// из трёх обычных видов (CatchTypes/CatchPanic/оба сразу) с довеском
// "окончательно". Диспетчеризация тела/поймать НЕ ОТЛИЧАЕТСЯ ни строчкой
// от версии без "окончательно" (execTryCatchTypes/execTryCatchPanic/
// execTryCatchMixed переиспользуются целиком) - единственная разница, defer
// ниже, запускающий Finally, оборачивает их СНАРУЖИ: Go defer срабатывает
// на ЛЮБОМ выходе из этой функции - обычный возврат, возврат ошибки
// (thrownException или любая другая) и ПРОБРОС паники дальше без recover
// здесь (Go продолжает разматывать стек через уже сработавший defer точно
// так же, как через любой другой) - "окончательно" в результате исполняется
// РОВНО ОДИН РАЗ, что бы ни случилось внутри, без отдельного кода на
// каждый случай.
//
// "вернуть" внутри самого Finally ПЕРЕОПРЕДЕЛЯЕТ результат (та же семантика,
// что и в Java/C# - известный сюрприз языка, но наименее удивительный из
// возможных вариантов, раз оба независимо пришли к нему же).
func (in *Interp) execTryWithFinally(s *ir.TryStmt, env *Env, cc *callCtx) (c ctrl, err error) {
	defer func() {
		fc, ferr := in.execBlock(s.Finally, newEnv(env), cc)
		if ferr != nil {
			err = ferr
			return
		}
		if fc.kind == ctrlReturn {
			c = fc
			err = nil
		}
	}()

	if s.NoCatch {
		return in.execBlock(s.Body, newEnv(env), cc)
	}
	if s.CatchPanic {
		if len(s.CatchTypes) > 0 {
			return in.execTryCatchMixed(s, env, cc)
		}
		return in.execTryCatchPanic(s, env, cc)
	}
	return in.execTryCatchTypes(s, env, cc)
}

// execTryCatchPanic - "попытаться { ... } поймать (<CatchVar>: Паника) { ... }"
// (docs/ROADMAP.md, изоляция паники) - в отличие от execTryStmt выше,
// ловит НЕ "выбросить"-исключение (обычная error, thrownException), а
// panicRuntime (rtmsg.PanicPrefix + внутреннее сообщение - выход за
// границы Список/Стопка/Очередь, несовпадение "как"/рефлексии,
// "утверждать" и т. п.) - это Go panic/recover, не error-возврат (см.
// package doc panicRuntime/callFunction - каждая граница вызова функции
// уже перехватывает *runtimeError, дописывает свой кадр стека и
// перепаниковывает дальше тем же r); recover здесь работает по тому же
// принципу, просто НЕ перепаниковывает - конвертирует в обычный возврат.
// "внутренняя ошибка интерпретатора" (r, не приводимый к *runtimeError -
// настоящий баг interp, не паника Rech-программы) пробрасывается дальше
// без изменений - НЕ наше дело перехватывать её здесь, как и Run() не
// перехватывает её иначе на самом верху.
func (in *Interp) execTryCatchPanic(s *ir.TryStmt, env *Env, cc *callCtx) (c ctrl, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		re, ok := r.(*runtimeError)
		if !ok {
			panic(r)
		}
		msg := strings.TrimPrefix(re.msg, rtmsg.PanicPrefix)
		catchEnv := newEnv(env)
		catchEnv.declare(s.CatchVar, stringValue(msg))
		c, err = in.execBlock(s.CatchBody, catchEnv, cc)
	}()
	return in.execBlock(s.Body, newEnv(env), cc)
}

// execTryCatchMixed - "попытаться { ... } поймать (<CatchVar>: Паника|<Тип>[|<Тип>...]) { ... }"
// (docs/ROADMAP.md, "интерфейс Выбрасываемое" - реализовано без него, тем же
// поймать(...) с "|", см. package doc ir.TryStmt.CatchPanic) - ОДИН
// обработчик и на панику (execTryCatchPanic), и на перечисленные исключения
// (execTryStmt) сразу; CatchVar в обоих случаях несёт только "Строка"
// (сообщение) - у паники другого и не бывает, а раз она в списке, общий тип
// уже не шире строки (см. package doc ast.TryStmt.CatchPanic). Устроено как
// execTryCatchPanic (defer/recover вокруг тела), но recover-ветка НЕ
// единственный источник обработки - обычное thrownException, вернувшееся
// без паники, проверяется после него тем же isA-перебором, что и в
// execTryStmt.
func (in *Interp) execTryCatchMixed(s *ir.TryStmt, env *Env, cc *callCtx) (c ctrl, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		re, ok := r.(*runtimeError)
		if !ok {
			panic(r)
		}
		msg := strings.TrimPrefix(re.msg, rtmsg.PanicPrefix)
		catchEnv := newEnv(env)
		catchEnv.declare(s.CatchVar, stringValue(msg))
		c, err = in.execBlock(s.CatchBody, catchEnv, cc)
	}()
	bc, berr := in.execBlock(s.Body, newEnv(env), cc)
	if berr == nil {
		return bc, nil
	}
	thrown, ok := berr.(*thrownException)
	if !ok {
		return ctrl{}, berr
	}
	matched := false
	for _, t := range s.CatchTypes {
		if in.isA(thrown.value.Obj.typeName, t) {
			matched = true
			break
		}
	}
	if !matched {
		return ctrl{}, berr
	}
	catchEnv := newEnv(env)
	catchEnv.declare(s.CatchVar, stringValue(thrown.value.Obj.getField("сообщение").S))
	return in.execBlock(s.CatchBody, catchEnv, cc)
}

// evalIsExpr - "<выражение> относится к <Тип>" - см. isA.
func (in *Interp) evalIsExpr(e *ir.IsExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	if v.Obj == nil {
		return boolValue(false), nil
	}
	return boolValue(in.isA(v.Obj.typeName, e.Target)), nil
}
