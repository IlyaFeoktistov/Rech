// Исключения ("выбросить"/"попытаться"/"поймать", "относится к") -
// нелокальный поток управления поверх C: реализован через setjmp/longjmp,
// а не через код возврата ошибки, потому что остальная кодогенерация
// (generateIRStmt/generateIRExpr) транслирует Rech в прямую C-структуру
// управления один в один, без сквозного канала распространения ошибки -
// вставлять проверку "ошибка ли это" после каждого вызова означало бы
// переписать всю кодогенерацию, тогда как setjmp/longjmp разматывает стек
// C-вызовов, не трогая ни одной другой инструкции.
//
// rech_isa - плоская таблица "конкретный тип относится к типу X" (и для
// "поймать", и для "относится к" - один и тот же вопрос), проиндексированная
// type_id из vtable.go (buildIsaTable). Исключения и паники - один канал:
// брошенное значение - всегда объект "Исключение"/"Паника" или наследника,
// оба корня всегда участвуют в диспетчеризации (computeDispatch), поэтому
// vt->type_id есть у любого брошенного объекта.
package cbackend

import (
	"fmt"
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// systemExceptionEntity - см. internal/semantic/exception.go, тот же
// принцип общего идентификатора константы, что и entryPointName.
const systemExceptionEntity = "Исключение"

// buildIsaTable строит соответствие "имя типа -> type_id" (rows - только
// сущности-участники диспетчеризации, т. к. только у них бывают реальные
// экземпляры; следом, тем же пространством id - интерфейсы, они встречаются
// только как цель проверки, никогда как строка таблицы) и саму таблицу
// "относится к" - table[id строки][id столбца] истинно, если сущность
// строки - сама цель столбца, ЛЮБОЙ её предок по цепочке BaseName - цель
// столбца, или она (либо любой предок) "реализует" интерфейс-цель столбца
// (Entity.Interfaces - собственные для каждого уровня, см. package doc
// internal/ir, Entity.Interfaces). Сортировка имён - только для
// детерминированного вывода C (id сами по себе не видны в Rech).
func buildIsaTable(prog *ir.Program, participants map[string]bool) (typeIDs map[string]int, rowNames []string, table [][]bool) {
	entities := map[string]*ir.Entity{}
	for _, ent := range prog.Entities {
		entities[ent.Name] = ent
	}

	for name := range participants {
		rowNames = append(rowNames, name)
	}
	sort.Strings(rowNames)

	var ifaceNames []string
	for _, iface := range prog.Interfaces {
		ifaceNames = append(ifaceNames, iface.Name)
	}
	sort.Strings(ifaceNames)

	typeIDs = make(map[string]int, len(rowNames)+len(ifaceNames))
	for i, name := range rowNames {
		typeIDs[name] = i
	}
	for i, name := range ifaceNames {
		typeIDs[name] = len(rowNames) + i
	}

	table = make([][]bool, len(rowNames))
	for i, rowName := range rowNames {
		table[i] = make([]bool, len(typeIDs))
		for name := rowName; name != ""; {
			ent, ok := entities[name]
			if !ok {
				break
			}
			table[i][typeIDs[name]] = true
			for _, iface := range ent.Interfaces {
				if id, ok := typeIDs[iface]; ok {
					table[i][id] = true
				}
			}
			name = ent.BaseName
		}
	}
	return typeIDs, rowNames, table
}

// catchFrameType/catchTopVar/throwFunc - имена рантайм-инфраструктуры
// setjmp/longjmp, общие на всю программу (как vtfuncType/vtableType в
// vtable.go).
const (
	catchFrameType = "rech_catch_frame"
	catchTopVar    = "rech_catch_top"
	throwFunc      = "rech_throw"
)

// cleanupAction - действие, которое должен выполнить "вернуть" ПЕРЕД
// настоящим C `return`, если он лексически встретился внутри Body/CatchBody
// охватывающего "попытаться" (см. package doc irGenCtx.cleanupStack,
// ir_generator.go) - raw C `return`, в отличие от "выбросить"/паники
// (setjmp/longjmp, восстанавливается самим рантаймом на месте кадра), сам
// по себе никак не взаимодействует с rech_catch_top.
type cleanupAction func(sb *strings.Builder, ctx *irGenCtx, depth int) error

// pushFrameRestoreCleanup регистрирует восстановление rech_catch_top до
// значения "до этого кадра" (frameVar.prev) как cleanup-действие - для
// узлов "попытаться" БЕЗ "окончательно" (generateIRTryStmt/
// generateIRTryCatchPanicStmt/generateIRTryCatchMixedStmt) им больше нечего
// делать при раннем возврате (у generateIRTryWithFinallyStmt действие
// сложнее - собирается на месте, не через эту функцию, там ЕЩЁ нужно
// запустить "окончательно"). Возвращает pop-функцию, которую вызывающая
// сторона обязана вызвать РОВНО ОДИН РАЗ, после того как Body/CatchBody
// узла уже сгенерированы.
func pushFrameRestoreCleanup(ctx *irGenCtx, frameVar string) func() {
	prev := ctx.cleanupStack
	action := cleanupAction(func(sb *strings.Builder, actx *irGenCtx, depth int) error {
		fmt.Fprintf(sb, "%s%s = %s.prev;\n", indent(depth), catchTopVar, frameVar)
		return nil
	})
	ctx.cleanupStack = append(append([]cleanupAction{}, prev...), action)
	return func() {
		ctx.cleanupStack = prev
	}
}

// emitCleanupUnwind прогоняет ЦЕЛИКОМ ctx.cleanupStack в ОБРАТНОМ порядке
// (LIFO - самый внутренний "попытаться" первым) - "вернуть" (generateIRReturnStmt,
// ir_generator.go) выходит из ФУНКЦИИ целиком, поэтому обязан размотать ВСЕ
// охватывающие кадры "попытаться", без исключений.
func emitCleanupUnwind(sb *strings.Builder, ctx *irGenCtx, depth int) error {
	return emitCleanupUnwindFrom(sb, ctx, depth, 0)
}

// emitCleanupUnwindFrom - как emitCleanupUnwind, но прогоняет только
// действия с индексом >= base (не весь стек) - "прервать"/"продолжить"
// (break_continue.go, emitLoopJump) выходят только из ОДНОГО охватывающего
// цикла, а не из функции целиком: действия, добавленные "попытаться" ВНЕ
// этого цикла (индекс < base - кадр перехвата ещё активен, цикл прерывается,
// но "попытаться" вокруг него - нет), не должны разматываться вовсе.
func emitCleanupUnwindFrom(sb *strings.Builder, ctx *irGenCtx, depth int, base int) error {
	for i := len(ctx.cleanupStack) - 1; i >= base; i-- {
		if err := ctx.cleanupStack[i](sb, ctx, depth); err != nil {
			return err
		}
	}
	return nil
}

// exceptionRuntimeCode - стек кадров "попытаться" (односвязный список,
// растёт с каждым вложенным "попытаться" и не требует заранее известной
// глубины) и rech_throw - единственная точка, которая ЛИБО прыгает в
// ближайший кадр (longjmp), ЛИБО, если кадров не осталось, печатает
// сообщение о необработанном исключении и завершает процесс (rtmsg.
// UncaughtExceptionFormat) - typeName вычисляет вызывающий код
// (thrownTypeNameExpr).
func exceptionRuntimeCode(ctx *irGenCtx) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "typedef struct %s {\n", catchFrameType)
	sb.WriteString("    jmp_buf buf;\n")
	fmt.Fprintf(&sb, "    struct %s *prev;\n", catchFrameType)
	sb.WriteString("    void *exc;\n")
	fmt.Fprintf(&sb, "} %s;\n", catchFrameType)
	// rech_catch_top - per-поток (не общий на процесс): без этого
	// "попытаться" в одном потоке ОС (шаг 15) видело бы/портило кадры
	// другого - RECH_THREAD_LOCAL (__declspec(thread)/__thread, не C11
	// _Thread_local - не на всех тулчейнах, которые нужно поддерживать)
	// определён безусловно в GenerateFromIR (нужен и rech_trace, trace.go,
	// эмитится всегда). Тред-локальность не вредит и
	// однопоточной программе (то же поведение, что и у обычного static) -
	// не гейтится отдельно от usesExceptions самим usesThreads.
	fmt.Fprintf(&sb, "static RECH_THREAD_LOCAL %s *%s = NULL;\n\n", catchFrameType, catchTopVar)

	// rech_isa/rech_type_names - общие для ЛЮБОГО участника диспетчеризации
	// (generateIRIsExpr использует их для "относится к" независимо от
	// исключений). Пустыми они бывают только у IR без встроенных корней
	// (юнит-тесты) - массивы нулевого размера в C невалидны.
	if len(ctx.excIsaTable) > 0 {
		fmt.Fprintf(&sb, "static const bool %s[%d][%d] = {\n", isaTableVar, len(ctx.excIsaTable), len(ctx.excTypeIDs))
		for i, row := range ctx.excIsaTable {
			var cols []string
			for j, v := range row {
				if v {
					cols = append(cols, fmt.Sprintf("[%d] = true", j))
				}
			}
			fmt.Fprintf(&sb, "    [%d] = { %s },\n", i, strings.Join(cols, ", "))
		}
		sb.WriteString("};\n\n")

		fmt.Fprintf(&sb, "static const char *%s[%d] = {\n", typeNamesVar, len(ctx.excTypeNames))
		for i, name := range ctx.excTypeNames {
			fmt.Fprintf(&sb, "    [%d] = \"%s\",\n", i, escapeCString(name))
		}
		sb.WriteString("};\n\n")
	}

	// template - тот же текст, что печатает interp (thrownException.Error,
	// internal/interp/exception.go) - "%s"/"%s" передаются в
	// UncaughtExceptionFormat как ЗНАЧЕНИЯ (не формат), поэтому в
	// результате остаются буквальными C-плейсхолдерами %s, не
	// интерпретируются повторно. Непойманная паника печатается без имени
	// типа (rtmsg.PanicPrefix + сообщение) - тем же текстом, что и раньше,
	// до иерархии паник.
	template := rtmsg.PanicPrefix + rtmsg.UncaughtExceptionFormat("%s", "%s")
	excMsg := fmt.Sprintf("((const %s *)exc)->%s", mangleType(systemExceptionEntity), mangleField("сообщение"))
	panicMsg := fmt.Sprintf("((const %s *)exc)->%s", mangleType(rtmsg.PanicEntity), mangleField("сообщение"))
	fmt.Fprintf(&sb, "static void %s(void *exc, const char *type_name) {\n", throwFunc)
	fmt.Fprintf(&sb, "    %s *f = %s;\n", catchFrameType, catchTopVar)
	sb.WriteString("    if (f == NULL) {\n")
	fmt.Fprintf(&sb, "        if (%s) {\n", isPanicExpr(ctx, "exc"))
	fmt.Fprintf(&sb, "            fprintf(stderr, \"%s%%s\\n\", %s);\n", escapeCString(rtmsg.PanicPrefix), panicMsg)
	sb.WriteString("        } else {\n")
	fmt.Fprintf(&sb, "            fprintf(stderr, \"%s\\n\", type_name, %s);\n", escapeCString(template), excMsg)
	sb.WriteString("        }\n")
	sb.WriteString("        rech_trace_print();\n")
	sb.WriteString("        exit(1);\n")
	sb.WriteString("    }\n")
	fmt.Fprintf(&sb, "    %s = f->prev;\n", catchTopVar)
	sb.WriteString("    f->exc = exc;\n")
	sb.WriteString("    longjmp(f->buf, 1);\n")
	sb.WriteString("}\n")
	return sb.String()
}

// isPanicExpr - C-условие "брошенное значение exc (void *) - паника"
// (сама "Паника" или наследник).
func isPanicExpr(ctx *irGenCtx, exc string) string {
	return fmt.Sprintf("%s[((const %s *)(%s))->vt->type_id][%d]", isaTableVar, vtableHeaderType, exc, ctx.excTypeIDs[rtmsg.PanicEntity])
}

// thrownMessageExpr - C-выражение поля "сообщение" брошенного значения exc
// любой из двух иерархий (у "Паника" и "Исключение" разные C-структуры).
func thrownMessageExpr(ctx *irGenCtx, exc string) string {
	return fmt.Sprintf("(%s ? ((const %s *)(%s))->%s : ((const %s *)(%s))->%s)", isPanicExpr(ctx, exc),
		mangleType(rtmsg.PanicEntity), exc, mangleField("сообщение"),
		mangleType(systemExceptionEntity), exc, mangleField("сообщение"))
}

// panicRuntimeCode - rech_panic_as(вид, сообщение): строит встроенную
// панику вида rtmsg.PanicKinds[вид] и бросает её тем же rech_throw, что и
// "выбросить" (поймать сопоставляет тип как у любого значения). Сообщение
// копируется в кучу ДО размотки - буфер вызывающей стороны может быть её
// локальной C-переменной (см. rech_arg, builtins_shared.go). rech_panic -
// корневая "Паника". Эмитится после прототипов конструкторов.
func panicRuntimeCode() string {
	var sb strings.Builder
	sb.WriteString("static void rech_panic_as(int kind, const char *msg) {\n")
	sb.WriteString("    size_t len = strlen(msg);\n")
	sb.WriteString("    char *copy = GC_MALLOC_ATOMIC(len + 1);\n")
	sb.WriteString("    memcpy(copy, msg, len + 1);\n")
	sb.WriteString("    switch (kind) {\n")
	for i, name := range rtmsg.PanicKinds {
		if i == 0 {
			continue
		}
		fmt.Fprintf(&sb, "    case %d: %s(%s(copy), \"%s\"); break;\n", i, throwFunc, mangleCtor(name), escapeCString(name))
	}
	fmt.Fprintf(&sb, "    default: %s(%s(copy), \"%s\"); break;\n", throwFunc, mangleCtor(rtmsg.PanicEntity), escapeCString(rtmsg.PanicEntity))
	sb.WriteString("    }\n")
	sb.WriteString("    exit(1);\n")
	sb.WriteString("}\n\n")
	sb.WriteString("static void rech_panic(const char *msg) {\n")
	sb.WriteString("    rech_panic_as(0, msg);\n")
	sb.WriteString("}\n\n")
	sb.WriteString(errorRuntimeCode())
	return sb.String()
}

// errorRuntimeCode - rech_error_as(вид, сообщение): встроенное исключение
// rtmsg.ErrorKinds[вид] (ожидаемая ситуация - файл, сеть, БД, разбор, ...)
// тем же rech_throw, что и "выбросить"; сообщение копируется до размотки,
// как у rech_panic_as.
func errorRuntimeCode() string {
	var sb strings.Builder
	sb.WriteString("static void rech_error_as(int kind, const char *msg) {\n")
	sb.WriteString("    size_t len = strlen(msg);\n")
	sb.WriteString("    char *copy = GC_MALLOC_ATOMIC(len + 1);\n")
	sb.WriteString("    memcpy(copy, msg, len + 1);\n")
	sb.WriteString("    switch (kind) {\n")
	for _, name := range rtmsg.ErrorKinds {
		fmt.Fprintf(&sb, "    case %s: %s(%s(copy), \"%s\"); break;\n", rtmsg.ErrorKindMacros[name], throwFunc, mangleCtor(name), escapeCString(name))
	}
	fmt.Fprintf(&sb, "    default: %s(%s(copy), \"%s\"); break;\n", throwFunc, mangleCtor(systemExceptionEntity), escapeCString(systemExceptionEntity))
	sb.WriteString("    }\n")
	sb.WriteString("    exit(1);\n")
	sb.WriteString("}\n\n")
	return sb.String()
}

// errorKindDefines - #define RECH_ERR_* для каждого встроенного исключения
// (код вида rech_error_as - индекс в rtmsg.ErrorKinds).
func errorKindDefines() string {
	var sb strings.Builder
	for i, name := range rtmsg.ErrorKinds {
		fmt.Fprintf(&sb, "#define %s %d\n", rtmsg.ErrorKindMacros[name], i)
	}
	return sb.String()
}

// panicPrototypes - объявления rech_panic/rech_panic_as до первого
// хелпера, который их вызывает (определения - panicRuntimeCode, позже).
// rech_error_as и коды видов RECH_ERR_* - там же (errorRuntimeCode).
var panicPrototypes = "static void rech_panic_as(int kind, const char *msg);\nstatic void rech_panic(const char *msg);\n" +
	errorKindDefines() + "static void rech_error_as(int kind, const char *msg);\n"

// panicCallC - C-вызов, бросающий встроенную панику kind (rtmsg.Panic*) с
// уже экранированным C-литералом/выражением msgC.
func panicCallC(kind, msgC string) string {
	return fmt.Sprintf("rech_panic_as(%d, %s)", rtmsg.PanicKind(kind), msgC)
}

// errorCallC - C-вызов, бросающий встроенное исключение kind (rtmsg.Error*)
// с уже экранированным C-литералом/выражением msgC.
func errorCallC(kind, msgC string) string {
	return fmt.Sprintf("rech_error_as(%s, %s)", rtmsg.ErrorKindMacros[kind], msgC)
}

// isaTableVar - имя C-массива rech_isa (см. exceptionRuntimeCode/
// generateIRTryStmt/generateIRIsExpr).
const isaTableVar = "rech_isa"

// typeNamesVar - имя C-массива type_id -> строковое имя типа Rech (см.
// thrownTypeNameExpr).
const typeNamesVar = "rech_type_names"

// generateIRThrowStmt - "выбросить <выражение>".
func generateIRThrowStmt(sb *strings.Builder, ctx *irGenCtx, s *ir.ThrowStmt, depth int) error {
	ctx.usesExceptions = true
	code, _, err := generateIRExpr(ctx, s.Value)
	if err != nil {
		return err
	}
	nameExpr := thrownTypeNameExpr(code)
	fmt.Fprintf(sb, "%s%s(%s, %s);\n", indent(depth), throwFunc, code, nameExpr)
	return nil
}

// generateIRTryStmt - "попытаться { ... } поймать(<имя>: <Тип>[|<Тип>...]) { ... }".
// Каждый "попытаться" заводит собственный кадр (frameVar, счётчик
// ctx.tryCounter гарантирует уникальное C-имя у вложенных) - см. package
// doc про сам механизм setjmp/longjmp. Исключения и паники - один канал:
// брошенное значение сопоставляется с CatchTypes по rech_isa, не
// подошло - бросается дальше тем же rech_throw.
func generateIRTryStmt(sb *strings.Builder, ctx *irGenCtx, s *ir.TryStmt, depth int) error {
	if s.NoCatch || len(s.Finally) > 0 {
		return generateIRTryWithFinallyStmt(sb, ctx, s, depth)
	}
	ctx.usesExceptions = true
	ctx.tryCounter++
	frame := fmt.Sprintf("rech_frame_%d", ctx.tryCounter)
	ind := indent(depth)
	in1 := indent(depth + 1)
	in2 := indent(depth + 2)
	in3 := indent(depth + 3)

	// traceVar - глубина стека вызовов (rech_trace, trace.go) на момент
	// входа в "попытаться". longjmp у rech_throw пропускает обычный
	// rech_trace_pop() каждого размотанного уровня между местом "выбросить"
	// и этим "поймать" - без восстановления эти кадры остались бы висеть в
	// rech_trace (следующая паника ПОСЛЕ этого catch увидела бы чужой, уже
	// завершившийся стек вызовов вперемешку со своим).
	traceVar := frame + "_trace"
	excPtr := frame + "_exc"
	fmt.Fprintf(sb, "%s{\n", ind)
	sb.WriteString(traceSave(ctx, in1, traceVar))
	fmt.Fprintf(sb, "%s%s %s = {0};\n", in1, catchFrameType, frame)
	fmt.Fprintf(sb, "%s%s.prev = %s;\n", in1, frame, catchTopVar)
	fmt.Fprintf(sb, "%s%s = &%s;\n", in1, catchTopVar, frame)
	// pushFrameCleanup/pop - см. package doc cleanupAction: "вернуть"
	// внутри Body/CatchBody ниже - голый C return, не проходит через
	// setjmp/longjmp, поэтому обязан восстановить rech_catch_top сам
	// (иначе он остаётся висеть на frame - локальной переменной, которая
	// вот-вот перестанет существовать при возврате из C-функции).
	popCleanup := pushFrameRestoreCleanup(ctx, frame)
	fmt.Fprintf(sb, "%sif (setjmp(%s.buf) == 0) {\n", in1, frame)
	for _, st := range s.Body {
		if err := generateIRStmt(sb, ctx, st, depth+2); err != nil {
			return err
		}
	}
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, frame)
	fmt.Fprintf(sb, "%s} else {\n", in1)
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, frame)
	sb.WriteString(traceRestore(ctx, in2, traceVar))
	fmt.Fprintf(sb, "%svoid *%s = %s.exc;\n", in2, excPtr, frame)
	cond, err := catchMatchExpr(ctx, s, excPtr)
	if err != nil {
		return err
	}
	fmt.Fprintf(sb, "%sif (%s) {\n", in2, cond)
	if err := writeCatchBody(sb, ctx, s, excPtr, depth+3); err != nil {
		return err
	}
	fmt.Fprintf(sb, "%s} else {\n", in2)
	fmt.Fprintf(sb, "%s%s(%s, %s);\n", in3, throwFunc, excPtr, thrownTypeNameExpr(excPtr))
	fmt.Fprintf(sb, "%s}\n", in2)
	popCleanup()
	fmt.Fprintf(sb, "%s}\n%s}\n", in1, ind)
	return nil
}

// catchMatchExpr - C-условие "брошенное exc относится хотя бы к одному из
// s.CatchTypes".
func catchMatchExpr(ctx *irGenCtx, s *ir.TryStmt, exc string) (string, error) {
	conds := make([]string, len(s.CatchTypes))
	for i, t := range s.CatchTypes {
		id, ok := ctx.excTypeIDs[t]
		if !ok {
			return "", fmt.Errorf("генерация C из IR: тип %q поймать(...) не найден в таблице rech_isa", t)
		}
		conds[i] = fmt.Sprintf("%s[((const %s *)(%s))->vt->type_id][%d]", isaTableVar, vtableHeaderType, exc, id)
	}
	return strings.Join(conds, " || "), nil
}

// thrownTypeNameExpr - имя фактического типа брошенного exc.
func thrownTypeNameExpr(exc string) string {
	return fmt.Sprintf("%s[((const %s *)(%s))->vt->type_id]", typeNamesVar, vtableHeaderType, exc)
}

// writeCatchBody - привязка CatchVar к брошенному exc (само значение или,
// у смешанного списка обеих иерархий, его "сообщение" - см. package doc
// ir.TryStmt.CatchVarType) и тело обработчика.
func writeCatchBody(sb *strings.Builder, ctx *irGenCtx, s *ir.TryStmt, exc string, depth int) error {
	catchVarC := fromIRType(s.CatchVarType).cType(ctx)
	catchVar := mangleVar(s.CatchVar)
	if s.CatchVarType.Kind == ir.KindString {
		fmt.Fprintf(sb, "%s%s %s = %s;\n", indent(depth), catchVarC, catchVar, thrownMessageExpr(ctx, exc))
	} else {
		fmt.Fprintf(sb, "%s%s %s = (%s)%s;\n", indent(depth), catchVarC, catchVar, catchVarC, exc)
	}
	for _, st := range s.CatchBody {
		if err := generateIRStmt(sb, ctx, st, depth); err != nil {
			return err
		}
	}
	return nil
}

// generateIRTryWithFinallyStmt - узел цепочки с "окончательно" (package doc
// ast.TryStmt.Finally/NoCatch) - либо вовсе без "поймать" (s.NoCatch), либо
// обычный "поймать" с довеском "окончательно". Кадр ловит ЛЮБОЕ брошенное
// значение: "окончательно" обязан выполниться и перед тем, как пробросить
// дальше то, что этот "поймать" не перехватывает (frame_outcome 2).
// CatchBody (если есть) генерируется РОВНО ОДИН РАЗ.
func generateIRTryWithFinallyStmt(sb *strings.Builder, ctx *irGenCtx, s *ir.TryStmt, depth int) error {
	ctx.usesExceptions = true
	ctx.tryCounter++
	frame := fmt.Sprintf("rech_frame_%d", ctx.tryCounter)
	ind := indent(depth)
	in1 := indent(depth + 1)
	in2 := indent(depth + 2)

	excPtr := frame + "_excptr"
	traceVar := frame + "_trace"
	outcomeVar := frame + "_outcome" // 0 обычное завершение, 1 - свой поймать сработал, 2 - брошенное прошло насквозь

	fmt.Fprintf(sb, "%s{\n", ind)
	sb.WriteString(traceSave(ctx, in1, traceVar))
	fmt.Fprintf(sb, "%s%s %s = {0};\n", in1, catchFrameType, frame)
	fmt.Fprintf(sb, "%s%s.prev = %s;\n", in1, frame, catchTopVar)
	fmt.Fprintf(sb, "%s%s = &%s;\n", in1, catchTopVar, frame)
	fmt.Fprintf(sb, "%sint %s = 0;\n", in1, outcomeVar)
	fmt.Fprintf(sb, "%svoid *%s = NULL;\n", in1, excPtr)

	// prevStack/action - см. package doc cleanupAction: раннее "вернуть"
	// внутри Body/CatchBody обязано и восстановить rech_catch_top, и
	// прогнать "окончательно" (temp-переключение actx.cleanupStack на
	// prevStack на время его генерации - вложенное "вернуть" внутри самого
	// Finally обязано видеть только ВНЕШНИЕ по отношению к этому узлу
	// кадры, не крутиться на этом же действии повторно).
	prevStack := ctx.cleanupStack
	action := cleanupAction(func(asb *strings.Builder, actx *irGenCtx, adepth int) error {
		aind := indent(adepth)
		fmt.Fprintf(asb, "%s%s = %s.prev;\n", aind, catchTopVar, frame)
		saved := actx.cleanupStack
		actx.cleanupStack = prevStack
		var ferr error
		for _, st := range s.Finally {
			if err := generateIRStmt(asb, actx, st, adepth); err != nil {
				ferr = err
				break
			}
		}
		actx.cleanupStack = saved
		return ferr
	})
	ctx.cleanupStack = append(append([]cleanupAction{}, prevStack...), action)

	fmt.Fprintf(sb, "%sif (setjmp(%s.buf) == 0) {\n", in1, frame)
	for _, st := range s.Body {
		if err := generateIRStmt(sb, ctx, st, depth+2); err != nil {
			return err
		}
	}
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, frame)
	fmt.Fprintf(sb, "%s} else {\n", in1)
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, frame)
	sb.WriteString(traceRestore(ctx, in2, traceVar))
	fmt.Fprintf(sb, "%s%s = %s.exc;\n", in2, excPtr, frame)
	if s.NoCatch {
		fmt.Fprintf(sb, "%s%s = 2;\n", in2, outcomeVar)
	} else {
		cond, err := catchMatchExpr(ctx, s, excPtr)
		if err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s%s = (%s) ? 1 : 2;\n", in2, outcomeVar, cond)
	}
	fmt.Fprintf(sb, "%s}\n", in1)

	ctx.cleanupStack = prevStack // Body/CatchBody сгенерированы - см. package doc выше.

	if !s.NoCatch {
		fmt.Fprintf(sb, "%sif (%s == 1) {\n", in1, outcomeVar)
		if err := writeCatchBody(sb, ctx, s, excPtr, depth+2); err != nil {
			return err
		}
		fmt.Fprintf(sb, "%s}\n", in1)
	}

	for _, st := range s.Finally {
		if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
			return err
		}
	}

	fmt.Fprintf(sb, "%sif (%s == 2) {\n", in1, outcomeVar)
	fmt.Fprintf(sb, "%s%s(%s, %s);\n", in2, throwFunc, excPtr, thrownTypeNameExpr(excPtr))
	fmt.Fprintf(sb, "%s}\n", in1)
	fmt.Fprintf(sb, "%s}\n", ind)
	return nil
}

// generateIRIsExpr - "<выражение> относится к <Тип>". Если статический тип
// операнда не участвует в диспетчеризации (needsDispatch) - его
// фактический тип статически равен объявленному (иного он нести не может,
// см. package doc internal/semantic, relatedForIsCheck) - результат уже
// известен на этапе компиляции (семантический анализ уже проверил, что
// Target связан с ним - значит совпадает), константа "true" без обращения
// к рантайму вовсе.
func generateIRIsExpr(ctx *irGenCtx, e *ir.IsExpr) (string, valueType, error) {
	operandType := ir.TypeOf(e.Value).Entity
	if !ctx.needsDispatch(operandType) {
		return "true", typeBool, nil
	}
	ctx.usesExceptions = true
	code, _, err := generateIRExpr(ctx, e.Value)
	if err != nil {
		return "", typeUnknown, err
	}
	id, ok := ctx.excTypeIDs[e.Target]
	if !ok {
		return "", typeUnknown, fmt.Errorf("генерация C из IR: тип %q в \"относится к\" не найден в таблице rech_isa", e.Target)
	}
	ptr := code
	if !ctx.receiverIsPointer(e.Value) {
		ptr = fmt.Sprintf("&(%s)", code)
	}
	return fmt.Sprintf("%s[((const %s *)(%s))->vt->type_id][%d]", isaTableVar, vtableHeaderType, ptr, id), typeBool, nil
}
