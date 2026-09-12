//go:build windows

package interp

import (
	"errors"
	"syscall"
)

// signalOrdinalToOS - Windows не знает POSIX-сигналов вообще, Go определяет
// (для исходной совместимости, package doc syscall/types_windows.go) только
// 13 имён: SIGHUP/SIGINT/SIGQUIT/SIGILL/SIGTRAP/SIGABRT/SIGBUS/SIGFPE/
// SIGKILL/SIGSEGV/SIGPIPE/SIGALRM/SIGTERM. Остального в списке
// StandardEnums["СигналОС"] на Windows просто нет - sigUnsupported (см.
// signals.go) - "получен_сигнал" с ними никогда не сработает, "отправить_
// сигнал" паникует (rtmsg.SignalSendFailed). Даже среди этих 13 РЕАЛЬНО
// доставляется через os/signal только SIGINT (Ctrl+C, консольное событие) -
// остальные определены лишь для компиляции, ОС их не генерирует - это уже
// ограничение самого Go на Windows, не решается на нашей стороне.
func init() {
	signalOrdinalToOS = [signalOrdinalCount]syscall.Signal{
		sigOrdSIGHUP:    syscall.SIGHUP,
		sigOrdSIGINT:    syscall.SIGINT,
		sigOrdSIGQUIT:   syscall.SIGQUIT,
		sigOrdSIGABRT:   syscall.SIGABRT,
		sigOrdSIGKILL:   syscall.SIGKILL,
		sigOrdSIGUSR1:   sigUnsupported,
		sigOrdSIGUSR2:   sigUnsupported,
		sigOrdSIGPIPE:   syscall.SIGPIPE,
		sigOrdSIGALRM:   syscall.SIGALRM,
		sigOrdSIGTERM:   syscall.SIGTERM,
		sigOrdSIGCHLD:   sigUnsupported,
		sigOrdSIGCONT:   sigUnsupported,
		sigOrdSIGSTOP:   sigUnsupported,
		sigOrdSIGTSTP:   sigUnsupported,
		sigOrdSIGWINCH:  sigUnsupported,
		sigOrdSIGILL:    syscall.SIGILL,
		sigOrdSIGTRAP:   syscall.SIGTRAP,
		sigOrdSIGBUS:    syscall.SIGBUS,
		sigOrdSIGFPE:    syscall.SIGFPE,
		sigOrdSIGSEGV:   syscall.SIGSEGV,
		sigOrdSIGSTKFLT: sigUnsupported,
		sigOrdSIGTTIN:   sigUnsupported,
		sigOrdSIGTTOU:   sigUnsupported,
		sigOrdSIGURG:    sigUnsupported,
		sigOrdSIGXCPU:   sigUnsupported,
		sigOrdSIGXFSZ:   sigUnsupported,
		sigOrdSIGVTALRM: sigUnsupported,
		sigOrdSIGPROF:   sigUnsupported,
		sigOrdSIGIO:     sigUnsupported,
		sigOrdSIGPWR:    sigUnsupported,
		sigOrdSIGSYS:    sigUnsupported,
	}
}

// signalKill - Windows не знает kill(2) (syscall.Kill там не существует
// вовсе, в отличие от Linux/Darwin) - настоящей отправки произвольного
// сигнала произвольному pid на Windows нет в принципе (см. package doc
// выше), "отправить_сигнал" здесь всегда паникует rtmsg.SignalSendFailed.
func signalKill(pid int, sig syscall.Signal) error {
	return errors.New("signal delivery is not supported on Windows")
}
