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
	resultC, paramList, err := ffiCallableCParts(ctx, t.Entity)
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
	if err := ensureCallbackPool(ctx, argType.Entity); err != nil {
		return "", err
	}
	callableC := mangleCallableType(argType.Entity)
	ctx.chainTmpCounter++
	tmp := fmt.Sprintf("rech_cbarg_tmp%d", ctx.chainTmpCounter)
	idx := fmt.Sprintf("rech_cbidx_tmp%d", ctx.chainTmpCounter)
	usedVar := mangleCallbackUsedVar(argType.Entity)
	fnSlot := mangleCallbackSlotFn(argType.Entity)
	envSlot := mangleCallbackSlotEnv(argType.Entity)
	poolExhaustedMsg := fmt.Sprintf(rtmsg.FFICallbackPoolExhaustedTemplate, argType.Entity, ir.FFICallbackPoolSize)
	ctx.chainTmpPreamble = append(ctx.chainTmpPreamble,
		fmt.Sprintf("%s%s %s = %s;\n", ctx.curIndent, callableC, tmp, code),
		fmt.Sprintf("%sint %s = %s++;\n", ctx.curIndent, idx, usedVar),
		fmt.Sprintf("%sif (%s >= %d) { rech_panic(\"%s\"); }\n", ctx.curIndent, idx, ir.FFICallbackPoolSize, escapeCString(poolExhaustedMsg)),
		fmt.Sprintf("%s%s[%s] = %s.fn;\n", ctx.curIndent, fnSlot, idx, tmp),
		fmt.Sprintf("%s%s[%s] = %s.env;\n", ctx.curIndent, envSlot, idx, tmp),
	)
	return fmt.Sprintf("%s[%s]", mangleCallbackTrampolinePtrs(argType.Entity), idx), nil
}
