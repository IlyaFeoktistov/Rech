package interp

import (
	"slices"

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
}

// Error - текст, который увидит вызывающий код (rech run/rech test), если
// исключение дошло до самого верха необработанным - см.
// rtmsg.UncaughtExceptionFormat (тот же формат, что и у cbackend.rech_throw).
func (e *thrownException) Error() string {
	return rtmsg.PanicPrefix + rtmsg.UncaughtExceptionFormat(e.value.Obj.typeName, e.value.Obj.fields["сообщение"].S)
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
