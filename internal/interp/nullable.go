package interp

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// evalNullLiteral - "Ничто" в контексте e.Type ("?Тип") - см. package doc
// Value.Nullable.
func evalNullLiteral(e *ir.NullLiteral) Value {
	return Value{Type: e.Type}
}

// evalNullWrapExpr - неявное заворачивание значения базового типа в
// "?Тип" (см. package doc ir.NullWrapExpr) - клонирует так же, как и любая
// другая точка "коммита" в новое хранилище (VarDecl/Assign/...), потому что
// значение теперь живёт по новому адресу (Value.Nullable), а не там же, где
// исходное выражение его вернуло.
func (in *Interp) evalNullWrapExpr(e *ir.NullWrapExpr, env *Env, cc *callCtx) (Value, error) {
	inner, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	wrapped := in.cloneValue(inner)
	return Value{Type: e.Type, Nullable: &wrapped}, nil
}

// evalNullUnwrapExpr - разворачивание "?Тип" в Тип (явный каст или доказанно
// непустое употребление, см. package doc ir.NullUnwrapExpr) - паникует, если
// значение на самом деле Ничто.
func (in *Interp) evalNullUnwrapExpr(e *ir.NullUnwrapExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	if v.Nullable == nil {
		panicRuntime("%s", rtmsg.NullAccess)
	}
	return *v.Nullable, nil
}

// evalNullCheckExpr - "== Ничто"/"!= Ничто" (IsNot=true у "!=").
func (in *Interp) evalNullCheckExpr(e *ir.NullCheckExpr, env *Env, cc *callCtx) (Value, error) {
	v, err := in.evalExpr(e.Value, env, cc)
	if err != nil {
		return Value{}, err
	}
	has := v.Nullable != nil
	if e.IsNot {
		return boolValue(has), nil
	}
	return boolValue(!has), nil
}

// evalNullCoalesceExpr - "а ?? б": значение а, если оно не Ничто (уже готово
// в правильной форме - развёрнутой или нет, смотря по e.Type, см. package
// doc ir.NullCoalesceExpr), иначе значение б (его собственный анализ
// (internal/semantic) уже гарантировал ровно ту форму, которую ожидает
// e.Type).
func (in *Interp) evalNullCoalesceExpr(e *ir.NullCoalesceExpr, env *Env, cc *callCtx) (Value, error) {
	left, err := in.evalExpr(e.Left, env, cc)
	if err != nil {
		return Value{}, err
	}
	if left.Nullable != nil {
		if e.Type.Kind == ir.KindNullable {
			return left, nil
		}
		return *left.Nullable, nil
	}
	return in.evalExpr(e.Right, env, cc)
}
