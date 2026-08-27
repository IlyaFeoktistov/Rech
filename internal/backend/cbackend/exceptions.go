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
	fmt.Fprintf(&sb, "} %s;\n", catchFrameType)
	// rech_catch_top - per-поток (не общий на процесс): без этого
	// "попытаться" в одном потоке ОС (шаг 15) видело бы/портило кадры
	// другого - __declspec(thread)/__thread - переносимая пара для этого
	// (не C11 _Thread_local - не на всех тулчейнах, которые нужно
	// поддерживать, есть <threads.h>). Тред-локальность не вредит и
	// однопоточной программе (то же поведение, что и у обычного static) -
	// не гейтится отдельно от usesExceptions самим usesThreads.
	sb.WriteString("#if defined(_WIN32)\n#define RECH_THREAD_LOCAL __declspec(thread)\n#else\n#define RECH_THREAD_LOCAL __thread\n#endif\n")
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
	fmt.Fprintf(&sb, "    if (%s == NULL) {\n", catchTopVar)
	fmt.Fprintf(&sb, "        fprintf(stderr, \"%s\\n\", type_name, %s);\n", escapeCString(template), msgField)
	sb.WriteString("        exit(1);\n")
	sb.WriteString("    }\n")
	fmt.Fprintf(&sb, "    %s *f = %s;\n", catchFrameType, catchTopVar)
	fmt.Fprintf(&sb, "    %s = f->prev;\n", catchTopVar)
	sb.WriteString("    f->exc = exc;\n")
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
	ctx.usesExceptions = true
	ctx.tryCounter++
	frame := fmt.Sprintf("rech_frame_%d", ctx.tryCounter)
	ind := indent(depth)
	in1 := indent(depth + 1)
	in2 := indent(depth + 2)
	in3 := indent(depth + 3)

	fmt.Fprintf(sb, "%s{\n", ind)
	fmt.Fprintf(sb, "%s%s %s;\n", in1, catchFrameType, frame)
	fmt.Fprintf(sb, "%s%s.prev = %s;\n", in1, frame, catchTopVar)
	fmt.Fprintf(sb, "%s%s = &%s;\n", in1, catchTopVar, frame)
	fmt.Fprintf(sb, "%sif (setjmp(%s.buf) == 0) {\n", in1, frame)
	for _, st := range s.Body {
		if err := generateIRStmt(sb, ctx, st, depth+2); err != nil {
			return err
		}
	}
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, frame)
	fmt.Fprintf(sb, "%s} else {\n", in1)
	fmt.Fprintf(sb, "%s%s = %s.prev;\n", in2, catchTopVar, frame)
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
	fmt.Fprintf(sb, "%s}\n%s}\n", in1, ind)
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
