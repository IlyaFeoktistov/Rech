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
	// isPanic - брошенное значение - "Паника" или наследник ("выбросить"
	// своей паники): непойманное печатается как паника, без имени типа.
	isPanic bool
}

// newSystemException строит thrownException для системного исключения
// (typeName - "ОшибкаРазбора"/...), выброшенного
// напрямую рантаймом интерпретатора, не пользовательским "выбросить" -
// общий для json.go/xml.go/..., чтобы поля не
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
	msg := e.value.Obj.getField("сообщение").S
	if e.isPanic {
		return rtmsg.PanicPrefix + msg + traceSuffix(e.trace)
	}
	return rtmsg.PanicPrefix + rtmsg.UncaughtExceptionFormat(e.value.Obj.typeName, msg) + traceSuffix(e.trace)
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
	return ctrl{}, &thrownException{value: v, isPanic: in.isA(v.Obj.typeName, rtmsg.PanicEntity)}
}

// execTryStmt - "попытаться { ... } поймать(<имя>: <Тип>[|<Тип>...]) { ... }".
// Брошенное значение приходит двумя путями - "выбросить" (thrownException,
// обычная error) и паника рантайма (panicRuntime*, Go panic с
// *runtimeError, превращается в объект встроенной паники, panicValue) -
// дальше оба сопоставляются с CatchTypes одинаково (isA). Не подошло -
// пробрасывается дальше тем же путём, каким пришло.
func (in *Interp) execTryStmt(s *ir.TryStmt, env *Env, cc *callCtx) (ctrl, error) {
	if s.NoCatch || len(s.Finally) > 0 {
		return in.execTryWithFinally(s, env, cc)
	}
	return in.execTryCatch(s, env, cc)
}

// execTryCatch - тело + сопоставление брошенного с CatchTypes (без
// "окончательно"; execTryWithFinally оборачивает его снаружи).
func (in *Interp) execTryCatch(s *ir.TryStmt, env *Env, cc *callCtx) (c ctrl, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		re, ok := r.(*runtimeError)
		if !ok {
			panic(r)
		}
		v := in.panicValue(re)
		if !in.catchMatches(s, v) {
			panic(r)
		}
		c, err = in.runCatch(s, env, cc, v)
	}()
	c, err = in.execBlock(s.Body, newEnv(env), cc)
	if err == nil {
		return c, nil
	}
	thrown, ok := err.(*thrownException)
	if !ok || !in.catchMatches(s, thrown.value) {
		return ctrl{}, err
	}
	return in.runCatch(s, env, cc, thrown.value)
}

func (in *Interp) catchMatches(s *ir.TryStmt, v Value) bool {
	for _, t := range s.CatchTypes {
		if in.isA(v.Obj.typeName, t) {
			return true
		}
	}
	return false
}

// runCatch - CatchBody с CatchVar, привязанным к самому брошенному
// значению (не к копии заголовка - "этот" внутри полиморфного вызова
// метода на CatchVar должен видеть тот же объект) или, у смешанного
// списка обеих иерархий, к его "сообщение" (ir.TryStmt.CatchVarType).
func (in *Interp) runCatch(s *ir.TryStmt, env *Env, cc *callCtx, v Value) (ctrl, error) {
	catchEnv := newEnv(env)
	if s.CatchVarType.Kind == ir.KindString {
		catchEnv.declare(s.CatchVar, stringValue(v.Obj.getField("сообщение").S))
	} else {
		catchEnv.declare(s.CatchVar, in.cloneValue(v))
	}
	return in.execBlock(s.CatchBody, catchEnv, cc)
}

// panicValue - объект встроенной паники re.kind ("" - корень "Паника") с
// сообщением без служебного префикса, или встроенного исключения
// (re.exception).
func (in *Interp) panicValue(re *runtimeError) Value {
	if re.exception {
		return newSystemException(re.kind, re.text).value
	}
	kind := re.kind
	if kind == "" {
		kind = rtmsg.PanicEntity
	}
	return Value{
		Type: ir.EntityType(kind),
		Obj: &entityObj{typeName: kind, fields: map[string]Value{
			"сообщение": stringValue(strings.TrimPrefix(re.msg, rtmsg.PanicPrefix)),
		}},
	}
}

// execTryWithFinally - узел цепочки с "окончательно" (package doc
// ast.TryStmt.Finally) - либо вовсе без "поймать" (s.NoCatch), либо
// обычный "поймать" с довеском "окончательно". Диспетчеризация тела/поймать
// та же, что без "окончательно" (execTryCatch) - единственная разница,
// defer ниже, запускающий Finally, оборачивает её СНАРУЖИ: Go defer
// срабатывает на ЛЮБОМ выходе из этой функции - обычный возврат, возврат
// ошибки и ПРОБРОС паники дальше без recover здесь - "окончательно" в
// результате исполняется РОВНО ОДИН РАЗ, что бы ни случилось внутри.
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
		if fc.kind != ctrlNone {
			// Любой control-flow внутри самого "окончательно" (не только
			// "вернуть", см. package doc выше - "прервать"/"продолжить",
			// если попытаться{...} лежит внутри цикла) переопределяет
			// исход попытки/поймать тем же принципом.
			c = fc
			err = nil
		}
	}()

	if s.NoCatch {
		return in.execBlock(s.Body, newEnv(env), cc)
	}
	return in.execTryCatch(s, env, cc)
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
