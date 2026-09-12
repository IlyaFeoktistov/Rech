//go:build linux

package interp

import (
	"fmt"
	"syscall"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// ptraceRegsToMap/mapToPtraceRegs - syscall.PtraceRegs (struct user_regs_
// struct, <sys/user.h>) <-> map[имя поля]значение - тем же набором имён,
// что и registersFieldNames (procregs.go).
func ptraceRegsToMap(r *syscall.PtraceRegs) map[string]int64 {
	return map[string]int64{
		"rax": int64(r.Rax), "rbx": int64(r.Rbx), "rcx": int64(r.Rcx), "rdx": int64(r.Rdx),
		"rsi": int64(r.Rsi), "rdi": int64(r.Rdi), "rbp": int64(r.Rbp), "rsp": int64(r.Rsp),
		"rip": int64(r.Rip), "r8": int64(r.R8), "r9": int64(r.R9), "r10": int64(r.R10),
		"r11": int64(r.R11), "r12": int64(r.R12), "r13": int64(r.R13), "r14": int64(r.R14),
		"r15": int64(r.R15), "eflags": int64(r.Eflags),
	}
}

func applyMapToPtraceRegs(r *syscall.PtraceRegs, vals map[string]int64) {
	r.Rax, r.Rbx, r.Rcx, r.Rdx = uint64(vals["rax"]), uint64(vals["rbx"]), uint64(vals["rcx"]), uint64(vals["rdx"])
	r.Rsi, r.Rdi, r.Rbp, r.Rsp = uint64(vals["rsi"]), uint64(vals["rdi"]), uint64(vals["rbp"]), uint64(vals["rsp"])
	r.Rip, r.R8, r.R9, r.R10 = uint64(vals["rip"]), uint64(vals["r8"]), uint64(vals["r9"]), uint64(vals["r10"])
	r.R11, r.R12, r.R13, r.R14 = uint64(vals["r11"]), uint64(vals["r12"]), uint64(vals["r13"]), uint64(vals["r14"])
	r.R15, r.Eflags = uint64(vals["r15"]), uint64(vals["eflags"])
}

// procRegsGet - "регистры" (Процесс) - PTRACE_GETREGS на залоченном
// потоке уже существующей ptrace-сессии (см. package doc procAttach,
// procmem_linux.go).
func procRegsGet(pid int64) map[string]int64 {
	var regs syscall.PtraceRegs
	err, ok := runOnPtraceSession(pid, func() error {
		return syscall.PtraceGetRegs(int(pid), &regs)
	})
	if !ok || err != nil {
		panicRuntime("%s", rtmsg.ProcRegsReadFailed)
	}
	return ptraceRegsToMap(&regs)
}

// procRegsSet - "установить_регистры" (Процесс) - GETREGS (чтобы не
// затронуть поля, которых нет в "Регистры" - cs/ss/ds/es/fs/gs/fs_base/
// gs_base/orig_rax), затем правит только известные поля, SETREGS обратно.
func procRegsSet(pid int64, vals map[string]int64) {
	err, ok := runOnPtraceSession(pid, func() error {
		var regs syscall.PtraceRegs
		if e := syscall.PtraceGetRegs(int(pid), &regs); e != nil {
			return e
		}
		applyMapToPtraceRegs(&regs, vals)
		return syscall.PtraceSetRegs(int(pid), &regs)
	})
	if !ok || err != nil {
		panicRuntime("%s", rtmsg.ProcRegsWriteFailed)
	}
}

// procBreakpointSet - "точка_останова" (Процесс) - PTRACE_PEEKTEXT/
// POKETEXT (не process_vm_readv/writev - живой найденный баг того же
// класса, что и в cbackend, internal/backend/cbackend/procregs.go,
// package doc там: текстовый сегмент почти всегда r-xp без записи,
// process_vm_writev уважает обычные права страницы, PTRACE_POKETEXT -
// нет, специально для отладчиков). syscall.PtracePeekText/PtracePokeText
// сами разбираются с выравниванием по машинному слову - не нужно
// вручную читать/чинить слово, как в C.
func procBreakpointSet(pid int64, addr int64) {
	var orig [1]byte
	err, ok := runOnPtraceSession(pid, func() error {
		if _, e := syscall.PtracePeekText(int(pid), uintptr(addr), orig[:]); e != nil {
			return e
		}
		_, e := syscall.PtracePokeText(int(pid), uintptr(addr), []byte{0xCC})
		return e
	})
	if !ok || err != nil {
		panicRuntime("%s", rtmsg.ProcBreakpointSetFailed)
	}
	bpTable.Store(bpKey{pid, addr}, orig[0])
}

// procBreakpointRemove - "снять_точку_останова" (Процесс) - восстанавливает
// исходный байт (наилучшее старание - если процесс уже завершился, ошибку
// восстановления игнорируем, запись в таблице всё равно удалена).
func procBreakpointRemove(pid int64, addr int64) {
	v, loaded := bpTable.LoadAndDelete(bpKey{pid, addr})
	if !loaded {
		panicRuntime("%s", rtmsg.ProcBreakpointRemoveFailed)
	}
	orig := v.(byte)
	_, _ = runOnPtraceSession(pid, func() error {
		_, e := syscall.PtracePokeText(int(pid), uintptr(addr), []byte{orig})
		return e
	})
}

// procResume - "возобновить" (Процесс) - если текущий RIP стоит РОВНО на
// уже поставленной точке останова, откатывает/шагает через неё/переармирует
// (см. package doc internal/backend/cbackend/procregs.go, rech_proc_resume -
// тот же алгоритм), затем PTRACE_CONT + ожидание следующей остановки.
// Возвращает адрес остановки (RIP точки останова, скорректированный
// назад на 1 после int3) или -1, если процесс завершился.
func procResume(pid int64) int64 {
	var result int64
	err, ok := runOnPtraceSession(pid, func() error {
		var regs syscall.PtraceRegs
		if e := syscall.PtraceGetRegs(int(pid), &regs); e == nil {
			if v, found := bpTable.Load(bpKey{pid, int64(regs.Rip)}); found {
				orig := v.(byte)
				syscall.PtracePokeText(int(pid), uintptr(regs.Rip), []byte{orig})
				syscall.PtraceSingleStep(int(pid))
				var stepWs syscall.WaitStatus
				syscall.Wait4(int(pid), &stepWs, 0, nil)
				syscall.PtracePokeText(int(pid), uintptr(regs.Rip), []byte{0xCC})
			}
		}
		if e := syscall.PtraceCont(int(pid), 0); e != nil {
			return e
		}
		var ws syscall.WaitStatus
		if _, e := syscall.Wait4(int(pid), &ws, 0, nil); e != nil {
			return e
		}
		if ws.Exited() || ws.Signaled() {
			result = -1
			return nil
		}
		if ws.Stopped() && ws.StopSignal() == syscall.SIGTRAP {
			var after syscall.PtraceRegs
			if e := syscall.PtraceGetRegs(int(pid), &after); e != nil {
				return e
			}
			after.Rip--
			if e := syscall.PtraceSetRegs(int(pid), &after); e != nil {
				return e
			}
			result = int64(after.Rip)
			return nil
		}
		return fmt.Errorf("неожиданная остановка процесса")
	})
	if !ok || err != nil {
		panicRuntime("%s", rtmsg.ProcResumeFailed)
	}
	return result
}
