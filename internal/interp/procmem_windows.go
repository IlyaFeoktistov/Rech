//go:build windows

package interp

import (
	"bytes"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// procMemFindMaxRegionSize - см. package doc internal/backend/cbackend/
// procmem.go, procMemFindHelper - тот же лимит, что и Linux-версия
// (procmem_linux.go), общий с cbackend.
const procMemFindMaxRegionSize = 256 * 1024 * 1024

// winDbgSession - сессия DebugActiveProcess - "tid"/"pending"/"hasPending" -
// поток и событие ПОСЛЕДНЕЙ остановки (нужны GetThreadContext/
// SetThreadContext и ContinueDebugEvent, который обязан указать ИМЕННО
// тот pid/tid, что был в событии) - используется и здесь (attach/detach),
// и в internal/interp/procregs_windows.go (регистры/точки останова/
// возобновить), тем же принципом, что rech_win_dbg_t в cbackend
// (internal/backend/cbackend/procmem.go, package doc там - там же честно
// про смену поведения "подключиться_к_процессу": раньше (tier1/tier2) -
// только OpenProcess-проверка без остановки, теперь - настоящий
// DebugActiveProcess, останавливающий процесс, как и на Linux).
type winDbgSession struct {
	tid        uint32
	hasPending bool
	pending    winDebugEvent
}

var winDbgSessions sync.Map

// enableDebugPrivilegeOnce - живой найденный баг: DebugActiveProcess
// падает с ERROR_ACCESS_DENIED даже на СВОЙ собственный дочерний процесс
// (запущенный обычным CreateProcess, БЕЗ флага DEBUG_PROCESS) - в отличие
// от "родитель отлаживает потомка через CREATE_PROCESS с DEBUG_PROCESS",
// подключение ПОСТФАКТУМ через DebugActiveProcess требует явно включённой
// привилегии SeDebugPrivilege в токене текущего процесса - она есть у
// большинства процессов (в т. ч. без прав администратора), но НЕ включена
// по умолчанию (доступна - не значит активна, Windows-специфика
// привилегий) - AdjustTokenPrivileges включает её, идемпотентно (sync.Once
// - один раз на процесс, не на каждое подключение).
var enableDebugPrivilegeOnce sync.Once

func enableDebugPrivilege() {
	enableDebugPrivilegeOnce.Do(func() {
		var token windows.Token
		if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
			return
		}
		defer token.Close()
		name, err := windows.UTF16PtrFromString("SeDebugPrivilege")
		if err != nil {
			return
		}
		var luid windows.LUID
		if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
			return
		}
		tp := windows.Tokenprivileges{
			PrivilegeCount: 1,
			Privileges:     [1]windows.LUIDAndAttributes{{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}},
		}
		windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil)
	})
}

// procAttach - "подключиться_к_процессу(pid): Процесс" - DebugActiveProcess
// (вызывающий становится ЕДИНСТВЕННЫМ отладчиком процесса) + первый
// WaitForDebugEvent (CREATE_PROCESS_DEBUG_EVENT, генерируется автоматически
// при подключении) - процесс остаётся остановленным, пока за него не
// вызван ContinueDebugEvent (см. procDetach/procregs_windows.go).
func procAttach(pid int64) {
	enableDebugPrivilege()
	ok, _, _ := procDebugActiveProcess.Call(uintptr(pid))
	if ok == 0 {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcAttachFailed)
	}
	s := &winDbgSession{}
	r, _, _ := procWaitForDebugEvent.Call(uintptr(unsafe.Pointer(&s.pending)), infinite)
	if r == 0 {
		procDebugActiveProcessStop.Call(uintptr(pid))
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcAttachFailed)
	}
	s.tid = s.pending.ThreadId
	s.hasPending = true
	winDbgSessions.Store(pid, s)
}

// procDetach - "отключиться" (Процесс, метод-сахар "отключиться_от_
// процесса") - ContinueDebugEvent за необработанное событие (иначе
// процесс так и останется висеть остановленным навсегда) +
// DebugActiveProcessStop (перестать быть отладчиком - процесс продолжает
// жить самостоятельно).
func procDetach(pid int64) {
	v, ok := winDbgSessions.LoadAndDelete(pid)
	if !ok {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcDetachFailed)
	}
	s := v.(*winDbgSession)
	if s.hasPending {
		procContinueDebugEvent.Call(uintptr(s.pending.ProcessId), uintptr(s.pending.ThreadId), dbgContinue)
	}
	ok2, _, _ := procDebugActiveProcessStop.Call(uintptr(pid))
	if ok2 == 0 {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcDetachFailed)
	}
}

// procRead - "прочитать" (Процесс) - ReadProcessMemory по хендлу, открытому
// и закрытому ВНУТРИ этого вызова (Процесс несёт только pid, не HANDLE -
// тот же принцип, что и rech_wait_process в cbackend, subprocess.go).
func procRead(pid int64, addr int64, size int64) []byte {
	if size <= 0 {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcMemReadFailed)
	}
	h, err := windows.OpenProcess(windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcMemReadFailed)
	}
	defer windows.CloseHandle(h)
	buf := make([]byte, size)
	var got uintptr
	if err := windows.ReadProcessMemory(h, uintptr(addr), &buf[0], uintptr(size), &got); err != nil || got != uintptr(size) {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcMemReadFailed)
	}
	return buf
}

// procWrite - "записать" (Процесс) - WriteProcessMemory, симметрично
// procRead.
func procWrite(pid int64, addr int64, data []byte) {
	if len(data) == 0 {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_VM_WRITE|windows.PROCESS_VM_OPERATION, false, uint32(pid))
	if err != nil {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcMemWriteFailed)
	}
	defer windows.CloseHandle(h)
	var written uintptr
	if err := windows.WriteProcessMemory(h, uintptr(addr), &data[0], uintptr(len(data)), &written); err != nil || written != uintptr(len(data)) {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcMemWriteFailed)
	}
}

// procFind - "найти" (Процесс) - VirtualQueryEx перебирает регионы
// адресного пространства процесса от начала (BaseAddress+RegionSize -
// следующий адрес), ReadProcessMemory читает только MEM_COMMIT-регионы
// без PAGE_NOACCESS/PAGE_GUARD - тем же принципом, что cbackend
// procMemFindHelper (internal/backend/cbackend/procmem.go, package doc
// там). Недоступный для чтения регион - пропускается молча, не паника.
func procFind(pid int64, pattern []byte) []int64 {
	if len(pattern) == 0 {
		panicRuntime("%s", rtmsg.ProcMemPatternEmpty)
	}
	h, err := windows.OpenProcess(windows.PROCESS_VM_READ|windows.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	var matches []int64
	var addr uintptr
	for {
		var mbi windows.MemoryBasicInformation
		if err := windows.VirtualQueryEx(h, addr, &mbi, unsafe.Sizeof(mbi)); err != nil {
			break
		}
		if mbi.State == windows.MEM_COMMIT && mbi.Protect&(windows.PAGE_NOACCESS|windows.PAGE_GUARD) == 0 {
			size := int64(mbi.RegionSize)
			if size > 0 && size <= procMemFindMaxRegionSize {
				buf := make([]byte, size)
				var got uintptr
				if err := windows.ReadProcessMemory(h, mbi.BaseAddress, &buf[0], uintptr(size), &got); err == nil && int64(got) == size {
					for i := 0; i+len(pattern) <= len(buf); i++ {
						if bytes.Equal(buf[i:i+len(pattern)], pattern) {
							matches = append(matches, int64(mbi.BaseAddress)+int64(i))
						}
					}
				}
			}
		}
		next := mbi.BaseAddress + mbi.RegionSize
		if next <= addr {
			break
		}
		addr = next
	}
	return matches
}
