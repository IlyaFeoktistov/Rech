// FFI-маршалинг колбэков ("FFI: маршалинг колбэков") -
// значение типа "Вызываемый(...)", переданное аргументом внешней функции
// или полем внешней структуры, там, где реальный C API ожидает голый
// указатель на функцию (без слота под окружение замыкания вовсе). Модель -
// ОДИН активный колбэк на СИГНАТУРУ, не на место вызова/присваивания: один
// глобальный слот {fn, env} на каждую отдельную "Вызываемый(...)"
// сигнатуру, встретившуюся в такой позиции где-либо в программе -
// регистрация нового замыкания в него ЗАМЕНЯЕТ предыдущее. Согласовано с
// пользователем как достаточная модель - ровно то же поведение, что у
// signal()/atexit() в реальном C API (один активный обработчик в любой
// момент); НЕ поддерживает несколько одновременно живых замыканий ОДНОЙ
// сигнатуры (несколько окон с разными WNDPROC и т. п.) - для этого
// понадобился бы userdata-слот у самого C API (тогда см. случай 1,
// "Вызываемый" уже устроен ровно как fn(void *userdata, ...) без единой
// строчки нового кода) или отдельный, более сложный механизм.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func mangleCallbackSlotFn(sigName string) string     { return "rech_cbfn_" + transliterate(sigName) }
func mangleCallbackSlotEnv(sigName string) string    { return "rech_cbenv_" + transliterate(sigName) }
func mangleCallbackTrampoline(sigName string) string { return "rech_cbtramp_" + transliterate(sigName) }

// ensureCallbackTrampoline эмитит (один раз на sigName, ctx.
// callbackTrampolinesEmitted - тот же принцип дедупликации, что
// funcRefTrampolines/lambdaFuncsEmitted, callable.go) глобальный слот
// {fn, env} этой сигнатуры и голую C-функцию (trampoline) с ЕЁ РЕАЛЬНОЙ
// внешней сигнатурой (без скрытого env-параметра) - она читает слот и
// зовёт fn(env, ...аргументы). Возвращает C-имя trampoline - именно оно
// передаётся туда, где C ожидает указатель на функцию (marshalCallbackArg
// ниже).
func ensureCallbackTrampoline(ctx *irGenCtx, sigName string) (string, error) {
	name := mangleCallbackTrampoline(sigName)
	if ctx.callbackTrampolinesEmitted == nil {
		ctx.callbackTrampolinesEmitted = map[string]bool{}
	}
	if ctx.callbackTrampolinesEmitted[sigName] {
		return name, nil
	}
	sig, ok := ctx.callableSigs[sigName]
	if !ok {
		return "", fmt.Errorf("генерация C из IR: сигнатура колбэка %q не найдена среди \"Вызываемый\" программы", sigName)
	}
	ctx.callbackTrampolinesEmitted[sigName] = true

	fnSlot := mangleCallbackSlotFn(sigName)
	envSlot := mangleCallbackSlotEnv(sigName)
	resultC := callableResultCType(ctx, sig.Result)

	fnPtrParams := make([]string, 0, len(sig.Params)+1)
	fnPtrParams = append(fnPtrParams, "void *")
	paramDecls := make([]string, len(sig.Params))
	argNames := make([]string, len(sig.Params))
	for i, p := range sig.Params {
		cT := fromIRType(p).cType(ctx)
		fnPtrParams = append(fnPtrParams, cT)
		argNames[i] = fmt.Sprintf("rech_cbarg%d", i)
		paramDecls[i] = fmt.Sprintf("%s %s", cT, argNames[i])
	}
	paramList := "void"
	if len(paramDecls) > 0 {
		paramList = strings.Join(paramDecls, ", ")
	}
	call := fmt.Sprintf("%s(%s)", fnSlot, strings.Join(append([]string{envSlot}, argNames...), ", "))

	var sb strings.Builder
	fmt.Fprintf(&sb, "static %s (*%s)(%s) = NULL;\n", resultC, fnSlot, strings.Join(fnPtrParams, ", "))
	fmt.Fprintf(&sb, "static void *%s = NULL;\n", envSlot)
	fmt.Fprintf(&sb, "static %s %s(%s) {\n", resultC, name, paramList)
	if sig.Result != nil {
		fmt.Fprintf(&sb, "    return %s;\n", call)
	} else {
		fmt.Fprintf(&sb, "    %s;\n", call)
	}
	sb.WriteString("}\n\n")
	ctx.callableCode.WriteString(sb.String())
	return name, nil
}

// ffiParamCType - C-тип параметра/поля на границе FFI: как fromIRType(t).
// cType(ctx) для всего, КРОМЕ "Вызываемый(...)" - там ожидаемый реальным C
// API тип - голый указатель на функцию ("Результат (*)(Параметры...)"), а
// не {fn, env} по значению (см. package doc выше) - используется
// externPrototypeOfIR (ir_generator.go) для самого прототипа внешней
// функции, отдельно от marshalCallbackArg (который меняет уже
// СГЕНЕРИРОВАННЫЙ КОД аргумента, не его C-тип в прототипе).
func ffiParamCType(ctx *irGenCtx, t ir.Type) (string, error) {
	if t.Kind != ir.KindCallable {
		return fromIRType(t).cType(ctx), nil
	}
	sig, ok := ctx.callableSigs[t.Entity]
	if !ok {
		return "", fmt.Errorf("генерация C из IR: сигнатура колбэка %q не найдена среди \"Вызываемый\" программы", t.Entity)
	}
	resultC := callableResultCType(ctx, sig.Result)
	params := make([]string, len(sig.Params))
	for i, p := range sig.Params {
		params[i] = fromIRType(p).cType(ctx)
	}
	paramList := "void"
	if len(params) > 0 {
		paramList = strings.Join(params, ", ")
	}
	return fmt.Sprintf("%s (*)(%s)", resultC, paramList), nil
}

// marshalCallbackArg - code уже сгенерированный C-код значения типа
// "Вызываемый(...)" (argType) на границе FFI (аргумент вызова внешней
// функции, значение поля внешней структуры) - вместо {fn, env} по значению
// регистрирует его в глобальный слот сигнатуры (ctx.chainTmpPreamble -
// вставляется ПЕРЕД текущей инструкцией, тем же приёмом, что и
// materializeIfNeeded, ir_generator.go) и возвращает имя trampoline (голый
// адрес C-функции) вместо кода {fn, env} - именно это имя и должно попасть
// в итоговый C-вызов/инициализатор поля.
func (ctx *irGenCtx) marshalCallbackArg(argType ir.Type, code string) (string, error) {
	trampoline, err := ensureCallbackTrampoline(ctx, argType.Entity)
	if err != nil {
		return "", err
	}
	callableC := mangleCallableType(argType.Entity)
	ctx.chainTmpCounter++
	tmp := fmt.Sprintf("rech_cbarg_tmp%d", ctx.chainTmpCounter)
	ctx.chainTmpPreamble = append(ctx.chainTmpPreamble,
		fmt.Sprintf("%s%s %s = %s;\n", ctx.curIndent, callableC, tmp, code),
		fmt.Sprintf("%s%s = %s.fn;\n", ctx.curIndent, mangleCallbackSlotFn(argType.Entity), tmp),
		fmt.Sprintf("%s%s = %s.env;\n", ctx.curIndent, mangleCallbackSlotEnv(argType.Entity), tmp),
	)
	return trampoline, nil
}
