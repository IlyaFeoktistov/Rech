// Стек вызовов Rech-функций/методов при аварийном завершении (rech_panic) и
// необработанном исключении (rech_throw, exceptions.go). Массив
// фиксированного размера (не связный список, как rech_catch_frame) - кадр
// нужен на КАЖДЫЙ вызов функции/метода программы, счётчик индекса дешевле
// выделения/освобождения памяти на каждый вызов. RECH_THREAD_LOCAL (не C11
// _Thread_local, см. exceptions.go) - каждый поток ОС ("запустить") ведёт
// свой собственный стек, не общий на процесс.
//
// Печатается вслед за самим сообщением об ошибке - имя функции/метода и
// номер строки, где выполнение этого уровня остановилось (сам вызов
// вложенного уровня или прямая паника), от места паники вверх до "главная" -
// см. rtmsg.TraceHeader/TraceFrameFormat (общий текст с internal/interp).
//
// Гранулярность - по инструкции (rech_trace_line эмитится перед КАЖДОЙ
// инструкцией верхнего уровня тела функции/метода, см. stmtLine/
// generateIRStmt), не по каждому вложенному вызову внутри составного
// выражения - для типичного стиля Rech (один вызов на инструкцию) этого
// достаточно, чтобы номер строки в кадре совпадал с местом настоящего
// вызова.
//
// Не покрыто (осознанно): вызов через значение типа "Вызываемый"
// (лямбда/"&имя") не заводит отдельный кадр - паника внутри такого вызова
// относится на кадр функции, которая его сделала, на строке самого вызова.
package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// traceMaxFrames - размер массива кадров на поток. Переполнение (глубокая
// рекурсия) не аварийно само по себе - лишние push просто не пишутся (см.
// traceHelper, rech_trace_push), rech_trace_print в этом случае печатает
// только сохранившиеся кадры и добавляет rtmsg.TraceTruncated.
const traceMaxFrames = 256

// traceHelper - рантайм стека вызовов, эмитится безусловно (как
// consoleInitHelper/argsGlobals) - дешёвая пара присваиваний на вызов, без
// дополнительных заголовков/библиотек, гейтить порогом "используется ли
// стек вызовов реально" не стоит (см. package doc).
var traceHelper = fmt.Sprintf(`typedef struct {
    const char *name;
    int line;
} rech_trace_frame;
static RECH_THREAD_LOCAL rech_trace_frame rech_trace[%[1]d];
static RECH_THREAD_LOCAL int rech_trace_top = 0;

static void rech_trace_push(const char *name) {
    if (rech_trace_top < %[1]d) {
        rech_trace[rech_trace_top].name = name;
        rech_trace[rech_trace_top].line = 0;
    }
    rech_trace_top++;
}

static void rech_trace_pop(void) {
    if (rech_trace_top > 0) {
        rech_trace_top--;
    }
}

static void rech_trace_line(int line) {
    if (rech_trace_top > 0 && rech_trace_top <= %[1]d) {
        rech_trace[rech_trace_top - 1].line = line;
    }
}

static void rech_trace_print(void) {
    int n = rech_trace_top < %[1]d ? rech_trace_top : %[1]d;
    fprintf(stderr, "%[2]s\n");
    for (int i = n - 1; i >= 0; i--) {
        fprintf(stderr, "%[3]s\n", rech_trace[i].name, rech_trace[i].line);
    }
    if (rech_trace_top > %[1]d) {
        fprintf(stderr, "%[4]s\n");
    }
}
`, traceMaxFrames, escapeCString(rtmsg.TraceHeader), rtmsg.TraceFrameFormat, escapeCString(rtmsg.TraceTruncated))

// tracePush - "rech_trace_push("<displayName>");" в начале сгенерированной
// функции/метода (см. generateIRFunction/generateIRMethod/
// generateIREntityCtorInit).
func tracePush(displayName string) string {
	return fmt.Sprintf("rech_trace_push(\"%s\");\n", escapeCString(displayName))
}

// tracePop - "rech_trace_pop();" перед каждым выходом из функции/метода
// (явный "вернуть" - см. case *ir.ReturnStmt в generateIRStmt - и неявный
// конец тела без объявленного результата).
const tracePop = "rech_trace_pop();\n"

// returnTmpVar - имя временной C-переменной, в которую "вернуть <выражение>"
// сохраняет результат ДО rech_trace_pop() (см. case *ir.ReturnStmt) - кадр
// функции должен оставаться на стеке, пока вычисляется само выражение
// (может содержать вложенный вызов, который паникует).
const returnTmpVar = "rech_ret_tmp"

// stmtLine - номер исходной строки s, если её вид несёт Line (см. package
// doc ir.Stmt в internal/ir/stmt.go), иначе 0 (rech_trace_line не эмитится -
// см. generateIRStmt). Управляющие конструкции (IfStmt/WhileStmt/...) несут
// номер строки самого заголовка ("если <условие>" и т. п.) - строка внутри
// тела перезаписывает её на следующей же вложенной инструкции.
func stmtLine(s ir.Stmt) int {
	switch s := s.(type) {
	case *ir.PrintStmt:
		return s.Line
	case *ir.ShowStmt:
		return s.Line
	case *ir.VarDecl:
		return s.Line
	case *ir.Assign:
		return s.Line
	case *ir.FieldAssign:
		return s.Line
	case *ir.ParentCallStmt:
		return s.Line
	case *ir.AssertStmt:
		return s.Line
	case *ir.IfStmt:
		return s.Line
	case *ir.WhileStmt:
		return s.Line
	case *ir.ForStmt:
		return s.Line
	case *ir.ForEachStmt:
		return s.Line
	case *ir.ReturnStmt:
		return s.Line
	case *ir.ExprStmt:
		return s.Line
	case *ir.ThrowStmt:
		return s.Line
	default:
		return 0
	}
}
