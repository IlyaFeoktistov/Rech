package debugadapter

import (
	"encoding/json"
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/dap"
)

type breakpointsArgs struct {
	Source struct {
		Path string `json:"path"`
	} `json:"source"`
	Breakpoints []struct {
		Line int `json:"line"`
	} `json:"breakpoints"`
}

func (a *adapter) handleSetBreakpoints(req *dap.Request) {
	var args breakpointsArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		a.fail(req, fmt.Errorf("аргументы setBreakpoints: %w", err))
		return
	}
	lines := make([]int, len(args.Breakpoints))
	for i, b := range args.Breakpoints {
		lines[i] = b.Line
	}
	verified := a.dbg.SetBreakpoints(args.Source.Path, lines)
	out := make([]map[string]any, len(verified))
	for i, l := range verified {
		out[i] = map[string]any{"verified": true, "line": l}
	}
	a.ok(req, map[string]any{"breakpoints": out})
}

// handleConfigurationDone - см. package doc: клиент закончил слать
// setBreakpoints, здесь и только здесь программа начинает исполняться.
func (a *adapter) handleConfigurationDone(req *dap.Request) {
	a.ok(req, nil)
	a.dbg.Start(a.stopOnEntry)
}

func (a *adapter) handleThreads(req *dap.Request) {
	threads := a.dbg.Threads()
	out := make([]map[string]any, len(threads))
	for i, t := range threads {
		out[i] = map[string]any{"id": t.ID, "name": t.Name}
	}
	a.ok(req, map[string]any{"threads": out})
}

type threadIDArgs struct {
	ThreadID int `json:"threadId"`
}

func (a *adapter) handleStackTrace(req *dap.Request) {
	var args threadIDArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		a.fail(req, fmt.Errorf("аргументы stackTrace: %w", err))
		return
	}
	frames := a.dbg.StackTrace(args.ThreadID)
	out := make([]map[string]any, len(frames))
	for i, f := range frames {
		out[i] = map[string]any{
			"id":     f.ID,
			"name":   f.Name,
			"line":   f.Line,
			"column": 1,
			"source": map[string]any{"path": f.File},
		}
	}
	a.ok(req, map[string]any{"stackFrames": out, "totalFrames": len(out)})
}

type frameIDArgs struct {
	FrameID int `json:"frameId"`
}

func (a *adapter) handleScopes(req *dap.Request) {
	var args frameIDArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		a.fail(req, fmt.Errorf("аргументы scopes: %w", err))
		return
	}
	scopes := a.dbg.Scopes(args.FrameID)
	out := make([]map[string]any, len(scopes))
	for i, s := range scopes {
		out[i] = map[string]any{"name": s.Name, "variablesReference": s.VariablesReference, "expensive": false}
	}
	a.ok(req, map[string]any{"scopes": out})
}

type variablesArgs struct {
	VariablesReference int `json:"variablesReference"`
}

func (a *adapter) handleVariables(req *dap.Request) {
	var args variablesArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		a.fail(req, fmt.Errorf("аргументы variables: %w", err))
		return
	}
	vars := a.dbg.Variables(args.VariablesReference)
	out := make([]map[string]any, len(vars))
	for i, v := range vars {
		out[i] = map[string]any{
			"name":               v.Name,
			"value":              v.Value,
			"type":               v.Type,
			"variablesReference": v.VariablesReference,
		}
	}
	a.ok(req, map[string]any{"variables": out})
}

func (a *adapter) handleContinue(req *dap.Request) {
	var args threadIDArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		a.fail(req, fmt.Errorf("аргументы continue: %w", err))
		return
	}
	a.dbg.Continue(args.ThreadID)
	a.ok(req, map[string]any{"allThreadsContinued": false})
}

// handleStep - общий обработчик next/stepIn/stepOut/pause: у всех четырёх
// одинаковые аргументы ({threadId}) и пустое тело ответа, отличается только
// какой метод Debugger вызвать.
func (a *adapter) handleStep(req *dap.Request, do func(threadID int)) {
	var args threadIDArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		a.fail(req, fmt.Errorf("аргументы %s: %w", req.Command, err))
		return
	}
	do(args.ThreadID)
	a.ok(req, nil)
}

type evaluateArgs struct {
	Expression string `json:"expression"`
	FrameID    int    `json:"frameId"`
}

func (a *adapter) handleEvaluate(req *dap.Request) {
	var args evaluateArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		a.fail(req, fmt.Errorf("аргументы evaluate: %w", err))
		return
	}
	result, err := a.dbg.Evaluate(args.FrameID, args.Expression)
	if err != nil {
		a.fail(req, err)
		return
	}
	a.ok(req, map[string]any{"result": result, "variablesReference": 0})
}

func (a *adapter) handleDisconnect(req *dap.Request) {
	if a.dbg != nil {
		a.dbg.Disconnect()
	}
	a.ok(req, nil)
	a.disconnected = true
}

// pumpEvents переводит события interp.Debugger в события DAP - отдельная
// горутина на всё время жизни сессии (запускается сразу после успешного
// launch, см. handleLaunch), т. к. события ("stopped" на точке останова и
// т. п.) приходят асинхронно, не в ответ на конкретный запрос клиента.
func (a *adapter) pumpEvents() {
	for e := range a.dbg.Events() {
		switch e.Kind {
		case "stopped":
			_ = a.server.SendEvent("stopped", map[string]any{
				"reason":            e.Reason,
				"threadId":          e.ThreadID,
				"allThreadsStopped": false,
			})
		case "thread":
			_ = a.server.SendEvent("thread", map[string]any{"reason": e.Reason, "threadId": e.ThreadID})
		case "exited":
			_ = a.server.SendEvent("exited", map[string]any{"exitCode": e.ExitCode})
			if err := a.dbg.Err(); err != nil {
				_ = a.server.SendEvent("output", map[string]any{"category": "stderr", "output": err.Error() + "\n"})
			}
		case "terminated":
			_ = a.server.SendEvent("terminated", nil)
			return
		}
	}
}
