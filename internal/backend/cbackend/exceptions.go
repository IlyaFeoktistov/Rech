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
// type_id из vtable.go (buildIsaTable). Безопасность чтения vt->type_id у
// брошенного значения опирается на то, что "Исключение" - общий предок
// ЛЮБОГО исключения программы: если хотя бы один наследник объявлен где-то
// в программе, "Исключение" становится участником диспетчеризации
// (computeDispatch, isBase) - значит и её C-структура, единая на всю
// программу, получает поле vt, и ЛЮБОЙ объект типа "Исключение" или
// наследника (в т. ч. голый "новое Исключение(...)" без единого наследника
// по соседству) несёт его тоже - см. irGenCtx.exceptionsUseDispatch. Если
// наследников нет вовсе - в программе физически не может существовать
// другого исключения, кроме голого "Исключение", и rech_isa/type_id не
// нужны вообще - поймать/относится к разрешаются на этапе компиляции.
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
// UncaughtExceptionFormat) - typeName вычисляется КАЖДЫМ вызывающим кодом
// (generateIRThrowStmt/generateIRTryStmt) заранее, а не внутри самой
// функции, потому что ровно вызывающий код знает, безопасно ли читать
// vt->type_id у конкретного значения (exceptionsUseDispatch).
func exceptionRuntimeCode(ctx *irGenCtx) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "typedef struct %s {\n", catchFrameType)
	sb.WriteString("    jmp_buf buf;\n")
	fmt.Fprintf(&sb, "    struct %s *prev;\n", catchFrameType)
	sb.WriteString("    void *exc;\n")
	// catch_panics/panic_msg - только у кадров "поймать (x: Паника)"
	// (generateIRTryCatchPanicStmt) - живут в ОБЩЕМ catchFrameType (не
	// отдельная C-структура), чтобы rech_throw и rech_panic
	// (panicCatchHelperCode) искали каждый свой кадр по одному и тому же
	// связному списку rech_catch_top, просто пропуская чужие (docs/
	// изоляция паники) - обычный кадр "поймать(Тип)" всегда
	// нулевой (см. rech_catch_frame_zero в generateIRTryStmt), catch_panics
	// у него гарантированно false.
	sb.WriteString("    bool catch_panics;\n")
	sb.WriteString("    const char *panic_msg;\n")
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
	// (не только наследников "Исключение" - generateIRIsExpr использует их
	// для "относится к" совершенно независимо от исключений), поэтому
	// эмитятся, когда есть хотя бы один участник (не exceptionsUseDispatch -
	// тот флаг узко про безопасность чтения vt->type_id у БРОШЕННОГО
	// значения, см. throwTypeNameExpr). Ноль участников, но usesExceptions
	// всё равно true - "попытаться"/"выбросить" голого "Исключение" без
	// единого наследника где-либо в программе (см. package doc) - массивы
	// нулевого размера в C не нужны и, скорее всего, невалидны.
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
	// интерпретируются повторно.
	template := rtmsg.PanicPrefix + rtmsg.UncaughtExceptionFormat("%s", "%s")
	msgField := fmt.Sprintf("((const %s *)exc)->%s", mangleType(systemExceptionEntity), mangleField("сообщение"))
	fmt.Fprintf(&sb, "static void %s(void *exc, const char *type_name) {\n", throwFunc)
	fmt.Fprintf(&sb, "    %s *f = %s;\n", catchFrameType, catchTopVar)
	// Кадры "поймать (x: Паника)" (catch_panics) для "выбросить" не в счёт -
	// у них нет ни диспетчеризации по типу, ни поля exc (f->panic_msg вместо
	// него, см. package doc catchFrameType) - брошенное исключение обязано
	// пролететь их насквозь до ближайшего НАСТОЯЩЕГО поймать(Тип)/самого
	// верха, как если бы их не было вовсе.
	sb.WriteString("    while (f != NULL && f->catch_panics) {\n")
	sb.WriteString("        f = f->prev;\n")
	sb.WriteString("    }\n")
	sb.WriteString("    if (f == NULL) {\n")
	fmt.Fprintf(&sb, "        fprintf(stderr, \"%s\\n\", type_name, %s);\n", escapeCString(template), msgField)
	sb.WriteString("        rech_trace_print();\n")
	sb.WriteString("        exit(1);\n")
	sb.WriteString("    }\n")
	fmt.Fprintf(&sb, "    %s = f->prev;\n", catchTopVar)
	sb.WriteString("    f->exc = exc;\n")
	sb.WriteString("    longjmp(f->buf, 1);\n")
	sb.WriteString("}\n")
	return sb.String()
}

// panicCatchHelperCode - альтернатива panicHelper (builtins_shared.go),
// эмитится вместо неё (полное тело rech_panic), когда программа содержит
// хотя бы одно "поймать (x: Паника)" (ctx.usesPanicCatch - см. прототип
// rech_panic на месте panicHelper в ir_generator.go). В отличие от
// panicHelper (безусловный fprintf+exit) ищет ближайший кадр с
// catch_panics вверх по rech_catch_top - тот же принцип пропуска ЧУЖИХ
// кадров, что и у rech_throw выше, только в обратную сторону (пропускает
// обычные поймать(Тип), а не catch_panics). Сообщение копируется в кучу
// (malloc) ДО longjmp - буфер/литерал, с которым пришёл rech_panic, может
// быть локальной C-переменной вызывающей стороны (см. rech_arg,
// builtins_shared.go), не переживающей размотку стека.
func panicCatchHelperCode() string {
	var sb strings.Builder
	sb.WriteString("static void rech_panic(const char *msg) {\n")
	fmt.Fprintf(&sb, "    %s *f = %s;\n", catchFrameType, catchTopVar)
	sb.WriteString("    while (f != NULL && !f->catch_panics) {\n")
	sb.WriteString("        f = f->prev;\n")
	sb.WriteString("    }\n")
	sb.WriteString("    if (f == NULL) {\n")
	fmt.Fprintf(&sb, "        fprintf(stderr, \"%s%%s\\n\", msg);\n", escapeCString(rtmsg.PanicPrefix))
	sb.WriteString("        rech_trace_print();\n")
	sb.WriteString("        exit(1);\n")
	sb.WriteString("    }\n")
	sb.WriteString("    size_t rech_panic_len = strlen(msg);\n")
	sb.WriteString("    char *rech_panic_copy = malloc(rech_panic_len + 1);\n")
	sb.WriteString("    if (rech_panic_copy == NULL) {\n")
	fmt.Fprintf(&sb, "        fprintf(stderr, \"%s%%s\\n\", msg);\n", escapeCString(rtmsg.PanicPrefix))
	sb.WriteString("        rech_trace_print();\n")
	sb.WriteString("        exit(1);\n")
	sb.WriteString("    }\n")
	sb.WriteString("    memcpy(rech_panic_copy, msg, rech_panic_len + 1);\n")
	fmt.Fprintf(&sb, "    %s = f->prev;\n", catchTopVar)
	sb.WriteString("    f->panic_msg = rech_panic_copy;\n")
	sb.WriteString("    longjmp(f->buf, 1);\n")
	sb.WriteString("}\n")
	return sb.String()
}

// isaTableVar - имя C-массива rech_isa (см. exceptionRuntimeCode/
// generateIRTryStmt/generateIRIsExpr).
const isaTableVar = "rech_isa"

// throwTypeNameExpr - C-выражение, вычисляющее имя фактического типа
// значения code (уже сгенерированного, указатель на исключение) для
// сообщения об ошибке при необработанном исключении/повторном выбросе -
// либо чтение через vt->type_id (если "Исключение" участвует в
// диспетчеризации где-то в программе - см. exceptionsUseDispatch), либо
// статически известное имя типа typeName (единственно возможное в этом
// случае - см. package doc).
func throwTypeNameExpr(ctx *irGenCtx, code, typeName string) string {
	if !ctx.exceptionsUseDispatch() {
		return fmt.Sprintf(`"%s"`, escapeCString(typeName))
	}
	return fmt.Sprintf("%s[((const %s *)(%s))->vt->type_id]", typeNamesVar, vtableHeaderType, code)
}

// typeNamesVar - имя C-массива type_id -> строковое имя типа Rech
// (используется только вместе с exceptionsUseDispatch() - см.
// throwTypeNameExpr/generateIRTryStmt).
const typeNamesVar = "rech_type_names"

// generateIRThrowStmt - "выбросить <выражение>".
func generateIRThrowStmt(sb *strings.Builder, ctx *irGenCtx, s *ir.ThrowStmt, depth int) error {
	ctx.usesExceptions = true
	code, _, err := generateIRExpr(ctx, s.Value)
	if err != nil {
		return err
	}
	nameExpr := throwTypeNameExpr(ctx, code, ir.TypeOf(s.Value).Entity)
	fmt.Fprintf(sb, "%s%s(%s, %s);\n", indent(depth), throwFunc, code, nameExpr)
	return nil
}

// generateIRTryStmt - "попытаться { ... } поймать(<имя>: <Тип>[|<Тип>...]) { ... }".
// Каждый "попытаться" заводит собственный кадр (frameVar, счётчик
// ctx.tryCounter гарантирует уникальное C-имя у вложенных) - см. package
// doc про сам механизм setjmp/longjmp.
func generateIRTryStmt(sb *strings.Builder, ctx *irGenCtx, s *ir.TryStmt, depth int) error {
	if s.NoCatch || len(s.Finally) > 0 {
		return generateIRTryWithFinallyStmt(sb, ctx, s, depth)
	}
	if s.CatchPanic {
		if len(s.CatchTypes) > 0 {
			return generateIRTryCatchMixedStmt(sb, ctx, s, depth)
		}
		return generateIRTryCatchPanicStmt(sb, ctx, s, depth)
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
	fmt.Fprintf(sb, "%s{\n", ind)
	fmt.Fprintf(sb, "%sint %s = rech_trace_top;\n", in1, traceVar)
	// "= {0}" - catch_panics обязан быть гарантированным false у ОБЫЧНОГО
	// кадра поймать(Тип) (см. package doc catchFrameType/rech_panic в
	// panicCatchHelperCode) - без явного нулевого инициализатора он нёс бы
	// неопределённый мусор стека.
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
	fmt.Fprintf(sb, "%srech_trace_top = %s;\n", in2, traceVar)
	fmt.Fprintf(sb, "%svoid *%s_exc = %s.exc;\n", in2, frame, frame)

	catchVarC := fromIRType(s.CatchVarType).cType(ctx)
	catchVar := mangleVar(s.CatchVar)
	writeCatchBody := func(w *strings.Builder, d int) error {
		fmt.Fprintf(w, "%s%s %s = (%s)%s_exc;\n", indent(d), catchVarC, catchVar, catchVarC, frame)
		for _, st := range s.CatchBody {
			if err := generateIRStmt(w, ctx, st, d); err != nil {
				return err
			}
		}
		return nil
	}

	if !ctx.exceptionsUseDispatch() {
		// "Исключение" нигде не участвует в диспетчеризации - в программе
		// физически не может существовать другого исключения, кроме него
		// самого (см. package doc) - совпадение гарантировано статически,
		// без чтения type_id.
		if err := writeCatchBody(sb, depth+2); err != nil {
			return err
		}
		popCleanup()
		fmt.Fprintf(sb, "%s}\n%s}\n", in1, ind)
		return nil
	}

	tid := frame + "_tid"
	fmt.Fprintf(sb, "%sint %s = ((const %s *)%s_exc)->vt->type_id;\n", in2, tid, vtableHeaderType, frame)
	conds := make([]string, len(s.CatchTypes))
	for i, t := range s.CatchTypes {
		id, ok := ctx.excTypeIDs[t]
		if !ok {
			return fmt.Errorf("генерация C из IR: тип %q поймать(...) не найден в таблице rech_isa", t)
		}
		conds[i] = fmt.Sprintf("%s[%s][%d]", isaTableVar, tid, id)
	}
	fmt.Fprintf(sb, "%sif (%s) {\n", in2, strings.Join(conds, " || "))
	if err := writeCatchBody(sb, depth+3); err != nil {
		return err
	}
	fmt.Fprintf(sb, "%s} else {\n", in2)
	fmt.Fprintf(sb, "%s%s(%s_exc, %s[%s]);\n", in3, throwFunc, frame, typeNamesVar, tid)
	fmt.Fprintf(sb, "%s}\n", in2)
	popCleanup()
	fmt.Fprintf(sb, "%s}\n%s}\n", in1, ind)
	return nil
}

// generateIRTryCatchPanicStmt - "попытаться { ... } поймать (<CatchVar>: Паника) { ... }"
// (изоляция паники, см. package doc ir.TryStmt.CatchPanic) -
// отдельная от generateIRTryStmt ветка: кадр не несёт типа для сравнения
// (никакого rech_isa/tid здесь нет вовсе - попадание в "else" уже
// гарантированный матч, см. panicCatchHelperCode - она сама ищет ближайший
// именно такой кадр), CatchVar читается напрямую из frame.panic_msg
// (Строка, уже скопированная в кучу до longjmp).
func generateIRTryCatchPanicStmt(sb *strings.Builder, ctx *irGenCtx, s *ir.TryStmt, depth int) error {
	ctx.usesExceptions = true
	ctx.usesPanicCatch = true
	ctx.tryCounter++
	frame := fmt.Sprintf("rech_frame_%d", ctx.tryCounter)
	ind := indent(depth)
	in1 := indent(depth + 1)
	in2 := indent(depth + 2)

	traceVar := frame + "_trace"
	fmt.Fprintf(sb, "%s{\n", ind)
	fmt.Fprintf(sb, "%sint %s = rech_trace_top;\n", in1, traceVar)
	fmt.Fprintf(sb, "%s%s %s = {0};\n", in1, catchFrameType, frame)
	fmt.Fprintf(sb, "%s%s.prev = %s;\n", in1, frame, catchTopVar)
	fmt.Fprintf(sb, "%s%s.catch_panics = true;\n", in1, frame)
	fmt.Fprintf(sb, "%s%s = &%s;\n", in1, catchTopVar, frame)
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
	fmt.Fprintf(sb, "%srech_trace_top = %s;\n", in2, traceVar)
	catchVarC := fromIRType(s.CatchVarType).cType(ctx)
	catchVar := mangleVar(s.CatchVar)
	fmt.Fprintf(sb, "%s%s %s = %s.panic_msg;\n", in2, catchVarC, catchVar, frame)
	for _, st := range s.CatchBody {
		if err := generateIRStmt(sb, ctx, st, depth+2); err != nil {
			return err
		}
	}
	popCleanup()
	fmt.Fprintf(sb, "%s}\n%s}\n", in1, ind)
	return nil
}

// generateIRTryCatchMixedStmt - "попытаться { ... } поймать (<CatchVar>: Паника|<Тип>[|<Тип>...]) { ... }"
// ("интерфейс Выбрасываемое" - реализовано без него, тем же
// поймать(...) с "|", см. package doc ir.TryStmt.CatchPanic) - ОДИН
// обработчик и на панику, и на перечисленные исключения сразу. Два
// вложенных кадра перехвата (тот же принцип, что и rech_db_run_transaction,
// dbtx.go: внешний - catch_panics, внутренний - обычный кадр, диспетчеризуя
// по s.CatchTypes так же, как generateIRTryStmt выше), сходящиеся в ОДИН
// вызов CatchBody (не два раза подряд - frame_caught/frame_msg копят
// результат любой из двух ветвей, CatchBody генерируется единожды ПОСЛЕ
// обеих). CatchVar везде "Строка" (сообщение) - у исключения это его
// собственное поле "сообщение", у паники - panic_msg как есть.
func generateIRTryCatchMixedStmt(sb *strings.Builder, ctx *irGenCtx, s *ir.TryStmt, depth int) error {
	ctx.usesExceptions = true
	ctx.usesPanicCatch = true
	ctx.tryCounter++
	frame := fmt.Sprintf("rech_frame_%d", ctx.tryCounter)
	ind := indent(depth)
	in1 := indent(depth + 1)
	in2 := indent(depth + 2)
	in3 := indent(depth + 3)
	in4 := indent(depth + 4)

	panicFrame := frame + "_panic"
	excFrame := frame + "_exc"
	excPtr := frame + "_excptr"
	traceVar := frame + "_trace"
	caughtVar := frame + "_caught"
	msgVar := frame + "_msg"

	fmt.Fprintf(sb, "%s{\n", ind)
	fmt.Fprintf(sb, "%sint %s = rech_trace_top;\n", in1, traceVar)
	fmt.Fprintf(sb, "%s%s %s = {0};\n", in1, catchFrameType, panicFrame)
	fmt.Fprintf(sb, "%s%s.prev = %s;\n", in1, panicFrame, catchTopVar)
	fmt.Fprintf(sb, "%s%s.catch_panics = true;\n", in1, panicFrame)
	fmt.Fprintf(sb, "%s%s = &%s;\n", in1, catchTopVar, panicFrame)
	fmt.Fprintf(sb, "%sbool %s = false;\n", in1, caughtVar)
	fmt.Fprintf(sb, "%sconst char *%s = NULL;\n", in1, msgVar)
	popCleanup := pushFrameRestoreCleanup(ctx, panicFrame)
	fmt.Fprintf(sb, "%sif (setjmp(%s.buf) == 0) {\n", in1, panicFrame)
	fmt.Fprintf(sb, "%s%s %s = {0};\n", in2, catchFrameType, excFrame)
	fmt.Fprintf(sb, "%s%s.prev = %s;\n", in2, excFrame, catchTopVar)
	fmt.Fprintf(sb, "%s%s = &%s;\n", in2, catchTopVar, excFrame)
	fmt.Fprintf(sb, "%sif (setjmp(%s.buf) == 0) {\n", in2, excFrame)
	for _, st := range s.Body {
		if err := generateIRStmt(sb, ctx, st, depth+3); err != nil {
			return err
		}
	}
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in3, catchTopVar, excFrame)
	fmt.Fprintf(sb, "%s} else {\n", in2)
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in3, catchTopVar, excFrame)
	fmt.Fprintf(sb, "%srech_trace_top = %s;\n", in3, traceVar)
	fmt.Fprintf(sb, "%svoid *%s = %s.exc;\n", in3, excPtr, excFrame)

	msgField := fmt.Sprintf("((const %s *)%s)->%s", mangleType(systemExceptionEntity), excPtr, mangleField("сообщение"))
	if !ctx.exceptionsUseDispatch() {
		fmt.Fprintf(sb, "%s%s = %s;\n", in3, msgVar, msgField)
		fmt.Fprintf(sb, "%s%s = true;\n", in3, caughtVar)
	} else {
		tid := frame + "_tid"
		fmt.Fprintf(sb, "%sint %s = ((const %s *)%s)->vt->type_id;\n", in3, tid, vtableHeaderType, excPtr)
		conds := make([]string, len(s.CatchTypes))
		for i, t := range s.CatchTypes {
			id, ok := ctx.excTypeIDs[t]
			if !ok {
				return fmt.Errorf("генерация C из IR: тип %q поймать(...) не найден в таблице rech_isa", t)
			}
			conds[i] = fmt.Sprintf("%s[%s][%d]", isaTableVar, tid, id)
		}
		fmt.Fprintf(sb, "%sif (%s) {\n", in3, strings.Join(conds, " || "))
		fmt.Fprintf(sb, "%s%s = %s;\n", in4, msgVar, msgField)
		fmt.Fprintf(sb, "%s%s = true;\n", in4, caughtVar)
		fmt.Fprintf(sb, "%s} else {\n", in3)
		fmt.Fprintf(sb, "%s%s(%s, %s[%s]);\n", in4, throwFunc, excPtr, typeNamesVar, tid)
		fmt.Fprintf(sb, "%s}\n", in3)
	}
	fmt.Fprintf(sb, "%s}\n", in2)
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, panicFrame)
	fmt.Fprintf(sb, "%s} else {\n", in1)
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, panicFrame)
	fmt.Fprintf(sb, "%srech_trace_top = %s;\n", in2, traceVar)
	fmt.Fprintf(sb, "%s%s = %s.panic_msg;\n", in2, msgVar, panicFrame)
	fmt.Fprintf(sb, "%s%s = true;\n", in2, caughtVar)
	fmt.Fprintf(sb, "%s}\n", in1)

	fmt.Fprintf(sb, "%sif (%s) {\n", in1, caughtVar)
	catchVarC := fromIRType(s.CatchVarType).cType(ctx)
	catchVar := mangleVar(s.CatchVar)
	fmt.Fprintf(sb, "%s%s %s = %s;\n", in2, catchVarC, catchVar, msgVar)
	for _, st := range s.CatchBody {
		if err := generateIRStmt(sb, ctx, st, depth+2); err != nil {
			return err
		}
	}
	fmt.Fprintf(sb, "%s}\n", in1)
	popCleanup()
	fmt.Fprintf(sb, "%s}\n", ind)
	return nil
}

// generateIRTryWithFinallyStmt - узел цепочки с "окончательно" (package doc
// ast.TryStmt.Finally/NoCatch) - либо вовсе без "поймать" (s.NoCatch), либо
// один из трёх обычных видов (CatchTypes/CatchPanic/оба сразу) с довеском
// "окончательно". В отличие от трёх функций выше, устанавливает ОБА кадра
// перехвата БЕЗУСЛОВНО, даже если сам узел не просит ловить один из двух
// классов ошибок - "окончательно" обязан увидеть ЛЮБОЙ сквозной проход
// (панику ИЛИ исключение, которые этот конкретный "поймать" не перехватывает)
// и всё равно выполниться ПЕРЕД тем, как пробросить их дальше (frame_outcome
// 2/3 ниже). CatchBody (если есть) генерируется РОВНО ОДИН РАЗ, уже после
// того как обе ветки (паника/исключение) сошлись в frame_outcome - тот же
// приём, что и в generateIRTryCatchMixedStmt, только с добавленным "не
// поймано, но окончательно всё равно нужно выполнить" исходом.
func generateIRTryWithFinallyStmt(sb *strings.Builder, ctx *irGenCtx, s *ir.TryStmt, depth int) error {
	ctx.usesExceptions = true
	ctx.usesPanicCatch = true
	ctx.tryCounter++
	frame := fmt.Sprintf("rech_frame_%d", ctx.tryCounter)
	ind := indent(depth)
	in1 := indent(depth + 1)
	in2 := indent(depth + 2)
	in3 := indent(depth + 3)
	in4 := indent(depth + 4)

	panicFrame := frame + "_panic"
	excFrame := frame + "_exc"
	excPtr := frame + "_excptr"
	traceVar := frame + "_trace"
	outcomeVar := frame + "_outcome" // 0 обычное завершение, 1 - свой поймать сработал, 2 - паника прошла насквозь, 3 - исключение прошло насквозь (не подошёл ни один тип)
	msgVar := frame + "_msg"

	fmt.Fprintf(sb, "%s{\n", ind)
	fmt.Fprintf(sb, "%sint %s = rech_trace_top;\n", in1, traceVar)
	fmt.Fprintf(sb, "%s%s %s = {0};\n", in1, catchFrameType, panicFrame)
	fmt.Fprintf(sb, "%s%s.prev = %s;\n", in1, panicFrame, catchTopVar)
	fmt.Fprintf(sb, "%s%s.catch_panics = true;\n", in1, panicFrame)
	fmt.Fprintf(sb, "%s%s = &%s;\n", in1, catchTopVar, panicFrame)
	fmt.Fprintf(sb, "%sint %s = 0;\n", in1, outcomeVar)
	fmt.Fprintf(sb, "%sconst char *%s = NULL;\n", in1, msgVar)
	fmt.Fprintf(sb, "%svoid *%s = NULL;\n", in1, excPtr)

	// prevStack/action - см. package doc cleanupAction: раннее "вернуть"
	// внутри Body/CatchBody обязано и восстановить rech_catch_top (одной
	// строкой, до panicFrame.prev - пропускает разом оба кадра, excFrame
	// вложен ПОСЛЕ panicFrame, восстанавливать его отдельно не нужно), и
	// прогнать "окончательно" (temp-переключение actx.cleanupStack на
	// prevStack на время его генерации - вложенное "вернуть" внутри самого
	// Finally обязано видеть только ВНЕШНИЕ по отношению к этому узлу
	// кадры, не крутиться на этом же действии повторно).
	prevStack := ctx.cleanupStack
	action := cleanupAction(func(asb *strings.Builder, actx *irGenCtx, adepth int) error {
		aind := indent(adepth)
		fmt.Fprintf(asb, "%s%s = %s.prev;\n", aind, catchTopVar, panicFrame)
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

	fmt.Fprintf(sb, "%sif (setjmp(%s.buf) == 0) {\n", in1, panicFrame)
	fmt.Fprintf(sb, "%s%s %s = {0};\n", in2, catchFrameType, excFrame)
	fmt.Fprintf(sb, "%s%s.prev = %s;\n", in2, excFrame, catchTopVar)
	fmt.Fprintf(sb, "%s%s = &%s;\n", in2, catchTopVar, excFrame)
	fmt.Fprintf(sb, "%sif (setjmp(%s.buf) == 0) {\n", in2, excFrame)
	for _, st := range s.Body {
		if err := generateIRStmt(sb, ctx, st, depth+3); err != nil {
			return err
		}
	}
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in3, catchTopVar, excFrame)
	fmt.Fprintf(sb, "%s} else {\n", in2)
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in3, catchTopVar, excFrame)
	fmt.Fprintf(sb, "%srech_trace_top = %s;\n", in3, traceVar)
	fmt.Fprintf(sb, "%s%s = %s.exc;\n", in3, excPtr, excFrame)
	fmt.Fprintf(sb, "%s%s = ((const %s *)%s)->%s;\n", in3, msgVar, mangleType(systemExceptionEntity), excPtr, mangleField("сообщение"))

	switch {
	case s.NoCatch || len(s.CatchTypes) == 0:
		// Ни одного своего типа исключения (NoCatch - вовсе без "поймать",
		// либо чистый "поймать(Паника)" с довеском "окончательно") -
		// исключение всегда проходит насквозь.
		fmt.Fprintf(sb, "%s%s = 3;\n", in3, outcomeVar)
	case !ctx.exceptionsUseDispatch():
		fmt.Fprintf(sb, "%s%s = 1;\n", in3, outcomeVar)
	default:
		tid := frame + "_tid"
		fmt.Fprintf(sb, "%sint %s = ((const %s *)%s)->vt->type_id;\n", in3, tid, vtableHeaderType, excPtr)
		conds := make([]string, len(s.CatchTypes))
		for i, t := range s.CatchTypes {
			id, ok := ctx.excTypeIDs[t]
			if !ok {
				return fmt.Errorf("генерация C из IR: тип %q поймать(...) не найден в таблице rech_isa", t)
			}
			conds[i] = fmt.Sprintf("%s[%s][%d]", isaTableVar, tid, id)
		}
		fmt.Fprintf(sb, "%sif (%s) {\n", in3, strings.Join(conds, " || "))
		fmt.Fprintf(sb, "%s%s = 1;\n", in4, outcomeVar)
		fmt.Fprintf(sb, "%s} else {\n", in3)
		fmt.Fprintf(sb, "%s%s = 3;\n", in4, outcomeVar)
		fmt.Fprintf(sb, "%s}\n", in3)
	}
	fmt.Fprintf(sb, "%s}\n", in2)
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, panicFrame)
	fmt.Fprintf(sb, "%s} else {\n", in1)
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, panicFrame)
	fmt.Fprintf(sb, "%srech_trace_top = %s;\n", in2, traceVar)
	fmt.Fprintf(sb, "%s%s = %s.panic_msg;\n", in2, msgVar, panicFrame)
	if s.CatchPanic {
		fmt.Fprintf(sb, "%s%s = 1;\n", in2, outcomeVar)
	} else {
		fmt.Fprintf(sb, "%s%s = 2;\n", in2, outcomeVar)
	}
	fmt.Fprintf(sb, "%s}\n", in1)

	ctx.cleanupStack = prevStack // Body/CatchBody сгенерированы - см. package doc выше.

	if !s.NoCatch {
		fmt.Fprintf(sb, "%sif (%s == 1) {\n", in1, outcomeVar)
		catchVarC := fromIRType(s.CatchVarType).cType(ctx)
		catchVar := mangleVar(s.CatchVar)
		if s.CatchVarType.Kind == ir.KindString {
			fmt.Fprintf(sb, "%s%s %s = %s;\n", in2, catchVarC, catchVar, msgVar)
		} else {
			fmt.Fprintf(sb, "%s%s %s = (%s)%s;\n", in2, catchVarC, catchVar, catchVarC, excPtr)
		}
		for _, st := range s.CatchBody {
			if err := generateIRStmt(sb, ctx, st, depth+2); err != nil {
				return err
			}
		}
		fmt.Fprintf(sb, "%s}\n", in1)
	}

	for _, st := range s.Finally {
		if err := generateIRStmt(sb, ctx, st, depth+1); err != nil {
			return err
		}
	}

	fmt.Fprintf(sb, "%sif (%s == 2) {\n", in1, outcomeVar)
	fmt.Fprintf(sb, "%srech_panic(%s);\n", in2, msgVar)
	fmt.Fprintf(sb, "%s} else if (%s == 3) {\n", in1, outcomeVar)
	fmt.Fprintf(sb, "%s%s(%s, %s);\n", in2, throwFunc, excPtr, throwTypeNameExpr(ctx, excPtr, systemExceptionEntity))
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
