//go:build linux

package interp

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// procMemFindMaxRegionSize - см. package doc internal/backend/cbackend/
// procmem.go, procMemFindHelper - тот же лимит (защита от чтения
// патологически огромных mmap-регионов целиком), общий с cbackend
// значением, не отдельно выбранный.
const procMemFindMaxRegionSize = 256 * 1024 * 1024

// ptraceSessions - pid -> канал управления выделенной, залоченной на СВОЙ
// OS-поток горутиной, которая держит ptrace-подключение (см. package doc
// procAttach ниже про причину, почему это вообще нужно).
var ptraceSessions sync.Map

// ptraceCmd - произвольная функция fn, которую нужно выполнить НА
// залоченном ОС-потоке сессии (см. package doc procAttach) - регистры/
// точки останова/возобновить (tier3, procregs_linux.go) настолько же
// thread-affine, насколько и сам attach/detach (все - часть одного
// ptrace-протокола) - тем же каналом, что и раньше, просто действие
// вместо жёстко зашитого "отключиться".
type ptraceCmd struct {
	fn   func() error
	done chan error
}

// runOnPtraceSession выполняет fn на залоченном ОС-потоке уже
// существующей ptrace-сессии pid - общая точка входа для ЛЮБОЙ ptrace-
// операции этого файла и procregs_linux.go (регистры/точки останова/
// возобновить). ok=false - сессии нет (не подключены/уже отключились).
func runOnPtraceSession(pid int64, fn func() error) (err error, ok bool) {
	v, loaded := ptraceSessions.Load(pid)
	if !loaded {
		return nil, false
	}
	done := make(chan error, 1)
	v.(chan ptraceCmd) <- ptraceCmd{fn: fn, done: done}
	return <-done, true
}

// procAttach - "подключиться_к_процессу(pid): Процесс" - ptrace(PTRACE_
// ATTACH) + ожидание остановки (waitpid) - ОСТАНАВЛИВАЕТ целевой процесс,
// тем же способом, каким это делает настоящий отладчик (см. package doc
// internal/backend/cbackend/procmem.go про честную асимметрию с Windows).
//
// PTRACE_ATTACH/PTRACE_DETACH в Linux привязаны к КОНКРЕТНОМУ ОС-потоку
// (ядро запоминает "трассировщика" по ID потока, не процесса) - планировщик
// Go горутин по умолчанию свободно переносит горутину между ОС-потоками
// между вызовами, поэтому и "подключиться", и "отключиться" одной и той же
// сессии выполняются НЕ прямо здесь, а внутри отдельной горутины,
// залоченной на один ОС-поток на всё время жизни подключения
// (runtime.LockOSThread) - без этого второй вызов (отключиться) почти
// наверняка попал бы на другой поток и получил бы ESRCH ("нет такого
// процесса"), хотя процесс на самом деле есть. Само чтение/запись памяти
// (process_vm_readv/writev) ТАКОЙ привязки не требует - это обычные
// системные вызовы, не часть ptrace-протокола, могут идти с любого потока
// (см. procRead/procWrite ниже).
func procAttach(pid int64) {
	ch := make(chan ptraceCmd)
	started := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := syscall.PtraceAttach(int(pid)); err != nil {
			started <- err
			return
		}
		var ws syscall.WaitStatus
		if _, err := syscall.Wait4(int(pid), &ws, 0, nil); err != nil {
			started <- err
			return
		}
		started <- nil
		for cmd := range ch {
			cmd.done <- cmd.fn()
		}
	}()
	if err := <-started; err != nil {
		panicRuntime("%s", rtmsg.ProcAttachFailed)
	}
	ptraceSessions.Store(pid, ch)
}

// procDetach - "отключиться" (Процесс, метод-сахар "отключиться_от_
// процесса") - отправляет команду детача той же горутине/потоку, что
// выполнил procAttach (см. package doc там), затем закрывает канал -
// сессия-горутина видит закрытый канал на следующей итерации "for cmd :=
// range ch" и естественным образом завершается (никакой утечки горутин).
func procDetach(pid int64) {
	v, loaded := ptraceSessions.LoadAndDelete(pid)
	if !loaded {
		panicRuntime("%s", rtmsg.ProcDetachFailed)
	}
	ch := v.(chan ptraceCmd)
	done := make(chan error, 1)
	ch <- ptraceCmd{fn: func() error { return syscall.PtraceDetach(int(pid)) }, done: done}
	err := <-done
	close(ch)
	if err != nil {
		panicRuntime("%s", rtmsg.ProcDetachFailed)
	}
}

// procRead - "прочитать" (Процесс) - process_vm_readv, один системный
// вызов на произвольное количество байт (в отличие от PTRACE_PEEKDATA,
// ограниченного словом машинного размера за вызов).
func procRead(pid int64, addr int64, size int64) []byte {
	if size <= 0 {
		panicRuntime("%s", rtmsg.ProcMemReadFailed)
	}
	buf := make([]byte, size)
	local := []unix.Iovec{{Base: &buf[0]}}
	local[0].SetLen(int(size))
	remote := []unix.RemoteIovec{{Base: uintptr(addr), Len: int(size)}}
	n, err := unix.ProcessVMReadv(int(pid), local, remote, 0)
	if err != nil || n != int(size) {
		panicRuntime("%s", rtmsg.ProcMemReadFailed)
	}
	return buf
}

// procWrite - "записать" (Процесс) - process_vm_writev, симметрично
// procRead.
func procWrite(pid int64, addr int64, data []byte) {
	if len(data) == 0 {
		return
	}
	local := []unix.Iovec{{Base: &data[0]}}
	local[0].SetLen(len(data))
	remote := []unix.RemoteIovec{{Base: uintptr(addr), Len: len(data)}}
	n, err := unix.ProcessVMWritev(int(pid), local, remote, 0)
	if err != nil || n != len(data) {
		panicRuntime("%s", rtmsg.ProcMemWriteFailed)
	}
}

// procFind - "найти" (Процесс) - сканирует ВСЕ читаемые страницы памяти
// процесса ("/proc/<pid>/maps") в поиске байтовой сигнатуры pattern,
// возвращает адреса всех вхождений (наивный поиск, включая
// перекрывающиеся совпадения - тот же принцип, что и cbackend
// procMemFindHelper, см. package doc там). Недоступный для чтения регион
// (сбой process_vm_readv) - пропускается молча, не паника.
func procFind(pid int64, pattern []byte) []int64 {
	if len(pattern) == 0 {
		panicRuntime("%s", rtmsg.ProcMemPatternEmpty)
	}
	f, err := os.Open(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil
	}
	defer f.Close()
	var matches []int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var start, end uint64
		var perms string
		if _, err := fmt.Sscanf(sc.Text(), "%x-%x %s", &start, &end, &perms); err != nil {
			continue
		}
		if len(perms) == 0 || perms[0] != 'r' {
			continue
		}
		size := int64(end - start)
		if size <= 0 || size > procMemFindMaxRegionSize {
			continue
		}
		buf := make([]byte, size)
		local := []unix.Iovec{{Base: &buf[0]}}
		local[0].SetLen(int(size))
		remote := []unix.RemoteIovec{{Base: uintptr(start), Len: int(size)}}
		n, err := unix.ProcessVMReadv(int(pid), local, remote, 0)
		if err != nil || n != int(size) {
			continue
		}
		for i := 0; i+len(pattern) <= len(buf); i++ {
			if bytes.Equal(buf[i:i+len(pattern)], pattern) {
				matches = append(matches, int64(start)+int64(i))
			}
		}
	}
	_ = sc.Err() // одна повреждённая строка "maps" - не повод паниковать, см. package doc выше
	return matches
}
