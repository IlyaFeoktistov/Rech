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
	signalOrdinalCount
)

// signalOrdinalToOS - ordinal ("получен_сигнал(СигналОС.SIGTERM)") ->
// syscall.Signal, уже правильный для ТЕКУЩЕЙ платформы (в отличие от
// numeric-значений StandardEnums, которые платформо-независимый индекс, не
// настоящий номер сигнала - syscall.SIGUSR1 и т. п. сам разрешается Go
// компилятором в верное число для GOOS/GOARCH). SIGKILL/SIGSTOP - паникуют
// раньше (signalReceived), их позиции здесь не используются вовсе.
var signalOrdinalToOS = [signalOrdinalCount]syscall.Signal{
	sigOrdSIGHUP:   syscall.SIGHUP,
	sigOrdSIGINT:   syscall.SIGINT,
	sigOrdSIGQUIT:  syscall.SIGQUIT,
	sigOrdSIGABRT:  syscall.SIGABRT,
	sigOrdSIGUSR1:  syscall.SIGUSR1,
	sigOrdSIGUSR2:  syscall.SIGUSR2,
	sigOrdSIGPIPE:  syscall.SIGPIPE,
	sigOrdSIGALRM:  syscall.SIGALRM,
	sigOrdSIGTERM:  syscall.SIGTERM,
	sigOrdSIGCHLD:  syscall.SIGCHLD,
	sigOrdSIGCONT:  syscall.SIGCONT,
	sigOrdSIGTSTP:  syscall.SIGTSTP,
	sigOrdSIGWINCH: syscall.SIGWINCH,
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
