package interp

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// Ordinal каждого сигнала - тот же порядок, что и StandardEnums["СигналОС"]
// (internal/semantic/types.go) - синхронизировать оба списка при изменении
// одного (тот же принцип соответствия, что уже есть между cbackend/
// internal/backend/cbackend/signals.go и этим файлом).
const (
	sigOrdSIGHUP = iota
	sigOrdSIGINT
	sigOrdSIGQUIT
	sigOrdSIGABRT
	sigOrdSIGKILL
	sigOrdSIGUSR1
	sigOrdSIGUSR2
	sigOrdSIGPIPE
	sigOrdSIGALRM
	sigOrdSIGTERM
	sigOrdSIGCHLD
	sigOrdSIGCONT
	sigOrdSIGSTOP
	sigOrdSIGTSTP
	sigOrdSIGWINCH
	sigOrdSIGILL
	sigOrdSIGTRAP
	sigOrdSIGBUS
	sigOrdSIGFPE
	sigOrdSIGSEGV
	sigOrdSIGSTKFLT
	sigOrdSIGTTIN
	sigOrdSIGTTOU
	sigOrdSIGURG
	sigOrdSIGXCPU
	sigOrdSIGXFSZ
	sigOrdSIGVTALRM
	sigOrdSIGPROF
	sigOrdSIGIO
	sigOrdSIGPWR
	sigOrdSIGSYS
	signalOrdinalCount
)

// signalOrdinalToOS - ordinal ("получен_сигнал(СигналОС.SIGTERM)") ->
// syscall.Signal, уже правильный для ТЕКУЩЕЙ платформы (в отличие от
// numeric-значений StandardEnums, которые платформо-независимый индекс, не
// настоящий номер сигнала - syscall.SIGUSR1 и т. п. сам разрешается Go
// компилятором в верное число для GOOS/GOARCH). SIGKILL/SIGSTOP ЗДЕСЬ ЕСТЬ
// (в отличие от прежней версии) - "отправить_сигнал" использует ту же
// таблицу для ЛЮБОГО ordinal (kill() отправить их можно, только перехватить
// самому нельзя) - "получен_сигнал" по-прежнему паникует на них РАНЬШЕ, чем
// заглянуть в таблицу (signalReceived).
var signalOrdinalToOS = [signalOrdinalCount]syscall.Signal{
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

var (
	signalFlags [signalOrdinalCount]atomic.Bool
	signalOnce  [signalOrdinalCount]sync.Once
)

// signalReceived - "получен_сигнал(сигнал: СигналОС): Логическое" -
// регистрирует (лениво, один раз за сигнал - sync.Once, тот же принцип, что
// ensureShutdownSignalHandler в shutdown.go) горутину, ждущую ИМЕННО этот
// сигнал, и взводит atomic-флаг НАВСЕГДА при первом получении - не
// сбрасывается, тем же принципом, что и "запрошена_остановка".
func signalReceived(ordinal int64) bool {
	if ordinal == sigOrdSIGKILL || ordinal == sigOrdSIGSTOP {
		panicRuntime("%s", rtmsg.SignalUncatchable)
	}
	if ordinal < 0 || ordinal >= signalOrdinalCount {
		return false
	}
	idx := int(ordinal)
	signalOnce[idx].Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, signalOrdinalToOS[idx])
		go func() {
			<-ch
			signalFlags[idx].Store(true)
		}()
	})
	return signalFlags[idx].Load()
}

// signalSend - "отправить_сигнал(pid: Целое, сигнал: СигналОС): Ничто" -
// kill(pid, sig), не signalReceived - НЕ паникует на SIGKILL/SIGSTOP (в
// отличие от получения, отправить их другому процессу - обычное дело,
// "kill -9"), паникует только если сама отправка не удалась (процесс не
// существует - ESRCH, нет прав - EPERM).
func signalSend(pid, ordinal int64) {
	if ordinal < 0 || ordinal >= signalOrdinalCount {
		panicRuntime("%s", rtmsg.SignalSendFailed)
	}
	if err := syscall.Kill(int(pid), signalOrdinalToOS[ordinal]); err != nil {
		panicRuntime("%s", rtmsg.SignalSendFailed)
	}
}
