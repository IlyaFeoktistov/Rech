// Значения типа "Вызываемый" (&имя/лямбда) - представлены двухсловной
// C-структурой {указатель на функцию, указатель на окружение замыкания}
// (см. callableStructTypedef) - типичная реализация замыкания через
// closure conversion: функция, принимающая окружение первым, скрытым
// параметром, и отдельно - само окружение как непрозрачный "void *".
//
// &имя (ссылка на уже объявленную функцию, ensureFuncRefTrampoline) не
// захватывает ничего - окружение всегда NULL, а функция-указатель ведёт на
// небольшую обёртку (trampoline), которая просто отбрасывает окружение и
// зовёт настоящую функцию - нужна, потому что обычная функция Rech не
// принимает лишний первый параметр "окружение", а любое значение типа
// "Вызываемый" обязано вызываться единым способом (см. generateIRCallValueExpr),
// независимо от того, ссылка это или лямбда.
//
// Лямбда (ensureLambdaFunc) захватывает переменные ПО ЗНАЧЕНИЮ в момент
// создания (см. package doc ir.CaptureVar) - окружение здесь настоящее:
// собственная C-структура с одним полем на каждую захваченную переменную,
// выделяется на куче Boehm GC (GC_MALLOC) и заполняется прямо в выражении,
// строящем значение лямбды (одно C-выражение через memcpy - см.
// generateIRLambdaExpr), без отдельной функции-инициализатора. Тело лямбды
// становится обычной static-функцией верхнего уровня (C не умеет вложенные
// функции) - тем же приёмом, что и точка входа потока у "запустить" (см.
// package doc internal/backend/cbackend/concurrency.go), но без catch-frame:
// лямбда исполняется синхронно в вызывающем потоке, исключение внутри нее
// поднимается через уже существующий rech_catch_top этого же потока, как у
// обычного вызова - для неё не нужна отдельная обвязка.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func mangleCallableType(sigName string) string       { return "rech_C_" + transliterate(sigName) }
func mangleCallableEnvType(lambdaName string) string { return "rech_ENV_" + transliterate(lambdaName) }
func mangleLambdaFunc(lambdaName string) string      { return "rech_" + transliterate(lambdaName) }
func mangleFuncRefTrampoline(funcName string) string {
	return "rech_fref_" + transliterate(funcName)
}

// callableResultCType - C-тип результата сигнатуры "Вызываемый" - "void"
// без результата, не "int" с заглушкой "return 0", как у обычных функций
// Rech (cReturnTypeFor) - у Callable нет такой заглушки нигде, ни в
// сигнатуре структуры, ни у trampoline/тела лямбды, поэтому все три обязаны
// договориться об одном и том же "void", а не унаследовать чужое соглашение.
func callableResultCType(ctx *irGenCtx, result *ir.Type) string {
	if result == nil {
		return "void"
	}
	return fromIRType(*result).cType(ctx)
}

// callableStructTypedef строит "typedef struct { Result (*fn)(void *, П1,
// П2, ...); void *env; } rech_C_...;" для сигнатуры sig - одна на каждую
// сигнатуру программы (prog.Callables, дедуплицировано internal/semantic),
// эмитится безусловно для всех них разом в GenerateFromIR, не лениво по
// первому использованию (в отличие от ensureFuncRefTrampoline/
// ensureLambdaFunc ниже - им нужно тело конкретной функции/лямбды, которое
// есть только в точке её кодогенерации).
func callableStructTypedef(ctx *irGenCtx, sig ir.CallableSig) string {
	parts := make([]string, 0, len(sig.Params)+1)
	parts = append(parts, "void *")
	for _, p := range sig.Params {
		parts = append(parts, fromIRType(p).cType(ctx))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "typedef struct {\n    %s (*fn)(%s);\n    void *env;\n} %s;\n\n",
		callableResultCType(ctx, sig.Result), strings.Join(parts, ", "), mangleCallableType(sig.Name))
	return sb.String()
}

// ensureFuncRefTrampoline эмитит (один раз на funcName, ctx.funcRefTrampolines -
// дедупликация, тот же принцип, что ctx.launchedFuncs у "запустить") C-функцию
// с сигнатурой "окружение первым параметром, дальше как у funcName",
// отбрасывающую окружение и зовущую funcName как есть - возвращает её C-имя.
func ensureFuncRefTrampoline(ctx *irGenCtx, funcName string) string {
	name := mangleFuncRefTrampoline(funcName)
	if ctx.funcRefTrampolines == nil {
		ctx.funcRefTrampolines = map[string]bool{}
	}
	if ctx.funcRefTrampolines[funcName] {
		return name
	}
	ctx.funcRefTrampolines[funcName] = true

	fn := ctx.irFuncs[funcName]
	var result *ir.Type
	if fn.HasReturn {
		result = &fn.ReturnType
	}
	argNames := make([]string, len(fn.Params))
	params := make([]string, 0, len(fn.Params)+1)
	params = append(params, "void *rech_env")
	for i, p := range fn.Params {
		argNames[i] = mangleVar(p.Name)
		params = append(params, fmt.Sprintf("%s %s", fromIRType(p.Type).cType(ctx), argNames[i]))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "static %s %s(%s) {\n", callableResultCType(ctx, result), name, strings.Join(params, ", "))
	sb.WriteString("    (void)rech_env;\n")
	call := fmt.Sprintf("%s(%s)", funcCName(ctx, funcName), strings.Join(argNames, ", "))
	if fn.HasReturn {
		fmt.Fprintf(&sb, "    return %s;\n", call)
	} else {
		fmt.Fprintf(&sb, "    %s;\n", call)
	}
	sb.WriteString("}\n\n")
	ctx.callableCode.WriteString(sb.String())
	return name
}

// generateIRFuncRefExpr - "&имя".
func generateIRFuncRefExpr(ctx *irGenCtx, e *ir.FuncRefExpr) (string, valueType, error) {
	trampoline := ensureFuncRefTrampoline(ctx, e.FuncName)
	return fmt.Sprintf("(%s){.fn = %s, .env = NULL}", mangleCallableType(e.Type.Entity), trampoline), fromIRType(e.Type), nil
}

// ensureLambdaFunc эмитит (один раз на e.Name - уникально для каждого узла
// лямбды во всей программе, см. internal/semantic.Analyzer.lambdaCounter)
// собственный typedef окружения (если есть хотя бы один захват) и саму
// C-функцию тела лямбды.
func ensureLambdaFunc(ctx *irGenCtx, e *ir.LambdaExpr) error {
	if ctx.lambdaFuncsEmitted == nil {
		ctx.lambdaFuncsEmitted = map[string]bool{}
	}
	if ctx.lambdaFuncsEmitted[e.Name] {
		return nil
	}
	ctx.lambdaFuncsEmitted[e.Name] = true

	var sb strings.Builder
	envType := mangleCallableEnvType(e.Name)
	if len(e.Captures) > 0 {
		sb.WriteString("typedef struct {\n")
		for _, c := range e.Captures {
			fmt.Fprintf(&sb, "    %s %s;\n", fromIRType(c.Type).cType(ctx), mangleField(c.Name))
		}
		fmt.Fprintf(&sb, "} %s;\n\n", envType)
	}

	var result *ir.Type
	if e.HasReturn {
		result = &e.ReturnType
	}
	params := make([]string, 0, len(e.Params)+1)
	params = append(params, "void *rech_env")
	for _, p := range e.Params {
		params = append(params, fmt.Sprintf("%s %s", fromIRType(p.Type).cType(ctx), mangleVar(p.Name)))
	}
	fmt.Fprintf(&sb, "static %s %s(%s) {\n", callableResultCType(ctx, result), mangleLambdaFunc(e.Name), strings.Join(params, ", "))
	if len(e.Captures) > 0 {
		fmt.Fprintf(&sb, "    %s *rech_captured = (%s *)rech_env;\n", envType, envType)
	} else {
		sb.WriteString("    (void)rech_env;\n")
	}

	// capturedRedirect - каждое имя захвата читается/пишется прямо через
	// rech_captured->..., НЕ через отдельную локальную переменную -
	// отдельная копия на входе (Type name = rech_captured->field;)
	// потерялась бы к следующему вызову ЭТОЙ ЖЕ лямбды: захват - общее
	// изменяемое хранилище между всеми вызовами одного значения лямбды
	// (счётчик, копящийся между вызовами), не свежий снимок на каждый
	// вызов. Сохраняем/восстанавливаем прежнее состояние - тело этой
	// лямбды может само содержать вложенную лямбду с собственным (в общем
	// случае другим) списком захватов.
	prevRedirect := ctx.capturedRedirect
	if len(e.Captures) > 0 {
		redirect := make(map[string]string, len(e.Captures))
		for _, c := range e.Captures {
			redirect[c.Name] = "rech_captured->" + mangleField(c.Name)
		}
		ctx.capturedRedirect = redirect
	} else {
		ctx.capturedRedirect = nil
	}
	// prevCleanupStack - см. package doc irGenCtx.cleanupStack: тело лямбды -
	// СОБСТВЕННАЯ C-функция, "вернуть" внутри неё не обязан (и не может)
	// видеть кадры попытаться, охватывающего саму лямбду СНАРУЖИ.
	prevCleanupStack := ctx.cleanupStack
	ctx.cleanupStack = nil
	var bodyErr error
	for _, s := range e.Body {
		if err := generateIRStmt(&sb, ctx, s, 1); err != nil {
			bodyErr = err
			break
		}
	}
	ctx.capturedRedirect = prevRedirect
	ctx.cleanupStack = prevCleanupStack
	if bodyErr != nil {
		return fmt.Errorf("лямбда: %w", bodyErr)
	}

	if !e.HasReturn {
		sb.WriteString("    return;\n")
	}
	sb.WriteString("}\n\n")
	ctx.callableCode.WriteString(sb.String())
	return nil
}

// generateIRLambdaExpr - "функция(...) { ... }" в позиции выражения -
// окружение (если есть захваты) строится одним C-выражением: GC_MALLOC под
// его размер, скопированный (memcpy) из compound literal со значениями
// захваченных переменных, взятыми прямо из текущей C-области видимости
// (mangleVar(c.Name) - то же самое C-имя, под которым эта же переменная
// доступна в окружающем коде, значение уже скопировано семантическим
// анализом самим фактом копирования Value при обычном присваивании/чтении -
// здесь достаточно просто прочитать её ещё раз).
func generateIRLambdaExpr(ctx *irGenCtx, e *ir.LambdaExpr) (string, valueType, error) {
	if err := ensureLambdaFunc(ctx, e); err != nil {
		return "", typeUnknown, err
	}
	envExpr := "NULL"
	if len(e.Captures) > 0 {
		ctx.usesGC = true
		envType := mangleCallableEnvType(e.Name)
		fields := make([]string, len(e.Captures))
		for i, c := range e.Captures {
			fields[i] = fmt.Sprintf(".%s = %s", mangleField(c.Name), varRef(ctx, c.Name))
		}
		envExpr = fmt.Sprintf("memcpy(GC_MALLOC(sizeof(%s)), &(%s){%s}, sizeof(%s))",
			envType, envType, strings.Join(fields, ", "), envType)
		ctx.usesMemcpy = true
	}
	return fmt.Sprintf("(%s){.fn = %s, .env = %s}", mangleCallableType(e.Type.Entity), mangleLambdaFunc(e.Name), envExpr), fromIRType(e.Type), nil
}

// generateIRCallValueExpr - "ф(<арг>, ...)", где ф - значение типа
// "Вызываемый" (не имя объявленной функции) - вызывает fn через env,
// единым способом для &имя и лямбды (обе стороны уже согласовали одну и ту
// же сигнатуру "окружение первым параметром" - см. package doc).
func generateIRCallValueExpr(ctx *irGenCtx, e *ir.CallValueExpr) (string, valueType, error) {
	fCode, _, err := generateIRExpr(ctx, e.Value)
	if err != nil {
		return "", typeUnknown, err
	}
	args := make([]string, 0, len(e.Args)+1)
	args = append(args, fmt.Sprintf("(%s).env", fCode))
	for _, a := range e.Args {
		code, _, err := generateIRExpr(ctx, a)
		if err != nil {
			return "", typeUnknown, err
		}
		args = append(args, code)
	}
	return fmt.Sprintf("(%s).fn(%s)", fCode, strings.Join(args, ", ")), fromIRType(e.Type), nil
}
