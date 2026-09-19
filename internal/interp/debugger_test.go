package interp

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

const debugTestFile = "test.rech"

// irProgForDebug - как runSrc (interp_test.go), но возвращает *ir.Program,
// не исполняет - и проставляет File (ast.StampFile), как это в реальном
// пайплайне делает module.loader.load/compiler.parseFileRaw ДО
// semantic.Analyze - без этого у всех функций/методов File == "", и точки
// останова были бы неотличимы по файлу.
func irProgForDebug(t *testing.T, src string) *ir.Program {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	ast.StampFile(prog, debugTestFile)
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		t.Fatalf("семантический анализ: %v", err)
	}
	return irProg
}

// waitEvent ждёт следующее событие из d.Events() не дольше timeout - тест
// не должен зависнуть навечно, если в отладчике логическая ошибка (поток
// никогда не остановился/никогда не разрешился).
func waitEvent(t *testing.T, d *Debugger, timeout time.Duration) DebugEvent {
	t.Helper()
	select {
	case e := <-d.Events():
		return e
	case <-time.After(timeout):
		t.Fatalf("не дождались события отладчика за %s", timeout)
		return DebugEvent{}
	}
}

// waitStopped ждёт событие "stopped" именно на threadID, пропуская мимо
// "thread"-события других потоков (у теста с "запустить" оба потока шлют
// свои события вперемешку).
func waitStopped(t *testing.T, d *Debugger, threadID int, timeout time.Duration) DebugEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		e := waitEvent(t, d, timeout)
		if e.Kind == "stopped" && e.ThreadID == threadID {
			return e
		}
	}
	t.Fatalf("не дождались stopped для потока %d", threadID)
	return DebugEvent{}
}

func TestDebuggerBreakpointStopsAndReportsStack(t *testing.T) {
	prog := irProgForDebug(t, `
функция вложенная(число: Целое): Целое
{
    вернуть число * 2
}

функция главная
{
    переменная х = вложенная(21)
    вывести `+"`${"+`х}\n`+"`"+`
}
`)
	d, err := NewDebugger(prog, &bytes.Buffer{}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("NewDebugger: %v", err)
	}
	d.SetBreakpoints(debugTestFile, []int{4})
	d.Start(false)

	ev := waitStopped(t, d, 1, 2*time.Second)
	if ev.Reason != "breakpoint" {
		t.Fatalf("ожидалась причина остановки breakpoint, получено %q", ev.Reason)
	}
	frames := d.StackTrace(1)
	if len(frames) != 2 {
		t.Fatalf("ожидалось 2 кадра (вложенная, главная), получено %d: %+v", len(frames), frames)
	}
	if frames[0].Name != "вложенная" || frames[0].Line != 4 || frames[0].File != debugTestFile {
		t.Fatalf("неожиданный верхний кадр: %+v", frames[0])
	}
	if frames[1].Name != "главная" {
		t.Fatalf("неожиданный второй кадр: %+v", frames[1])
	}

	scopes := d.Scopes(frames[0].ID)
	if len(scopes) != 1 || scopes[0].Name != "Локальные" {
		t.Fatalf("ожидалась ровно одна область \"Локальные\" (свободная функция), получено %+v", scopes)
	}
	vars := d.Variables(scopes[0].VariablesReference)
	if len(vars) != 1 || vars[0].Name != "число" || vars[0].Value != "21" {
		t.Fatalf("ожидалась переменная число=21, получено %+v", vars)
	}

	d.Continue(1)
	ev = waitEvent(t, d, 2*time.Second)
	for ev.Kind != "terminated" {
		ev = waitEvent(t, d, 2*time.Second)
	}
	if d.Err() != nil {
		t.Fatalf("неожиданная ошибка исполнения: %v", d.Err())
	}
}

func TestDebuggerStepOverDoesNotEnterCall(t *testing.T) {
	prog := irProgForDebug(t, `
функция вспомогательная()
{
    вывести "внутри\n"
}

функция главная
{
    вывести "перед\n"
    вспомогательная()
    вывести "после\n"
}
`)
	d, err := NewDebugger(prog, &bytes.Buffer{}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("NewDebugger: %v", err)
	}
	d.SetBreakpoints(debugTestFile, []int{9})
	d.Start(false)

	waitStopped(t, d, 1, 2*time.Second)
	frames := d.StackTrace(1)
	if len(frames) != 1 || frames[0].Line != 9 {
		t.Fatalf("ожидалась остановка на строке 9 в главная, получено %+v", frames)
	}

	d.Next(1) // строка 10 - вызов вспомогательная(), шаг НЕ должен зайти внутрь
	ev := waitStopped(t, d, 1, 2*time.Second)
	if ev.Reason != "step" {
		t.Fatalf("ожидалась причина step, получено %q", ev.Reason)
	}
	frames = d.StackTrace(1)
	if len(frames) != 1 || frames[0].Name != "главная" || frames[0].Line != 10 {
		t.Fatalf("ожидалась остановка на строке 10 всё ещё в главная (не зашли внутрь вызова), получено %+v", frames)
	}

	d.Continue(1)
	for {
		ev = waitEvent(t, d, 2*time.Second)
		if ev.Kind == "terminated" {
			break
		}
	}
}

func TestDebuggerStepIntoEntersCall(t *testing.T) {
	prog := irProgForDebug(t, `
функция вспомогательная()
{
    вывести "внутри\n"
}

функция главная
{
    вспомогательная()
    вывести "после\n"
}
`)
	d, err := NewDebugger(prog, &bytes.Buffer{}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("NewDebugger: %v", err)
	}
	d.SetBreakpoints(debugTestFile, []int{9})
	d.Start(false)

	waitStopped(t, d, 1, 2*time.Second)
	d.StepIn(1)
	waitStopped(t, d, 1, 2*time.Second)
	frames := d.StackTrace(1)
	if len(frames) != 2 || frames[0].Name != "вспомогательная" || frames[0].Line != 4 {
		t.Fatalf("ожидалась остановка внутри вспомогательная на строке 4, получено %+v", frames)
	}

	d.StepOut(1)
	waitStopped(t, d, 1, 2*time.Second)
	frames = d.StackTrace(1)
	if len(frames) != 1 || frames[0].Name != "главная" || frames[0].Line != 10 {
		t.Fatalf("ожидалась остановка сразу после возврата, на строке 10 в главная, получено %+v", frames)
	}

	d.Continue(1)
	for {
		ev := waitEvent(t, d, 2*time.Second)
		if ev.Kind == "terminated" {
			break
		}
	}
}

func TestDebuggerLaunchedTaskGetsOwnThread(t *testing.T) {
	prog := irProgForDebug(t, `
функция задача(): Целое
{
    вернуть 1 + 1
}

функция главная
{
    переменная т = запустить задача()
    вывести `+"`${"+`дождаться т}\n`+"`"+`
}
`)
	d, err := NewDebugger(prog, &bytes.Buffer{}, strings.NewReader(""))
	if err != nil {
		t.Fatalf("NewDebugger: %v", err)
	}
	d.SetBreakpoints(debugTestFile, []int{4})
	d.Start(false)

	// Первое "thread"-событие - поток 1 (главная), стартующий вместе с
	// Start; второе - поток 2 (задача), заведённый "запустить".
	var taskThreadID int
	for taskThreadID == 0 {
		e := waitEvent(t, d, 2*time.Second)
		if e.Kind == "thread" && e.Reason == "started" && e.ThreadID != 1 {
			taskThreadID = e.ThreadID
		}
	}

	ev := waitStopped(t, d, taskThreadID, 2*time.Second)
	if ev.Reason != "breakpoint" {
		t.Fatalf("ожидалась остановка задачи на точке останова, получено %q", ev.Reason)
	}
	threads := d.Threads()
	if len(threads) != 2 {
		t.Fatalf("ожидалось 2 активных потока (главная + задача), получено %+v", threads)
	}

	d.Continue(taskThreadID)
	for {
		e := waitEvent(t, d, 2*time.Second)
		if e.Kind == "terminated" {
			break
		}
	}
}
