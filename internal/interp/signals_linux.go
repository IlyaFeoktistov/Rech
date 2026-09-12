//go:build linux

package interp

import "syscall"

// signalOrdinalToOS - полный список: все сигналы StandardEnums["СигналОС"]
// существуют на Linux (в отличие от Darwin - нет SIGSTKFLT/SIGPWR - и
// Windows - есть только 13 базовых, см. signals_darwin.go/
// signals_windows.go).
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
		sigOrdSIGSTKFLT: syscall.SIGSTKFLT,
		sigOrdSIGTTIN:   syscall.SIGTTIN,
		sigOrdSIGTTOU:   syscall.SIGTTOU,
		sigOrdSIGURG:    syscall.SIGURG,
		sigOrdSIGXCPU:   syscall.SIGXCPU,
		sigOrdSIGXFSZ:   syscall.SIGXFSZ,
		sigOrdSIGVTALRM: syscall.SIGVTALRM,
		sigOrdSIGPROF:   syscall.SIGPROF,
		sigOrdSIGIO:     syscall.SIGIO,
		sigOrdSIGPWR:    syscall.SIGPWR,
		sigOrdSIGSYS:    syscall.SIGSYS,
	}
}

func signalKill(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
