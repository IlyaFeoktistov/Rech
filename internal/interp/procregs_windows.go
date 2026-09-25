//go:build windows

// Мини-отладчик, tier3 (регистры/точки останова) на Windows - см. package
// doc internal/semantic/procregs.go. golang.org/x/sys/windows НЕ
// оборачивает сам debug API (DebugActiveProcess/WaitForDebugEvent/
// ContinueDebugEvent/GetThreadContext/SetThreadContext) - объявлены здесь
// вручную через NewLazySystemDLL (тот же приём, что использует сам
// golang.org/x/sys/windows внутри себя для остальных функций) - без cgo.
//
// winContext (CONTEXT, WinNT.h, amd64) - раскладка полей взята из
// внутреннего рантайма самого Go (runtime/defs_windows_amd64.go - тот же
// стабильный публичный Win32 ABI, не деталь реализации Go), проверить
// можно там же. winDebugEvent (DEBUG_EVENT) - только фиксированный
// префикс полей нужен по имени (код/pid/tid) - сам union (несколько
// структур-вариантов события) захвачен как буфер байт с ЗАПАСОМ (крупнее
// самого большого настоящего варианта), из него читается только первые 4
// байта (ExceptionRecord.ExceptionCode) при EXCEPTION_DEBUG_EVENT.
package interp

import (
	"encoding/binary"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// modKernel32 - уже объявлен в nonblocking_read_windows.go (PeekNamedPipe) -
// переиспользуется здесь, не заводится заново (один "kernel32.dll" на
// весь пакет).
var (
	procDebugActiveProcess     = modKernel32.NewProc("DebugActiveProcess")
	procDebugActiveProcessStop = modKernel32.NewProc("DebugActiveProcessStop")
	procWaitForDebugEvent      = modKernel32.NewProc("WaitForDebugEvent")
	procContinueDebugEvent     = modKernel32.NewProc("ContinueDebugEvent")
	procGetThreadContext       = modKernel32.NewProc("GetThreadContext")
	procSetThreadContext       = modKernel32.NewProc("SetThreadContext")
	procFlushInstructionCache  = modKernel32.NewProc("FlushInstructionCache")
)

const (
	contextAMD64         = 0x100000
	contextControl       = contextAMD64 | 0x1
	contextInteger       = contextAMD64 | 0x2
	contextFloatingPoint = contextAMD64 | 0x8
	contextFull          = contextControl | contextInteger | contextFloatingPoint

	debugEventException = 1
	exceptionBreakpoint = 0x80000003
	exceptionSingleStep = 0x80000004
	dbgContinue         = 0x00010002
	infinite            = 0xFFFFFFFF

	eflagsTrapFlag = 0x100
)

// m128a/winContext - CONTEXT (WinNT.h, amd64) - см. package doc выше.
type m128a struct {
	Low  uint64
	High int64
}

type winContext struct {
	P1Home, P2Home, P3Home, P4Home, P5Home, P6Home   uint64
	ContextFlags, MxCsr                              uint32
	SegCs, SegDs, SegEs, SegFs, SegGs, SegSs         uint16
	EFlags                                           uint32
	Dr0, Dr1, Dr2, Dr3, Dr6, Dr7                     uint64
	Rax, Rcx, Rdx, Rbx, Rsp, Rbp, Rsi, Rdi           uint64
	R8, R9, R10, R11, R12, R13, R14, R15             uint64
	Rip                                              uint64
	fltSave                                          [512]byte
	vectorRegister                                   [26]m128a
	vectorControl                                    uint64
	debugControl, lastBranchToRip, lastBranchFromRip uint64
	lastExceptionToRip, lastExceptionFromRip         uint64
}

// winDebugEvent - DEBUG_EVENT (WinNT.h/debugapi.h) - см. package doc выше.
type winDebugEvent struct {
	DebugEventCode uint32
	ProcessId      uint32
	ThreadId       uint32
	_              uint32 // выравнивание union'а до 8 байт (внутри - HANDLE/указатели)
	u              [256]byte
}

func (e *winDebugEvent) exceptionCode() uint32 {
	return binary.LittleEndian.Uint32(e.u[0:4])
}

// contextToMap/applyMapToContext - winContext <-> map[имя поля]значение -
// тем же набором имён, что registersFieldNames (procregs.go).
func contextToMap(c *winContext) map[string]int64 {
	return map[string]int64{
		"rax": int64(c.Rax), "rbx": int64(c.Rbx), "rcx": int64(c.Rcx), "rdx": int64(c.Rdx),
		"rsi": int64(c.Rsi), "rdi": int64(c.Rdi), "rbp": int64(c.Rbp), "rsp": int64(c.Rsp),
		"rip": int64(c.Rip), "r8": int64(c.R8), "r9": int64(c.R9), "r10": int64(c.R10),
		"r11": int64(c.R11), "r12": int64(c.R12), "r13": int64(c.R13), "r14": int64(c.R14),
		"r15": int64(c.R15), "eflags": int64(c.EFlags),
	}
}

func applyMapToContext(c *winContext, vals map[string]int64) {
	c.Rax, c.Rbx, c.Rcx, c.Rdx = uint64(vals["rax"]), uint64(vals["rbx"]), uint64(vals["rcx"]), uint64(vals["rdx"])
	c.Rsi, c.Rdi, c.Rbp, c.Rsp = uint64(vals["rsi"]), uint64(vals["rdi"]), uint64(vals["rbp"]), uint64(vals["rsp"])
	c.Rip, c.R8, c.R9, c.R10 = uint64(vals["rip"]), uint64(vals["r8"]), uint64(vals["r9"]), uint64(vals["r10"])
	c.R11, c.R12, c.R13, c.R14 = uint64(vals["r11"]), uint64(vals["r12"]), uint64(vals["r13"]), uint64(vals["r14"])
	c.R15, c.EFlags = uint64(vals["r15"]), uint32(vals["eflags"])
}

// getThreadContext/setThreadContext - открывает поток ПО id ИЗ сессии
// (winDbgSession.tid, procmem_windows.go) - НЕ хендл процесса, хендл
// именно того потока, что сообщил последнее debug-событие.
func getThreadContext(tid uint32) (winContext, error) {
	th, err := windows.OpenThread(windows.THREAD_GET_CONTEXT|windows.THREAD_SET_CONTEXT, false, tid)
	if err != nil {
		return winContext{}, err
	}
	defer windows.CloseHandle(th)
	var ctx winContext
	ctx.ContextFlags = contextFull
	r, _, _ := procGetThreadContext.Call(uintptr(th), uintptr(unsafe.Pointer(&ctx)))
	if r == 0 {
		return winContext{}, windows.GetLastError()
	}
	return ctx, nil
}

func setThreadContext(tid uint32, ctx *winContext) error {
	th, err := windows.OpenThread(windows.THREAD_GET_CONTEXT|windows.THREAD_SET_CONTEXT, false, tid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(th)
	r, _, _ := procSetThreadContext.Call(uintptr(th), uintptr(unsafe.Pointer(ctx)))
	if r == 0 {
		return windows.GetLastError()
	}
	return nil
}

// procRegsGet - "регистры" (Процесс) - GetThreadContext потока последней
// остановки (см. package doc winDbgSession, procmem_windows.go).
func procRegsGet(pid int64) map[string]int64 {
	v, ok := winDbgSessions.Load(pid)
	if !ok {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcRegsReadFailed)
	}
	ctx, err := getThreadContext(v.(*winDbgSession).tid)
	if err != nil {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcRegsReadFailed)
	}
	return contextToMap(&ctx)
}

// procRegsSet - "установить_регистры" (Процесс) - GetThreadContext (чтобы
// не затронуть поля, которых нет в "Регистры" - сегментные регистры/
// отладочные регистры/FPU-состояние), затем правит только известные
// поля, SetThreadContext обратно.
func procRegsSet(pid int64, vals map[string]int64) {
	v, ok := winDbgSessions.Load(pid)
	if !ok {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcRegsWriteFailed)
	}
	tid := v.(*winDbgSession).tid
	ctx, err := getThreadContext(tid)
	if err != nil {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcRegsWriteFailed)
	}
	applyMapToContext(&ctx, vals)
	if err := setThreadContext(tid, &ctx); err != nil {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcRegsWriteFailed)
	}
}

// procBreakpointSet - "точка_останова" (Процесс) - ReadProcessMemory/
// WriteProcessMemory, обёрнутые VirtualProtectEx (страница кода почти
// всегда PAGE_EXECUTE_READ - без записи - явно делаем её временно
// записываемой, пишем 0xCC, возвращаем исходную защиту).
func procBreakpointSet(pid int64, addr int64) {
	h, err := windows.OpenProcess(windows.PROCESS_VM_READ|windows.PROCESS_VM_WRITE|windows.PROCESS_VM_OPERATION, false, uint32(pid))
	if err != nil {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcBreakpointSetFailed)
	}
	defer windows.CloseHandle(h)
	var orig [1]byte
	var n uintptr
	if err := windows.ReadProcessMemory(h, uintptr(addr), &orig[0], 1, &n); err != nil || n != 1 {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcBreakpointSetFailed)
	}
	var oldProtect uint32
	protectedOK := windows.VirtualProtectEx(h, uintptr(addr), 1, windows.PAGE_EXECUTE_READWRITE, &oldProtect) == nil
	int3 := [1]byte{0xCC}
	writeErr := windows.WriteProcessMemory(h, uintptr(addr), &int3[0], 1, &n)
	if protectedOK {
		var unused uint32
		windows.VirtualProtectEx(h, uintptr(addr), 1, oldProtect, &unused)
	}
	if writeErr != nil || n != 1 {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcBreakpointSetFailed)
	}
	procFlushInstructionCache.Call(uintptr(h), uintptr(addr), 1)
	bpTable.Store(bpKey{pid, addr}, orig[0])
}

// procBreakpointRemove - "снять_точку_останова" (Процесс) - наилучшее
// старание (если процесс уже завершился, ошибку восстановления
// игнорируем, запись в таблице всё равно удалена).
func procBreakpointRemove(pid int64, addr int64) {
	v, loaded := bpTable.LoadAndDelete(bpKey{pid, addr})
	if !loaded {
		panicRuntime("%s", rtmsg.ProcBreakpointRemoveFailed)
	}
	orig := v.(byte)
	h, err := windows.OpenProcess(windows.PROCESS_VM_WRITE|windows.PROCESS_VM_OPERATION, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	var oldProtect uint32
	protectedOK := windows.VirtualProtectEx(h, uintptr(addr), 1, windows.PAGE_EXECUTE_READWRITE, &oldProtect) == nil
	var n uintptr
	windows.WriteProcessMemory(h, uintptr(addr), &orig, 1, &n)
	if protectedOK {
		var unused uint32
		windows.VirtualProtectEx(h, uintptr(addr), 1, oldProtect, &unused)
	}
	procFlushInstructionCache.Call(uintptr(h), uintptr(addr), 1)
}

// procResume - "возобновить" (Процесс) - если текущий RIP стоит РОВНО на
// уже поставленной точке останова, откатывает/шагает через неё (через
// флаг трассировки EFlags, у Windows нет отдельного "выполнить одну
// инструкцию" вызова, в отличие от PTRACE_SINGLESTEP)/переармирует, затем
// ContinueDebugEvent + цикл WaitForDebugEvent до следующего интересного
// события (точка останова/завершение процесса - остальные, создание
// потоков/загрузка DLL и т. п., молча продолжаются дальше).
func procResume(pid int64) int64 {
	v, ok := winDbgSessions.Load(pid)
	if !ok {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcResumeFailed)
	}
	s := v.(*winDbgSession)

	if ctx, err := getThreadContext(s.tid); err == nil {
		if origVal, found := bpTable.Load(bpKey{pid, int64(ctx.Rip)}); found {
			h, hErr := windows.OpenProcess(windows.PROCESS_VM_READ|windows.PROCESS_VM_WRITE|windows.PROCESS_VM_OPERATION, false, uint32(pid))
			if hErr == nil {
				var oldProtect uint32
				protectedOK := windows.VirtualProtectEx(h, uintptr(ctx.Rip), 1, windows.PAGE_EXECUTE_READWRITE, &oldProtect) == nil
				orig := origVal.(byte)
				var n uintptr
				windows.WriteProcessMemory(h, uintptr(ctx.Rip), &orig, 1, &n)
				procFlushInstructionCache.Call(uintptr(h), uintptr(ctx.Rip), 1)

				ctx.EFlags |= eflagsTrapFlag
				setThreadContext(s.tid, &ctx)
				if s.hasPending {
					procContinueDebugEvent.Call(uintptr(s.pending.ProcessId), uintptr(s.pending.ThreadId), dbgContinue)
					s.hasPending = false
				}
				for {
					var stepEv winDebugEvent
					r, _, _ := procWaitForDebugEvent.Call(uintptr(unsafe.Pointer(&stepEv)), infinite)
					if r == 0 {
						windows.CloseHandle(h)
						throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcResumeFailed)
					}
					if stepEv.DebugEventCode == debugEventException && stepEv.exceptionCode() == exceptionSingleStep {
						s.tid, s.pending, s.hasPending = stepEv.ThreadId, stepEv, true
						break
					}
					procContinueDebugEvent.Call(uintptr(stepEv.ProcessId), uintptr(stepEv.ThreadId), dbgContinue)
				}

				int3 := [1]byte{0xCC}
				windows.WriteProcessMemory(h, uintptr(ctx.Rip), &int3[0], 1, &n)
				if protectedOK {
					var unused uint32
					windows.VirtualProtectEx(h, uintptr(ctx.Rip), 1, oldProtect, &unused)
				}
				procFlushInstructionCache.Call(uintptr(h), uintptr(ctx.Rip), 1)
				windows.CloseHandle(h)
			}
		}
	}

	if s.hasPending {
		procContinueDebugEvent.Call(uintptr(s.pending.ProcessId), uintptr(s.pending.ThreadId), dbgContinue)
		s.hasPending = false
	}
	for {
		var ev winDebugEvent
		r, _, _ := procWaitForDebugEvent.Call(uintptr(unsafe.Pointer(&ev)), infinite)
		if r == 0 {
			throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcResumeFailed)
		}
		s.tid, s.pending, s.hasPending = ev.ThreadId, ev, true
		if ev.DebugEventCode == exitProcessDebugEvent {
			procContinueDebugEvent.Call(uintptr(ev.ProcessId), uintptr(ev.ThreadId), dbgContinue)
			s.hasPending = false
			winDbgSessions.Delete(pid)
			return -1
		}
		if ev.DebugEventCode == debugEventException && ev.exceptionCode() == exceptionBreakpoint {
			// Живой найденный баг: ЛЮБОЙ EXCEPTION_BREAKPOINT здесь -
			// не обязательно НАША точка останова - DebugActiveProcess на
			// уже запущенном процессе сама вставляет один синтетический
			// "attach breakpoint" (ntdll!DbgBreakPoint, стандартное
			// поведение Windows debug API - сигнал "подключение
			// завершено") - принимать его за нашу точку останова
			// неверно. Проверяем адрес по таблице ПЕРЕД тем, как считать
			// остановку "нашей" - иначе не наша точка останова тоже
			// молча "срабатывает".
			if ctx, err := getThreadContext(ev.ThreadId); err == nil {
				hitAddr := int64(ctx.Rip) - 1
				if _, isOurs := bpTable.Load(bpKey{pid, hitAddr}); isOurs {
					ctx.Rip = uint64(hitAddr)
					setThreadContext(ev.ThreadId, &ctx)
					return hitAddr
				}
			}
			// не наша точка останова (например, синтетический "attach
			// breakpoint") - продолжить дальше молча, как и любое другое
			// неинтересное событие.
		}
		procContinueDebugEvent.Call(uintptr(ev.ProcessId), uintptr(ev.ThreadId), dbgContinue)
		s.hasPending = false
	}
}

const exitProcessDebugEvent = 5
