//go:build darwin

package interp

import "syscall"

// signalOrdinalToOS - как в signals_linux.go, но БЕЗ SIGSTKFLT/SIGPWR -
// это расширения Linux/glibc, Darwin (BSD-сигналы) их не знает вовсе - оба
// ordinal'а помечены sigUnsupported (see signals.go) - "получен_сигнал"/
// "отправить_сигнал" с ними на macOS ведут себя как заведомо недостижимый
// сигнал, не паникуют.
func init() {
	signalOrdinalToOS = [signalOrdinalCount]syscall.Signal{
		sigOrdSIGHUP:    syscall.SIGHUP,
		sigOrdSIGINT:    syscall.SIGINT,
		sigOrdSIGQUIT:   syscall.SIGQUIT,
		sigOrdSIGABRT:   syscall.SIGABRT,
		sigOrdSIGKILL:   syscall.SIGKILL,
		sigOrdSIGUSR1:   syscall.SIGUSR1,
		sigOrdSIGUSR2:   syscall.SIGUSR2,
		sigOrdSIGPIPE:   syscall.SIGPIPE,
		sigOrdSIGALRM:   syscall.SIGALRM,
		sigOrdSIGTERM:   syscall.SIGTERM,
		sigOrdSIGCHLD:   syscall.SIGCHLD,
		sigOrdSIGCONT:   syscall.SIGCONT,
		sigOrdSIGSTOP:   syscall.SIGSTOP,
		sigOrdSIGTSTP:   syscall.SIGTSTP,
		sigOrdSIGWINCH:  syscall.SIGWINCH,
		sigOrdSIGILL:    syscall.SIGILL,
		sigOrdSIGTRAP:   syscall.SIGTRAP,
		sigOrdSIGBUS:    syscall.SIGBUS,
		sigOrdSIGFPE:    syscall.SIGFPE,
		sigOrdSIGSEGV:   syscall.SIGSEGV,
		sigOrdSIGSTKFLT: sigUnsupported,
		sigOrdSIGTTIN:   syscall.SIGTTIN,
		sigOrdSIGTTOU:   syscall.SIGTTOU,
		sigOrdSIGURG:    syscall.SIGURG,
		sigOrdSIGXCPU:   syscall.SIGXCPU,
		sigOrdSIGXFSZ:   syscall.SIGXFSZ,
		sigOrdSIGVTALRM: syscall.SIGVTALRM,
		sigOrdSIGPROF:   syscall.SIGPROF,
		sigOrdSIGIO:     syscall.SIGIO,
		sigOrdSIGPWR:    sigUnsupported,
		sigOrdSIGSYS:    syscall.SIGSYS,
	}
}

func signalKill(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
