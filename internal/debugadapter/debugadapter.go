// Package debugadapter переводит между DAP (internal/dap - транспорт,
// ничего не знает про Rech) и internal/interp.Debugger (пошаговое
// исполнение Rech-программы, ничего не знает про DAP/JSON) - весь смысл
// пакета в этом переводе, собственной логики отладки здесь нет.
//
// Последовательность запросов - стандартная для DAP (см. спецификацию
// Microsoft): initialize -> (сервер шлёт событие "initialized") ->
// setBreakpoints* -> configurationDone (здесь и только здесь запускается
// исполнение программы - launch к этому моменту уже отвечен, но точки
// останова уже все стоят) -> continue/next/stepIn/stepOut/pause/
// stackTrace/scopes/variables/evaluate по ходу остановок -> disconnect.
package debugadapter

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
	"github.com/IlyaFeoktistov/Rech/internal/dap"
	"github.com/IlyaFeoktistov/Rech/internal/interp"
	"github.com/IlyaFeoktistov/Rech/internal/project"
)

// Run обслуживает одну сессию отладки поверх in/out (в реальном запуске -
// stdin/stdout процесса "rech debug") - блокируется до тех пор, пока клиент
// не отключится ("disconnect"/закрыл соединение). "вывести"/"показать"
// отлаживаемой программы становится событием DAP "output" (см.
// outputWriter), а не пишется в настоящий stdout - тот целиком занят
// протоколом DAP.
//
// programIn - stdin ОТЛАЖИВАЕМОЙ программы (прочитать_строку/
// прочитать_целое/прочитать_число) - в этом срезе не подключён к
// настоящему интерактивному вводу (сам os.Stdin процесса занят протоколом
// DAP, см. выше - смешивать в одном потоке JSON-сообщения DAP и
// произвольный текст программы нельзя) - явное, задокументированное
// ограничение (docs/ROADMAP.md, шаг 19): вызов любой из этих встроенных
// функций под отладчиком сразу получает конец ввода.
func Run(in io.Reader, out io.Writer, programIn io.Reader) error {
	s := dap.NewServer(in, out)
	a := &adapter{server: s, programIn: programIn}
	for {
		req, err := s.ReadRequest()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		a.handle(req)
		if a.disconnected {
			return nil
		}
	}
}

type adapter struct {
	server       *dap.Server
	dbg          *interp.Debugger
	programIn    io.Reader
	stopOnEntry  bool
	disconnected bool
}

func (a *adapter) handle(req *dap.Request) {
	switch req.Command {
	case "initialize":
		a.handleInitialize(req)
	case "launch":
		a.handleLaunch(req)
	case "setBreakpoints":
		a.handleSetBreakpoints(req)
	case "configurationDone":
		a.handleConfigurationDone(req)
	case "threads":
		a.handleThreads(req)
	case "stackTrace":
		a.handleStackTrace(req)
	case "scopes":
		a.handleScopes(req)
	case "variables":
		a.handleVariables(req)
	case "continue":
		a.handleContinue(req)
	case "next":
		a.handleStep(req, a.dbg.Next)
	case "stepIn":
		a.handleStep(req, a.dbg.StepIn)
	case "stepOut":
		a.handleStep(req, a.dbg.StepOut)
	case "pause":
		a.handleStep(req, a.dbg.Pause)
	case "evaluate":
		a.handleEvaluate(req)
	case "disconnect", "terminate":
		a.handleDisconnect(req)
	default:
		// Неизвестная команда - DAP допускает расширения протокола, о
		// которых этот сервер не обязан знать; отвечаем неуспехом, а не
		// падаем и не молчим (клиент иначе ждал бы ответ вечно).
		_ = a.server.SendResponse(req, false, nil, fmt.Sprintf("команда %q не поддерживается", req.Command))
	}
}

func (a *adapter) ok(req *dap.Request, body any) {
	_ = a.server.SendResponse(req, true, body, "")
}

func (a *adapter) fail(req *dap.Request, err error) {
	_ = a.server.SendResponse(req, false, nil, err.Error())
}

func (a *adapter) handleInitialize(req *dap.Request) {
	a.ok(req, map[string]any{
		"supportsConfigurationDoneRequest": true,
		"supportsEvaluateForHovers":        true,
		"supportsTerminateRequest":         true,
	})
	_ = a.server.SendEvent("initialized", nil)
}

type launchArgs struct {
	Program     string `json:"program"`
	StopOnEntry bool   `json:"stopOnEntry"`
}

// resolveProgram - program как есть (файл/каталог .rech), если это не файл
// проекта; иначе - путь к его главному файлу (project.File.MainPath) - тот
// же выбор цели, что и у "rech build"/"rech run" (cmd/rech/main.go),
// приведён к общему коду, а не продублирован.
func resolveProgram(program string) (string, error) {
	if filepath.Ext(program) != ".rechproj" {
		return program, nil
	}
	pf, err := project.Load(program)
	if err != nil {
		return "", err
	}
	return pf.MainPath(), nil
}

// handleLaunch строит программу (разбор + семантический анализ, БЕЗ
// C-кодогенерации/системного компилятора - см. compiler.Compiler.BuildIR) и
// готовит Debugger - само исполнение начинается позже, на
// configurationDone (см. package doc), не здесь: точки останова между
// launch и configurationDone ещё придут.
func (a *adapter) handleLaunch(req *dap.Request) {
	var args launchArgs
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		a.fail(req, fmt.Errorf("аргументы launch: %w", err))
		return
	}
	if args.Program == "" {
		a.fail(req, fmt.Errorf("launch: не указан \"program\" (файл/каталог .rech или файл .rechproj)"))
		return
	}
	target, err := resolveProgram(args.Program)
	if err != nil {
		a.fail(req, err)
		return
	}
	c := compiler.NewCompiler(compiler.NewFrontend(), nil, nil)
	prog, err := c.BuildIR(target)
	if err != nil {
		a.fail(req, err)
		return
	}
	dbg, err := interp.NewDebugger(prog, &outputWriter{server: a.server}, a.programIn)
	if err != nil {
		a.fail(req, err)
		return
	}
	a.dbg = dbg
	a.stopOnEntry = args.StopOnEntry
	go a.pumpEvents()
	a.ok(req, nil)
}

// outputWriter - "вывести"/"показать" отлаживаемой программы становятся
// событиями DAP "output" (Debug Console в VS Code), а не пишутся в
// настоящий stdout процесса "rech debug" (тот целиком занят протоколом DAP,
// см. Run).
type outputWriter struct {
	server *dap.Server
}

func (w *outputWriter) Write(p []byte) (int, error) {
	_ = w.server.SendEvent("output", map[string]any{"category": "stdout", "output": string(p)})
	return len(p), nil
}
