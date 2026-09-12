//go:build windows

package interp

import (
	"bytes"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// procMemFindMaxRegionSize - см. package doc internal/backend/cbackend/
// procmem.go, procMemFindHelper - тот же лимит, что и Linux-версия
// (procmem_linux.go), общий с cbackend.
const procMemFindMaxRegionSize = 256 * 1024 * 1024

// procAttach - "подключиться_к_процессу(pid): Процесс" - Windows не знает
// ptrace-подобного "остановить процесс по pid без полноценной отладочной
// сессии" (DebugActiveProcess делает вызывающего ЕДИНСТВЕННЫМ отладчиком
// процесса и требует цикла WaitForDebugEvent/ContinueDebugEvent -
// самостоятельная тема будущих регистров/точек останова, не этого шага) -
// здесь ТОЛЬКО проверка, что процесс с таким pid существует и доступен
// (OpenProcess), процесс НЕ останавливается (честная асимметрия платформ,
// см. package doc internal/backend/cbackend/procmem.go).
func procAttach(pid int64) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		panicRuntime("%s", rtmsg.ProcAttachFailed)
	}
	windows.CloseHandle(h)
}

// procDetach - "отключиться" (Процесс, метод-сахар "отключиться_от_
// процесса") - ничего не делает: "подключиться" ничего не останавливало
// (см. package doc procAttach выше).
func procDetach(pid int64) {}

// procRead - "прочитать" (Процесс) - ReadProcessMemory по хендлу, открытому
// и закрытому ВНУТРИ этого вызова (Процесс несёт только pid, не HANDLE -
// тот же принцип, что и rech_wait_process в cbackend, subprocess.go).
func procRead(pid int64, addr int64, size int64) []byte {
	if size <= 0 {
		panicRuntime("%s", rtmsg.ProcMemReadFailed)
	}
	h, err := windows.OpenProcess(windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		panicRuntime("%s", rtmsg.ProcMemReadFailed)
	}
	defer windows.CloseHandle(h)
	buf := make([]byte, size)
	var got uintptr
	if err := windows.ReadProcessMemory(h, uintptr(addr), &buf[0], uintptr(size), &got); err != nil || got != uintptr(size) {
		panicRuntime("%s", rtmsg.ProcMemReadFailed)
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
		panicRuntime("%s", rtmsg.ProcMemWriteFailed)
	}
	defer windows.CloseHandle(h)
	var written uintptr
	if err := windows.WriteProcessMemory(h, uintptr(addr), &data[0], uintptr(len(data)), &written); err != nil || written != uintptr(len(data)) {
		panicRuntime("%s", rtmsg.ProcMemWriteFailed)
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
