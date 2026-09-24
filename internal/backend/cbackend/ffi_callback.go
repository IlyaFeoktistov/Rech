// FFI-маршалинг колбэков ("FFI: маршалинг колбэков") -
// значение типа "Вызываемый(...)", переданное аргументом внешней функции
// или полем внешней структуры, там, где реальный C API ожидает голый
// указатель на функцию (без слота под окружение замыкания вовсе). Модель -
// пул из ir.FFICallbackPoolSize (см. internal/ir/decl.go, общая константа
// с internal/interp) одновременно живых слотов {fn, env} НА КАЖДУЮ
// отдельную "Вызываемый(...)"-сигнатуру, встретившуюся в такой позиции
// где-либо в программе - каждая новая регистрация занимает СЛЕДУЮЩИЙ
// свободный слот (счётчик, не переиспользуется - у Rech нет понятия
// "это замыкание больше не нужно" для FFI-колбэков), пока пул не
// исчерпан (тогда явная паника). Раньше был единственный слот на
// сигнатуру ("один активный колбэк", как signal()/atexit()) - заменён
// пулом по прямому запросу поддержать реалистичный случай нескольких
// ОДНОВРЕМЕННО живых замыканий одной сигнатуры (несколько оконных КЛАССОВ
// со своим WNDPROC каждый, WNDCLASS-паттерн) - без userdata-слота в
// самом C API единственный способ различить их - РАЗНЫЕ АДРЕСА функций,
// поэтому под каждый слот пула эмитится СВОЙ статический C-трамплин
// (голая функция без параметра-окружения), а нужный адрес выбирается ВО
// ВРЕМЯ ВЫПОЛНЕНИЯ индексацией по массиву указателей на них
// (rech_cbtrampptrs_<sig>[idx]) - количество трамплинов фиксировано на
// этапе кодогенерации (сам механизм голого указателя на функцию не
// позволяет породить новый адрес в рантайме без JIT, которого у cbackend
// нет и не планируется).
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

func mangleCallbackSlotFn(sigName string) string  { return "rech_cbfn_" + transliterate(sigName) }
func mangleCallbackSlotEnv(sigName string) string { return "rech_cbenv_" + transliterate(sigName) }
func mangleCallbackUsedVar(sigName string) string { return "rech_cbused_" + transliterate(sigName) }
func mangleCallbackTrampolinePtrs(sigName string) string {
	return "rech_cbtrampptrs_" + transliterate(sigName)
}
func mangleCallbackTrampolineFn(sigName string, i int) string {
	return fmt.Sprintf("rech_cbtramp_%s_%d", transliterate(sigName), i)
}

// ensureCallbackPool эмитит (один раз на sigName, ctx.
// callbackTrampolinesEmitted - тот же принцип дедупликации, что
// funcRefTrampolines/lambdaFuncsEmitted, callable.go) массивы слотов
// {fn, env}, счётчик занятых слотов и ir.FFICallbackPoolSize статических
// C-трамплинов (голых функций С РЕАЛЬНОЙ внешней сигнатурой, без скрытого
// env-параметра - i-й трамплин жёстко читает i-й слот) этой сигнатуры, а
// также массив указателей на них - именно из него marshalCallbackArg ниже
// берёт нужный адрес по индексу занятого слота.
func ensureCallbackPool(ctx *irGenCtx, sigName string) error {
	if ctx.callbackTrampolinesEmitted == nil {
		ctx.callbackTrampolinesEmitted = map[string]bool{}
	}
	if ctx.callbackTrampolinesEmitted[sigName] {
		return nil
	}
	sig, ok := ctx.callableSigs[sigName]
	if !ok {
		return fmt.Errorf("генерация C из IR: сигнатура колбэка %q не найдена среди \"Вызываемый\" программы", sigName)
	}
	ctx.callbackTrampolinesEmitted[sigName] = true

	fnSlot := mangleCallbackSlotFn(sigName)
	envSlot := mangleCallbackSlotEnv(sigName)
	usedVar := mangleCallbackUsedVar(sigName)
	trampPtrs := mangleCallbackTrampolinePtrs(sigName)
	resultC := callableResultCType(ctx, sig.Result)

	fnPtrParams := make([]string, 0, len(sig.Params)+1)
	fnPtrParams = append(fnPtrParams, "void *")
	paramDecls := make([]string, len(sig.Params))
	argNames := make([]string, len(sig.Params))
	paramTypes := make([]string, len(sig.Params))
	for i, p := range sig.Params {
		cT := fromIRType(p).cType(ctx)
		fnPtrParams = append(fnPtrParams, cT)
		paramTypes[i] = cT
		argNames[i] = fmt.Sprintf("rech_cbarg%d", i)
		paramDecls[i] = fmt.Sprintf("%s %s", cT, argNames[i])
	}
	paramList := "void"
	if len(paramDecls) > 0 {
		paramList = strings.Join(paramDecls, ", ")
	}
	bareParamList := "void"
	if len(paramTypes) > 0 {
		bareParamList = strings.Join(paramTypes, ", ")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "static %s (*%s[%d])(%s) = {0};\n", resultC, fnSlot, ir.FFICallbackPoolSize, strings.Join(fnPtrParams, ", "))
	fmt.Fprintf(&sb, "static void *%s[%d] = {0};\n", envSlot, ir.FFICallbackPoolSize)
	fmt.Fprintf(&sb, "static int %s = 0;\n", usedVar)

	trampNames := make([]string, ir.FFICallbackPoolSize)
	for i := range ir.FFICallbackPoolSize {
		name := mangleCallbackTrampolineFn(sigName, i)
		trampNames[i] = name
		call := fmt.Sprintf("%s[%d](%s)", fnSlot, i, strings.Join(append([]string{fmt.Sprintf("%s[%d]", envSlot, i)}, argNames...), ", "))
		fmt.Fprintf(&sb, "static %s %s(%s) {\n", resultC, name, paramList)
		if sig.Result != nil {
			fmt.Fprintf(&sb, "    return %s;\n", call)
		} else {
			fmt.Fprintf(&sb, "    %s;\n", call)
		}
		sb.WriteString("}\n")
	}
	fmt.Fprintf(&sb, "static %s (*%s[%d])(%s) = {\n", resultC, trampPtrs, ir.FFICallbackPoolSize, bareParamList)
	for _, name := range trampNames {
		fmt.Fprintf(&sb, "    %s,\n", name)
	}
	sb.WriteString("};\n\n")

	ctx.callableCode.WriteString(sb.String())
	return nil
}

// ffiCallableCParts - Result C-тип + список C-типов параметров (уже готовая
// "%s, %s, ..."-строка, "void" если параметров нет) сигнатуры колбэка
// sigName - общая часть ffiParamCType (анонимный тип для прототипа/
// аргумента) и ffiFieldDeclarator (именованный декларатор для поля
// структуры) ниже.
func ffiCallableCParts(ctx *irGenCtx, sigName string) (resultC, paramList string, err error) {
	sig, ok := ctx.callableSigs[sigName]
	if !ok {
		return "", "", fmt.Errorf("генерация C из IR: сигнатура колбэка %q не найдена среди \"Вызываемый\" программы", sigName)
	}
	resultC = callableResultCType(ctx, sig.Result)
	params := make([]string, len(sig.Params))
	for i, p := range sig.Params {
		params[i] = fromIRType(p).cType(ctx)
	}
	paramList = "void"
	if len(params) > 0 {
		paramList = strings.Join(params, ", ")
	}
	return resultC, paramList, nil
}

// ffiParamCType - C-тип параметра на границе FFI: как fromIRType(t).
// cType(ctx) для всего, КРОМЕ "Вызываемый(...)" - там ожидаемый реальным C
// API тип - голый АНОНИМНЫЙ указатель на функцию ("Результат (*)
// (Параметры...)", без имени - годится для прототипа/аргумента вызова, но
// НЕ для декларации поля/переменной, см. ffiFieldDeclarator), а не {fn,
// env} по значению (см. package doc выше) - используется
// externPrototypeOfIR (ir_generator.go) для самого прототипа внешней
// функции, отдельно от marshalCallbackArg (который меняет уже
// СГЕНЕРИРОВАННЫЙ КОД аргумента, не его C-тип в прототипе).
func ffiParamCType(ctx *irGenCtx, t ir.Type) (string, error) {
	if t.Kind != ir.KindCallable {
		return fromIRType(t).cType(ctx), nil
	}
	resultC, paramList, err := ffiCallableCParts(ctx, t.Entity)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s (*)(%s)", resultC, paramList), nil
}

// ffiFieldDeclarator - t (KindCallable) как ИМЕНОВАННЫЙ C-декларатор поля
// "внешняя структура" - "Результат (*fieldName)(Параметры...)", имя ВНУТРИ
// скобок после "*" (так C синтаксически требует для указателя на функцию -
// в отличие от ffiParamCType, анонимного типа для прототипа, где имя не
// нужно вовсе). См. generateIREntityStruct (ir_generator.go),
// codegen.StructDecl.FieldDeclarator.
func ffiFieldDeclarator(ctx *irGenCtx, t ir.Type, fieldName string) (string, error) {
	sigName, _, _ := ctx.ffiFuncPointerSig(t)
	resultC, paramList, err := ffiCallableCParts(ctx, sigName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s (*%s)(%s)", resultC, fieldName, paramList), nil
}

// marshalCallbackArg - code уже сгенерированный C-код значения типа
// "Вызываемый(...)" (argType) на границе FFI (аргумент вызова внешней
// функции, значение поля внешней структуры) - вместо {fn, env} по значению
// занимает следующий свободный слот пула сигнатуры (ctx.chainTmpPreamble -
// вставляется ПЕРЕД текущей инструкцией, тем же приёмом, что и
// materializeIfNeeded, ir_generator.go), паникует явно, если пул уже
// исчерпан, и возвращает выражение "массив_трамплинов[индекс]" (адрес
// СВОЕГО, ни с кем не разделяемого трамплина) - именно оно и должно
// попасть в итоговый C-вызов/инициализатор поля.
func (ctx *irGenCtx) marshalCallbackArg(argType ir.Type, code string) (string, error) {
	sigName, nullable, ok := ctx.ffiFuncPointerSig(argType)
	if !ok {
		return code, nil
	}
	if err := ensureToCallback(ctx, sigName); err != nil {
		return "", err
	}
	if nullable {
		if err := ensureToCallbackOpt(ctx, sigName, argType.Entity); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s(%s)", mangleToCallbackOpt(sigName), code), nil
	}
	return fmt.Sprintf("%s(%s)", mangleToCallback(sigName), code), nil
}

func mangleToCallback(sigName string) string    { return "rech_tocb_" + transliterate(sigName) }
func mangleToCallbackOpt(sigName string) string { return "rech_tocb_opt_" + transliterate(sigName) }

// ensureToCallback эмитит rech_tocb_... - Вызываемый -> голый указатель
// на функцию для C: значение, само пришедшее от C (fn - thunk сырого
// указателя, ensureRawThunk), уходит тем же сырым указателем, остальные
// занимают слот пула и получают его трамплин.
func ensureToCallback(ctx *irGenCtx, sigName string) error {
	if ctx.toCallbackEmitted == nil {
		ctx.toCallbackEmitted = map[string]bool{}
	}
	if ctx.toCallbackEmitted[sigName] {
		return nil
	}
	if err := ensureCallbackPool(ctx, sigName); err != nil {
		return err
	}
	if err := ensureRawThunk(ctx, sigName); err != nil {
		return err
	}
	ctx.toCallbackEmitted[sigName] = true
	ctx.usesPanic = true
	rawFn := mangleRawFnType(sigName)
	usedVar := mangleCallbackUsedVar(sigName)
	poolExhaustedMsg := fmt.Sprintf(rtmsg.FFICallbackPoolExhaustedTemplate, sigName, ir.FFICallbackPoolSize)
	var sb strings.Builder
	fmt.Fprintf(&sb, "static %s %s(%s c) {\n", rawFn, mangleToCallback(sigName), mangleCallableType(sigName))
	fmt.Fprintf(&sb, "    if (c.fn == %s) {\n        return (%s)c.env;\n    }\n", mangleRawThunk(sigName), rawFn)
	fmt.Fprintf(&sb, "    int idx = %s++;\n", usedVar)
	fmt.Fprintf(&sb, "    if (idx >= %d) {\n        rech_panic(\"%s\");\n    }\n", ir.FFICallbackPoolSize, escapeCString(poolExhaustedMsg))
	fmt.Fprintf(&sb, "    %s[idx] = c.fn;\n", mangleCallbackSlotFn(sigName))
	fmt.Fprintf(&sb, "    %s[idx] = c.env;\n", mangleCallbackSlotEnv(sigName))
	fmt.Fprintf(&sb, "    return %s[idx];\n}\n\n", mangleCallbackTrampolinePtrs(sigName))
	ctx.callableCode.WriteString(sb.String())
	return nil
}

// ensureToCallbackOpt - rech_tocb_opt_...: "?Вызываемый" -> указатель, Ничто
// - NULL.
func ensureToCallbackOpt(ctx *irGenCtx, sigName, nullableName string) error {
	if ctx.toCallbackOptEmitted == nil {
		ctx.toCallbackOptEmitted = map[string]bool{}
	}
	if ctx.toCallbackOptEmitted[sigName] {
		return nil
	}
	ctx.toCallbackOptEmitted[sigName] = true
	var sb strings.Builder
	fmt.Fprintf(&sb, "static %s %s(%s v) {\n", mangleRawFnType(sigName), mangleToCallbackOpt(sigName), mangleType(nullableName))
	fmt.Fprintf(&sb, "    return v.has ? %s(v.value) : NULL;\n}\n\n", mangleToCallback(sigName))
	ctx.callableCode.WriteString(sb.String())
	return nil
}

// Обратное направление - сырой указатель на функцию, пришедший ОТ C
// (результат внешней функции, поле внешней структуры), становится
// значением Вызываемый: fn - thunk сигнатуры (rech_rawcb_...), который
// зовёт сырой указатель, лежащий в env. Такое значение, переданное обратно
// в C (marshalCallbackArg), уходит тем же сырым указателем, без слота
// пула - поэтому и особые значения вроде SIG_DFL/SIG_IGN возвращаются в C
// без искажений.

func mangleRawFnType(sigName string) string { return "rech_rawfn_" + transliterate(sigName) }
func mangleRawThunk(sigName string) string  { return "rech_rawcb_" + transliterate(sigName) }
func mangleFromRaw(sigName string) string   { return "rech_fromraw_" + transliterate(sigName) }
func mangleFromRawOpt(sigName string) string {
	return "rech_fromraw_opt_" + transliterate(sigName)
}

// ffiFuncPointerSig - сигнатура указателя на функцию типа t: "Вызываемый"
// (nullable == false) или "?Вызываемый" (nullable == true); ok == false -
// t не такой.
func (ctx *irGenCtx) ffiFuncPointerSig(t ir.Type) (sigName string, nullable, ok bool) {
	if t.Kind == ir.KindCallable {
		return t.Entity, false, true
	}
	if t.Kind == ir.KindNullable {
		if inner := ctx.nullableInner[t.Entity]; inner.Kind == ir.KindCallable {
			return inner.Entity, true, true
		}
	}
	return "", false, false
}

// ensureRawThunk эмитит (один раз на сигнатуру) thunk и rech_fromraw_...
// (NULL - паника с переданным текстом).
func ensureRawThunk(ctx *irGenCtx, sigName string) error {
	if ctx.rawThunksEmitted == nil {
		ctx.rawThunksEmitted = map[string]bool{}
	}
	if ctx.rawThunksEmitted[sigName] {
		return nil
	}
	sig, ok := ctx.callableSigs[sigName]
	if !ok {
		return fmt.Errorf("генерация C из IR: сигнатура %q не найдена среди \"Вызываемый\" программы", sigName)
	}
	ctx.rawThunksEmitted[sigName] = true
	ctx.usesPanic = true

	resultC := callableResultCType(ctx, sig.Result)
	params := []string{"void *rech_env"}
	args := make([]string, len(sig.Params))
	for i, p := range sig.Params {
		args[i] = fmt.Sprintf("rech_a%d", i)
		params = append(params, fmt.Sprintf("%s %s", fromIRType(p).cType(ctx), args[i]))
	}
	rawFn := mangleRawFnType(sigName)
	call := fmt.Sprintf("((%s)rech_env)(%s)", rawFn, strings.Join(args, ", "))
	var sb strings.Builder
	fmt.Fprintf(&sb, "static %s %s(%s) {\n", resultC, mangleRawThunk(sigName), strings.Join(params, ", "))
	if sig.Result != nil {
		fmt.Fprintf(&sb, "    return %s;\n", call)
	} else {
		fmt.Fprintf(&sb, "    %s;\n", call)
	}
	sb.WriteString("}\n")
	callableC := mangleCallableType(sigName)
	fmt.Fprintf(&sb, "static %s %s(%s raw, const char *null_msg) {\n", callableC, mangleFromRaw(sigName), rawFn)
	sb.WriteString("    if (raw == NULL) {\n        rech_panic(null_msg);\n    }\n")
	fmt.Fprintf(&sb, "    return (%s){.fn = %s, .env = (void *)raw};\n", callableC, mangleRawThunk(sigName))
	sb.WriteString("}\n\n")
	ctx.callableCode.WriteString(sb.String())
	return nil
}

// ensureRawThunkOpt - rech_fromraw_opt_...: NULL - "Ничто" (nullableName -
// каноническое имя "?Вызываемый(...)").
func ensureRawThunkOpt(ctx *irGenCtx, sigName, nullableName string) error {
	if err := ensureRawThunk(ctx, sigName); err != nil {
		return err
	}
	if ctx.rawThunksOptEmitted == nil {
		ctx.rawThunksOptEmitted = map[string]bool{}
	}
	if ctx.rawThunksOptEmitted[sigName] {
		return nil
	}
	ctx.rawThunksOptEmitted[sigName] = true
	n := mangleType(nullableName)
	var sb strings.Builder
	fmt.Fprintf(&sb, "static %s %s(%s raw) {\n", n, mangleFromRawOpt(sigName), mangleRawFnType(sigName))
	fmt.Fprintf(&sb, "    if (raw == NULL) {\n        return %s_empty();\n    }\n", n)
	fmt.Fprintf(&sb, "    return %s_wrap(%s(raw, \"\"));\n", n, mangleFromRaw(sigName))
	sb.WriteString("}\n\n")
	ctx.callableCode.WriteString(sb.String())
	return nil
}

// unmarshalFuncPointer - C-выражение значения типа t ("Вызываемый"/
// "?Вызываемый") из сырого указателя raw; nullMsg - текст паники на NULL у
// "Вызываемый" без "?".
func (ctx *irGenCtx) unmarshalFuncPointer(t ir.Type, raw, nullMsg string) (string, error) {
	sigName, nullable, ok := ctx.ffiFuncPointerSig(t)
	if !ok {
		return raw, nil
	}
	rawFn := mangleRawFnType(sigName)
	if nullable {
		if err := ensureRawThunkOpt(ctx, sigName, t.Entity); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s((%s)(%s))", mangleFromRawOpt(sigName), rawFn, raw), nil
	}
	if err := ensureRawThunk(ctx, sigName); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s((%s)(%s), \"%s\")", mangleFromRaw(sigName), rawFn, raw, escapeCString(nullMsg)), nil
}
