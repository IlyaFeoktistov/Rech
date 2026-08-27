// Стек вызовов Rech-функций/методов при аварийном завершении (runtimeError)
// и необработанном исключении (thrownException) - тот же формат сообщения,
// что и cbackend (internal/backend/cbackend/trace.go, rtmsg.TraceHeader/
// TraceFrameFormat).
//
// В отличие от cbackend (тред-локальный массив фиксированного размера) -
// накапливается по ходу разматывания самого Go-стека: callFunction/
// callMethod (interp.go) - единственные точки, где заводится и читается
// curLine (см. callCtx) одной активации вызова, каждая перехватывает
// панику/thrownException, проходящую через неё, добавляет СВОЙ кадр (имя +
// строка, на которой эта активация остановилась) и пробрасывает дальше -
// то же самое "снизу вверх", что и push/pop у cbackend, но без явного
// стека - опирается на то, что вызов callFunction/callMethod в Go 1:1
// соответствует вызову функции/метода Rech. Без своего предела глубины (в
// отличие от cbackend, см. traceMaxFrames) - обычный растущий срез Go,
// ограничен только Go-стеком горутины; при переполнении cbackend'овского
// массива в редком случае глубокой рекурсии вывод двух backend'ов может
// разойтись (см. rtmsg.TraceTruncated).
//
// Гранулярность - по инструкции (execStmt обновляет *cc.curLine перед
// исполнением каждой инструкции верхнего уровня тела, см. stmtLine), не по
// каждому вложенному вызову внутри составного выражения - тот же принцип,
// что и у cbackend (trace.go, package doc).
//
// Вызов через значение типа "Вызываемый" (лямбда/"&имя") не заводит
// отдельный кадр (evalLambdaExpr исполняет тело напрямую через execBlock,
// не через callFunction/callMethod) - паника внутри относится на кадр
// функции, которая сделала этот вызов, на строке самого вызова - то же
// осознанное ограничение, что и в cbackend.
package interp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// traceFrame - один уровень стека вызовов: имя функции/метода ("Тип.метод"
// у методов, "Тип.конструктор" у явного конструктора - тот же формат, что и
// cbackend.tracePush) и строка, на которой этот уровень остановился.
type traceFrame struct {
	name string
	line int
}

// traceSuffix - текст стека вызовов, добавляемый после самого сообщения об
// ошибке (см. runtimeError.Error/thrownException.Error) - пусто, если trace
// пуст (внутренняя ошибка интерпретатора без прохода через callFunction/
// callMethod вообще, см. Run). frames накапливаются от места паники к
// "главная" (см. package doc) - печатаются в этом же порядке, самый
// глубокий уровень первым.
func traceSuffix(frames []traceFrame) string {
	if len(frames) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(rtmsg.TraceHeader)
	for _, f := range frames {
		sb.WriteString("\n")
		sb.WriteString(rtmsg.TraceFrameLine(f.name, f.line))
	}
	return sb.String()
}

// stmtLine - номер исходной строки s, если её вид несёт Line (см. package
// doc ir.Stmt в internal/ir/stmt.go), иначе 0 (execStmt не обновляет
// *cc.curLine - см. package doc). Тот же список видов, что и у
// cbackend.stmtLine.
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
