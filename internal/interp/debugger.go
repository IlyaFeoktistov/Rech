// Отладчик (шаг 19, "точки останова") - пошаговое, останавливаемое
// исполнение программы через internal/interp, без какого-либо знания о DAP/
// JSON (см. internal/dap - транспорт, internal/debugadapter - перевод
// между ними) - Debugger возвращает и принимает простые Go-структуры,
// внешний слой сам решает, как завернуть их в конкретный протокол.
//
// Модель потоков: "главная" - поток 1, каждое "запустить" (шаг 15) - новый
// независимый поток (см. concurrency.go, evalLaunchExpr) - у каждого свой
// стек кадров и свой режим шага, точки останова же общие для всей
// программы (внутри одного файла - то, что видит VS Code, у которого нет
// понятия "точка останова только в этом потоке"). Каждый вызов функции/
// метода/явного конструктора заводит кадр (dbgFrame, push/pop в
// callFunction/callMethod, interp.go) - лямбда/"&имя" кадр не заводит (тот
// же принцип, что и в trace.go), точки останова внутри нею продолжают
// работать, относясь к кадру вызывающей стороны (тот же файл).
//
// Пауза реализована без явного планировщика: onStatement, вызываемый
// execStmt перед КАЖДОЙ инструкцией, блокируется на канале, если решил
// остановиться - сама горутина Go, исполняющая программу, и есть точка
// остановки, продолжает исполнение сразу после закрытия канала (Continue/
// Next/StepIn/StepOut).
package interp

import (
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"sync"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// ThreadInfo/FrameInfo/Scope/Variable/DebugEvent - DAP-независимое описание
// состояния отладки, которое переводит в протокол internal/debugadapter.
type ThreadInfo struct {
	ID   int
	Name string
}

// FrameInfo - один кадр стека вызовов, ID - для последующих Scopes(ID).
type FrameInfo struct {
	ID   int
	Name string
	File string
	Line int
}

// Scope - "Локальные"/"этот" в кадре, VariablesReference - для Variables(...).
type Scope struct {
	Name               string
	VariablesReference int
}

// Variable - VariablesReference == 0 у примитива (нечего разворачивать).
type Variable struct {
	Name               string
	Value              string
	Type               string
	VariablesReference int
}

// DebugEvent - то, что Debugger сообщает наружу асинхронно (не в ответ на
// конкретный запрос) - читается через Events().
type DebugEvent struct {
	Kind     string // "stopped" | "thread" | "exited" | "terminated"
	ThreadID int
	Reason   string // "breakpoint" | "step" | "pause" | "started" | "exited"
	ExitCode int
}

// stepMode - режим продолжения потока после Continue/Next/StepIn/StepOut.
type stepMode int

const (
	modeRunning  stepMode = iota // свободный бег - останавливается только на точке останова/по Pause
	modeStepIn                   // остановиться на следующей же инструкции, на любой глубине
	modeStepOver                 // остановиться на следующей инструкции НЕ глубже targetDepth
	modeStepOut                  // остановиться, когда глубина стала <= targetDepth (обычно depth-1)
)

// dbgFrame - один кадр стека вызовов потока - line/env мутируются
// execStmt-ом (через onStatement) по мере исполнения этой активации,
// остальные поля постоянны с момента push.
type dbgFrame struct {
	id   int
	name string
	file string
	line int
	env  *Env
	this *entityObj // nil у свободной функции
}

// dbgThread - независимый поток исполнения (см. package doc). mu защищает
// весь изменяемый набор полей - frames мутируется push/pop (см. interp.go)
// одной и той же горутиной, но читается запросами DAP (StackTrace/Scopes/
// Variables) с ДРУГОЙ горутины (та, что обслуживает internal/dap.Server), в
// то время как этот поток либо исполняется, либо стоит на паузе.
type dbgThread struct {
	id   int
	name string
	dbg  *Debugger

	mu          sync.Mutex
	frames      []*dbgFrame
	mode        stepMode
	targetDepth int
	pauseReq    bool
	stopped     bool
	resumeCh    chan struct{}
}

// push/pushMethod/pop - nil-получатель безопасен (обычный Run/rech test не
// заводит поток вовсе, см. callCtx.dbg) - тогда это буквально ничего не
// делает, без единой лишней проверки в вызывающем коде.
func (t *dbgThread) push(name, file string) {
	if t == nil {
		return
	}
	f := &dbgFrame{id: t.dbg.nextFrameID(), name: name, file: file}
	t.dbg.registerFrame(f)
	t.mu.Lock()
	t.frames = append(t.frames, f)
	t.mu.Unlock()
}

func (t *dbgThread) pushMethod(name, file string, this *entityObj) {
	if t == nil {
		return
	}
	f := &dbgFrame{id: t.dbg.nextFrameID(), name: name, file: file, this: this}
	t.dbg.registerFrame(f)
	t.mu.Lock()
	t.frames = append(t.frames, f)
	t.mu.Unlock()
}

func (t *dbgThread) pop() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if len(t.frames) > 0 {
		t.frames = t.frames[:len(t.frames)-1]
	}
	t.mu.Unlock()
}

// onStatement - вызывается execStmt (exec.go) перед КАЖДОЙ инструкцией
// верхнего уровня тела (та же гранулярность, что и у trace.go) - обновляет
// текущий кадр и, если решил остановиться (точка останова/выполнен шаг/
// запрошена пауза), блокируется до Continue/Next/StepIn/StepOut. Условие
// остановки читается БЕЗ удержания t.mu во время обращения к
// t.dbg.hasBreakpoint (свой мьютекс) - иначе параллельные запросы других
// потоков рисковали бы инверсией порядка блокировок.
func (t *dbgThread) onStatement(env *Env, line int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if len(t.frames) == 0 {
		t.mu.Unlock()
		return
	}
	top := t.frames[len(t.frames)-1]
	top.line = line
	top.env = env
	file := top.file
	depth := len(t.frames)
	mode, targetDepth, pauseReq := t.mode, t.targetDepth, t.pauseReq
	t.mu.Unlock()

	reason := ""
	switch {
	case pauseReq:
		reason = "pause"
	case mode == modeStepIn:
		reason = "step"
	case (mode == modeStepOver || mode == modeStepOut) && depth <= targetDepth:
		reason = "step"
	case t.dbg.hasBreakpoint(file, line):
		reason = "breakpoint"
	}
	if reason == "" {
		return
	}

	t.mu.Lock()
	t.mode = modeRunning
	t.pauseReq = false
	t.stopped = true
	resumeCh := make(chan struct{})
	t.resumeCh = resumeCh
	t.mu.Unlock()

	t.dbg.sendEvent(DebugEvent{Kind: "stopped", ThreadID: t.id, Reason: reason})
	<-resumeCh
}

// varHandle - то, что стоит за одним DAP variablesReference: локальные
// переменные кадра (окружение целиком, включая внешние блоки того же
// вызова), псевдо-поле "этот", либо конкретное развёртываемое значение
// (сущность/generic-коллекция).
type varHandle struct {
	env  *Env
	obj  *entityObj
	val  Value
	kind byte // 'l' - locals, 't' - this, 'v' - value
}

// Debugger управляет одним запуском программы под отладкой - см. package
// doc. Один Debugger - один запуск ("rech debug" исполняет ровно одну
// программу за сессию, как и rech run).
type Debugger struct {
	in   *Interp
	main *ir.Function

	mu           sync.Mutex
	threads      map[int]*dbgThread
	threadSeq    int
	breakpoints  map[string]map[int]bool
	frameSeq     int
	frameHandles map[int]*dbgFrame
	varSeq       int
	varHandles   map[int]varHandle
	exitErr      error

	events chan DebugEvent
}

// NewDebugger готовит prog к пошаговому исполнению (см. Start) - out/stdin
// те же, что и у Run (вывести/прочитать_строку и т. п.). Возвращает ошибку
// в тех же случаях, что и Run (внешние функции, нет "главная") - до
// исполнения программа не должна начинать выполняться.
func NewDebugger(prog *ir.Program, out io.Writer, stdin io.Reader) (*Debugger, error) {
	in, main, err := newInterp(prog, out, stdin)
	if err != nil {
		return nil, err
	}
	d := &Debugger{
		in:           in,
		main:         main,
		threads:      map[int]*dbgThread{},
		breakpoints:  map[string]map[int]bool{},
		frameHandles: map[int]*dbgFrame{},
		varHandles:   map[int]varHandle{},
		events:       make(chan DebugEvent, 64),
	}
	in.debugger = d
	return d, nil
}

// Events - канал событий ("stopped"/"thread"/"exited"/"terminated"),
// читать до тех пор, пока не пришло "terminated" - после него Debugger
// больше ничего не пришлёт.
func (d *Debugger) Events() <-chan DebugEvent {
	return d.events
}

func (d *Debugger) sendEvent(e DebugEvent) {
	d.events <- e
}

// Start запускает "главная" на отдельной горутине - поток 1. stopOnEntry -
// остановиться на самой первой исполненной инструкции "главная" (DAP-опция
// launch-запроса), иначе программа бежит свободно до первой точки
// останова/конца.
func (d *Debugger) Start(stopOnEntry bool) {
	thread := d.newThread("главная")
	if stopOnEntry {
		thread.mu.Lock()
		thread.mode = modeStepIn
		thread.mu.Unlock()
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if re, ok := r.(*runtimeError); ok {
					d.setExitErr(re)
				} else {
					d.setExitErr(fmt.Errorf("внутренняя ошибка интерпретатора: %v", r))
				}
			}
			d.threadExited(thread)
			code := 0
			if d.Err() != nil {
				code = 1
			}
			d.sendEvent(DebugEvent{Kind: "exited", ExitCode: code})
			d.sendEvent(DebugEvent{Kind: "terminated"})
		}()
		if _, err := d.in.callFunction(d.main, entryArgs(d.main), thread); err != nil {
			d.setExitErr(err)
		}
	}()
}

func (d *Debugger) setExitErr(err error) {
	d.mu.Lock()
	d.exitErr = err
	d.mu.Unlock()
}

// Err - ошибка, с которой завершилась программа (паника/необработанное
// исключение), nil - завершилась успешно. Осмысленно спрашивать только
// после события "exited".
func (d *Debugger) Err() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.exitErr
}

func (d *Debugger) newThread(name string) *dbgThread {
	d.mu.Lock()
	d.threadSeq++
	t := &dbgThread{id: d.threadSeq, name: name, dbg: d}
	d.threads[t.id] = t
	d.mu.Unlock()
	d.sendEvent(DebugEvent{Kind: "thread", ThreadID: t.id, Reason: "started"})
	return t
}

func (d *Debugger) threadExited(t *dbgThread) {
	d.mu.Lock()
	delete(d.threads, t.id)
	d.mu.Unlock()
	d.sendEvent(DebugEvent{Kind: "thread", ThreadID: t.id, Reason: "exited"})
}

func (d *Debugger) thread(id int) *dbgThread {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.threads[id]
}

func (d *Debugger) nextFrameID() int {
	d.mu.Lock()
	d.frameSeq++
	id := d.frameSeq
	d.mu.Unlock()
	return id
}

func (d *Debugger) registerFrame(f *dbgFrame) {
	d.mu.Lock()
	d.frameHandles[f.id] = f
	d.mu.Unlock()
}

func (d *Debugger) frameByID(id int) *dbgFrame {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.frameHandles[id]
}

// SetBreakpoints заменяет ПОЛНЫЙ набор точек останова для file (тем же
// принципом, что и DAP setBreakpoints - клиент каждый раз шлёт весь список
// целиком, не дифф) - возвращает lines обратно (в этой простой модели любая
// строка считается допустимой, без проверки "здесь вообще есть
// инструкция").
func (d *Debugger) SetBreakpoints(file string, lines []int) []int {
	set := make(map[int]bool, len(lines))
	for _, l := range lines {
		set[l] = true
	}
	d.mu.Lock()
	d.breakpoints[file] = set
	d.mu.Unlock()
	out := append([]int{}, lines...)
	sort.Ints(out)
	return out
}

func (d *Debugger) hasBreakpoint(file string, line int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.breakpoints[file][line]
}

// Threads - список активных потоков (главная + ещё не завершившиеся задачи
// "запустить").
func (d *Debugger) Threads() []ThreadInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]ThreadInfo, 0, len(d.threads))
	for _, t := range d.threads {
		out = append(out, ThreadInfo{ID: t.id, Name: t.name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// StackTrace - кадры threadID, самый глубокий (текущий) первым - DAP
// ожидает именно такой порядок (frame[0] - место, где поток сейчас стоит).
func (d *Debugger) StackTrace(threadID int) []FrameInfo {
	t := d.thread(threadID)
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]FrameInfo, 0, len(t.frames))
	for _, f := range slices.Backward(t.frames) {
		out = append(out, FrameInfo{ID: f.id, Name: f.name, File: f.file, Line: f.line})
	}
	return out
}

// Scopes - "Локальные" всегда, "этот" - только у метода/конструктора.
func (d *Debugger) Scopes(frameID int) []Scope {
	f := d.frameByID(frameID)
	if f == nil {
		return nil
	}
	out := []Scope{{Name: "Локальные", VariablesReference: d.registerVar(varHandle{kind: 'l', env: f.env})}}
	if f.this != nil {
		out = append(out, Scope{Name: "этот", VariablesReference: d.registerVar(varHandle{kind: 't', obj: f.this})})
	}
	return out
}

func (d *Debugger) registerVar(h varHandle) int {
	d.mu.Lock()
	d.varSeq++
	id := d.varSeq
	d.varHandles[id] = h
	d.mu.Unlock()
	return id
}

// Variables разворачивает ранее выданный varRef (Scope.VariablesReference
// или Variable.VariablesReference) в список переменных/полей/элементов.
func (d *Debugger) Variables(varRef int) []Variable {
	d.mu.Lock()
	h, ok := d.varHandles[varRef]
	d.mu.Unlock()
	if !ok {
		return nil
	}
	switch h.kind {
	case 'l':
		return d.envVariables(h.env)
	case 't':
		return d.valueChildren(Value{Type: ir.EntityType(h.obj.typeName), Obj: h.obj})
	case 'v':
		return d.valueChildren(h.val)
	default:
		return nil
	}
}

// envVariables - объявленные имена по всей цепочке Env (окружение вложенных
// если/пока/для ОДНОГО вызова, см. package doc Env) - имя из более
// внутренней области перекрывает одноимённое из внешней (обычное
// затенение), поэтому проход идёт от env к parent, первое вхождение имени
// побеждает.
func (d *Debugger) envVariables(env *Env) []Variable {
	seen := map[string]bool{}
	names := make([]string, 0, 8)
	values := map[string]Value{}
	for e := env; e != nil; e = e.parent {
		for name, v := range e.vars {
			if seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
			values[name] = v
		}
	}
	sort.Strings(names)
	out := make([]Variable, 0, len(names))
	for _, name := range names {
		out = append(out, d.describeVariable(name, values[name]))
	}
	return out
}

// formatValue - текстовое представление v для DAP Variable.Value (тип
// отдельно, см. describeVariable) - примитивы печатаются как есть (тем же
// форматом, что и showValue/printValue, eval.go, без суффикса "(Тип)"),
// сущность/generic-коллекция - краткий плейсхолдер (сами поля/элементы
// видны через разворачивание по VariablesReference, а не в этой строке).
func (d *Debugger) formatValue(v Value) string {
	switch v.Type.Kind {
	case ir.KindString:
		return "\"" + v.S + "\""
	case ir.KindInt, ir.KindByte:
		return strconv.FormatInt(v.I, 10)
	case ir.KindFloat, ir.KindFloat32:
		return formatFloat(v)
	case ir.KindChar:
		return string(rune(v.I))
	case ir.KindBool:
		if v.B {
			return "истина"
		}
		return "ложь"
	case ir.KindEntity:
		if v.Obj == nil {
			return "ничто"
		}
		if v.Obj.coll != nil {
			n := len(v.Obj.coll.elems)
			if v.Obj.coll.template == "Словарь" {
				n = v.Obj.coll.pairs.size()
			}
			return fmt.Sprintf("%s (%d)", v.Obj.typeName, n)
		}
		return v.Obj.typeName + " {...}"
	case ir.KindFile:
		return "[файл]"
	case ir.KindSocket:
		return "[сокет]"
	case ir.KindDB:
		return "[бд]"
	case ir.KindHTTP2Session:
		return "[http2-сессия]"
	default:
		return v.S
	}
}

func (d *Debugger) describeVariable(name string, v Value) Variable {
	return Variable{
		Name:               name,
		Value:              d.formatValue(v),
		Type:               v.Type.String(),
		VariablesReference: d.childrenRef(v),
	}
}

// childrenRef - 0 у примитива (нечего разворачивать), иначе - свежий varRef
// на v саму (Variables(varRef) потом развернёт её через valueChildren).
func (d *Debugger) childrenRef(v Value) int {
	if v.Type.Kind == ir.KindEntity && v.Obj != nil {
		return d.registerVar(varHandle{kind: 'v', val: v})
	}
	return 0
}

// valueChildren - поля сущности (в порядке объявления, как и showValue,
// eval.go) или элементы встроенной generic-коллекции по индексу.
func (d *Debugger) valueChildren(v Value) []Variable {
	if v.Obj == nil {
		return nil
	}
	if v.Obj.coll != nil {
		if v.Obj.coll.template == "Словарь" {
			out := make([]Variable, 0, v.Obj.coll.pairs.size())
			for i, key := range v.Obj.coll.pairs.keys {
				out = append(out, d.describeVariable("["+d.formatValue(key)+"]", v.Obj.coll.pairs.vals[i]))
			}
			return out
		}
		out := make([]Variable, len(v.Obj.coll.elems))
		for i, elem := range v.Obj.coll.elems {
			out[i] = d.describeVariable("["+strconv.Itoa(i)+"]", elem)
		}
		return out
	}
	ent := d.in.entities[v.Obj.typeName]
	if ent == nil {
		return nil
	}
	out := make([]Variable, len(ent.Fields))
	for i, f := range ent.Fields {
		out[i] = d.describeVariable(f.Name, v.Obj.getField(f.Name))
	}
	return out
}

// resumeThread - общая часть Continue/Next/StepIn/StepOut: выставляет режим
// (и targetDepth для Next/StepOut - глубина считается ОТ ТЕКУЩЕЙ, на
// момент возобновления) и снимает поток с паузы. Не в паузе - тихо
// игнорирует (клиент DAP не должен слать эти запросы иначе, но лучше не
// паниковать, если всё же прислал).
func (d *Debugger) resumeThread(threadID int, mode stepMode) {
	t := d.thread(threadID)
	if t == nil {
		return
	}
	t.mu.Lock()
	if !t.stopped {
		t.mu.Unlock()
		return
	}
	depth := len(t.frames)
	switch mode {
	case modeStepOver:
		t.targetDepth = depth
	case modeStepOut:
		t.targetDepth = depth - 1
	}
	t.mode = mode
	t.stopped = false
	ch := t.resumeCh
	t.resumeCh = nil
	t.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// Continue - свободный бег до следующей точки останова/конца.
func (d *Debugger) Continue(threadID int) { d.resumeThread(threadID, modeRunning) }

// Next - "через" (step over): не заходить в вызовы, сделанные текущей
// инструкцией.
func (d *Debugger) Next(threadID int) { d.resumeThread(threadID, modeStepOver) }

// StepIn - "в" (step into): остановиться на следующей же инструкции, даже
// если она внутри только что вызванной функции/метода.
func (d *Debugger) StepIn(threadID int) { d.resumeThread(threadID, modeStepIn) }

// StepOut - "из" (step out): доработать текущую функцию/метод до конца и
// остановиться сразу после возврата в вызывающую сторону.
func (d *Debugger) StepOut(threadID int) { d.resumeThread(threadID, modeStepOut) }

// Pause запрашивает остановку threadID на его следующей инструкции - в
// отличие от Continue/Next/StepIn/StepOut, не требует, чтобы поток уже
// стоял на паузе (наоборот, обычно используется, когда он бежит свободно).
func (d *Debugger) Pause(threadID int) {
	t := d.thread(threadID)
	if t == nil {
		return
	}
	t.mu.Lock()
	t.pauseReq = true
	t.mu.Unlock()
}

// Disconnect отпускает все остановленные потоки, чтобы процесс мог
// завершиться естественным образом - у отладчика Rech нет отдельного
// "убить процесс" (в отличие от отладки скомпилированной программы), сама
// программа - горутины этого же процесса rech debug.
func (d *Debugger) Disconnect() {
	d.mu.Lock()
	threads := make([]*dbgThread, 0, len(d.threads))
	for _, t := range d.threads {
		threads = append(threads, t)
	}
	d.mu.Unlock()
	for _, t := range threads {
		d.resumeThread(t.id, modeRunning)
	}
}
