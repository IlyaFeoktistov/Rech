// Рантайм-представление typestate - "state_flags", скрытое поле в начале
// C-структуры сущности (см. package doc ir.Entity.HasStateFlags/
// generateIREntityStruct). Два потребителя:
//   - ir.StateCheckExpr - параметр, объявленный производным типом
//     состояния, чей переход established не смог доказать на этапе
//     компиляции (internal/semantic, checkCallArgs) - вместо ошибки
//     компиляции рантайм-проверка бросает исключение, если бит не
//     установлен. Читает бит через offsetof(...), а не фиксированный
//     литерал 0/8 (позиция поля зависит от того, участвует ли конкретная
//     сущность в диспетчеризации - "vt" тоже претендует на самое начало
//     структуры, см. vtable.go) - вызывается через общий rech_require_state,
//     не привязанный к конкретному C-типу сущности.
//   - ir.HasStateExpr - "<выражение> относится к <ПроизводныйТипСостояния>"
//     (internal/semantic, analyzeIsExpr) - в отличие от StateCheckExpr,
//     никогда не бросает, просто возвращает Логическое. Здесь Entity уже
//     точно известен на этапе кодогенерации (не какой-то общий указатель
//     из разделяемой функции) - offsetof не нужен, читает поле через
//     прямой typed-каст к C-структуре Entity.
package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

const requireStateFunc = "rech_require_state"

// requireStateHelperCode - см. package doc. Возвращает obj без изменений
// при успехе (тот же приём, что и rech_struct_throw -
// свой маленький рантайм-хелпер вместо raw кода на каждом месте вызова),
// чтобы вызывающая сторона могла подставить сам вызов прямо на место
// исходного выражения (generateIRStateCheckExpr) - obj вычисляется РОВНО
// один раз, даже если исходное выражение не безобидный идентификатор
// (например, результат вызова функции).
func requireStateHelperCode() string {
	return fmt.Sprintf(`static void *%s(void *obj, size_t offset, uint64_t bit, const char *msg) {
    uint64_t flags = *(const uint64_t *)((const char *)obj + offset);
    if ((flags & bit) == 0) {
        rech_throw(%s(msg), "%s");
    }
    return obj;
}

`, requireStateFunc, mangleCtor(systemExceptionEntity), escapeCString(systemExceptionEntity))
}

// generateIRStateCheckExpr - см. package doc. Сообщение уже целиком
// известно на этапе компиляции (имя типа сущности + имя состояния, оба
// статические строки) - строится сразу, без snprintf времени исполнения.
func generateIRStateCheckExpr(ctx *irGenCtx, e *ir.StateCheckExpr) (string, valueType, error) {
	code, vt, err := generateIRExpr(ctx, e.Value)
	if err != nil {
		return "", typeUnknown, err
	}
	ent, ok := ctx.entities[e.Entity]
	if !ok {
		return "", typeUnknown, fmt.Errorf("генерация C из IR: сущность %q (typestate-проверка) не найдена", e.Entity)
	}
	bit, ok := ent.StateBit(e.State)
	if !ok {
		return "", typeUnknown, fmt.Errorf("генерация C из IR: у сущности %q нет состояния %q", e.Entity, e.State)
	}
	msg := rtmsg.StateNotEstablishedFormat(e.Entity, e.State)
	ctx.usesExceptions = true
	return fmt.Sprintf("(%s)%s((void *)(%s), offsetof(%s, state_flags), 1ULL << %d, \"%s\")",
		vt.cType(ctx), requireStateFunc, code, entityCName(ctx, e.Entity), bit, escapeCString(msg)), vt, nil
}

// generateIRHasStateExpr - см. package doc. Entity здесь - точная сущность,
// объявившая состояние (не какой-то общий указатель через разделяемую
// функцию, как у generateIRStateCheckExpr) - прямой typed-каст, offsetof не
// нужен.
func generateIRHasStateExpr(ctx *irGenCtx, e *ir.HasStateExpr) (string, valueType, error) {
	code, _, err := generateIRExpr(ctx, e.Value)
	if err != nil {
		return "", typeUnknown, err
	}
	ent, ok := ctx.entities[e.Entity]
	if !ok {
		return "", typeUnknown, fmt.Errorf("генерация C из IR: сущность %q (\"относится к\" состоянию) не найдена", e.Entity)
	}
	bit, ok := ent.StateBit(e.State)
	if !ok {
		return "", typeUnknown, fmt.Errorf("генерация C из IR: у сущности %q нет состояния %q", e.Entity, e.State)
	}
	return fmt.Sprintf("((((%s *)(%s))->state_flags & (1ULL << %d)) != 0)", entityCName(ctx, e.Entity), code, bit), typeBool, nil
}

const revokeStatesFunc = "rech_revoke_states"

// revokeStatesHelperCode - см. wrapRevokeStates. Возвращает obj без
// изменений (тот же приём, что и rech_require_state выше) - подставляется
// прямо на место получателя вызова метода-"противоположности".
func revokeStatesHelperCode() string {
	return fmt.Sprintf(`static void *%s(void *obj, size_t offset, uint64_t bits) {
    uint64_t *flags = (uint64_t *)((char *)obj + offset);
    *flags &= ~bits;
    return obj;
}

`, revokeStatesFunc)
}

// wrapRevokeStates оборачивает уже готовый, вычисленный РОВНО один раз
// C-код указателя на получателя (receiverPtr - см. generateIRMethodCallExpr,
// generateIRAddressableExpr) вызовом rech_revoke_states, подставляемым
// прямо на место получателя в аргументах вызова метода - снятие бита
// происходит ДО тела метода (сам вызов метода ещё не начал исполняться),
// поэтому "становится" внутри тела метода-"противоположности" (если есть)
// выполняется позже и потому побеждает - см. package doc
// ir.MethodCallExpr.RevokesStates.
func wrapRevokeStates(ctx *irGenCtx, entity string, states []string, receiverPtr string) (string, error) {
	ent, ok := ctx.entities[entity]
	if !ok {
		return "", fmt.Errorf("генерация C из IR: сущность %q (\"противоположность\") не найдена", entity)
	}
	var mask uint64
	for _, st := range states {
		bit, ok := ent.StateBit(st)
		if !ok {
			return "", fmt.Errorf("генерация C из IR: у сущности %q нет состояния %q", entity, st)
		}
		mask |= 1 << bit
	}
	return fmt.Sprintf("(%s *)%s((void *)(%s), offsetof(%s, state_flags), %dULL)",
		mangleType(entity), revokeStatesFunc, receiverPtr, mangleType(entity), mask), nil
}
