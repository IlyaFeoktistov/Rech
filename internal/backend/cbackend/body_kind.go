package cbackend

import "strings"

// bodyKind - чем C-функция текущего тела отличается от обычной функции/
// метода Rech ("int" с техническим "return 0", собственный кадр стека
// вызовов). Нулевое значение - обычная функция/метод.
type bodyKind struct {
	// void - C-функция "void" (тело конструктора, лямбда без результата):
	// "вернуть" без значения - "return;", не "return 0;".
	void bool
	// noTraceFrame - у тела нет собственного кадра стека вызовов (лямбда,
	// см. package doc trace.go): "вернуть" не снимает кадр - он принадлежит
	// вызывающей функции.
	noTraceFrame bool
}

// writeTracePop - "rech_trace_pop();" с отступом ind перед выходом из
// тела, если у тела есть собственный кадр (см. bodyKind.noTraceFrame).
func (b bodyKind) writeTracePop(sb *strings.Builder, ind string) {
	if !b.noTraceFrame {
		sb.WriteString(ind + tracePop)
	}
}

// bareReturn - C-инструкция для "вернуть" без значения: "return;" в
// "void"-теле, иначе технический "return 0;" (см. cReturnTypeFor).
func (b bodyKind) bareReturn() string {
	if b.void {
		return "return;"
	}
	return "return 0;"
}
