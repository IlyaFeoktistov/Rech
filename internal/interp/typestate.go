package interp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// evalStateCheckExpr - рантайм-проверка typestate (ir.StateCheckExpr),
// вставленная семантическим анализом вместо ошибки компиляции там, где
// established не смог доказать переход значения в требуемое состояние (см.
// package doc internal/semantic, checkCallArgs). Значение прозрачно
// возвращается дальше при успехе - StateCheckExpr стоит на месте
// исходного выражения, вычисленного ровно один раз.
func (in *Interp) evalStateCheckExpr(e *ir.StateCheckExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	if !v.Obj.hasState(e.State) {
		return Value{}, newSystemException("Исключение", rtmsg.StateNotEstablishedFormat(e.Entity, e.State))
	}
	return v, nil
}

// evalHasStateExpr - "<выражение> относится к <ПроизводныйТипСостояния>"
// (ir.HasStateExpr) - в отличие от evalStateCheckExpr выше, никогда не
// бросает исключение, просто сообщает текущий (времени исполнения) статус
// состояния как Логическое.
func (in *Interp) evalHasStateExpr(e *ir.HasStateExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	return boolValue(v.Obj.hasState(e.State)), nil
}
