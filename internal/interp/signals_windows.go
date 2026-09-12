//go:build windows

package interp

import (
	"syscall"

	"golang.org/x/sys/windows"
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
// вовсе, в отличие от Linux/Darwin) - настоящей ДОСТАВКИ произвольного
// сигнала (в POSIX-смысле - перехватываемого получателем через
// "получен_сигнал") произвольному pid на Windows нет в принципе (см.
// package doc выше про 13 sig-имён, которые Go определяет лишь для
// исходной совместимости). Для ЧУЖОГО процесса (не себя самого) у "kill"
// есть только ОДНО настоящее исполнимое действие на Windows - остановить
// его целиком: TerminateProcess (тот же примитив, которым Go сам реализует
// (*os.Process).Kill() на этой ОС) - соответствует POSIX-конвенции "128 +
// номер сигнала" для кода завершения (WaitStatus.Signaled() у Go на
// Windows всегда false, см. syscall/syscall_windows.go - "дождаться_
// процесса", signals.go, читает ExitCode() напрямую, поэтому именно ЭТОТ
// код возврата и должен туда попасть). Разница с Unix - там ЛЮБОЙ сигнал
// из 13 (включая, например, SIGHUP) реально доставляется и может быть
// перехвачен получателем; здесь - только останавливает его безусловно,
// независимо от того, какой из 13 запрошен. Для СЕБЯ САМОГО (pid ==
// os.Getpid()) это означало бы самозавершение вместо доставки - не то,
// что нужно "получен_сигнал"-циклу (TestSignalSendDeliversToOwnProcess) -
// но там ordinal - SIGUSR2, который на Windows sigUnsupported (signalSend,
// signals.go, паникует ДО вызова этой функции), так что этот случай сюда
// и не доходит.
func signalKill(pid int, sig syscall.Signal) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, uint32(128+int(sig)))
}
