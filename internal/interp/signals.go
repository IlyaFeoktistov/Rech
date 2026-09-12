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

// sigUnsupported - значение-часовой в signalOrdinalToOS (заполняется per-OS
// файлами - signals_linux.go/signals_darwin.go/signals_windows.go) для
// ordinal'ов, у которых на ЭТОЙ ОС просто нет соответствующего сигнала
// (например, SIGSTKFLT/SIGPWR - расширения Linux/glibc, отсутствуют на
// Darwin; на Windows отсутствует всё, кроме 13 сигналов, которые Go там
// определяет для совместимости, см. signals_windows.go). "получен_сигнал"/
// "отправить_сигнал" с таким ordinal'ом ведут себя как заведомо
// недостижимый сигнал - не паникуют (в отличие от SIGKILL/SIGSTOP, которые
// недостижимы по правилам самого языка, а не платформы), просто никогда не
// срабатывают/не доставляются.
const sigUnsupported = syscall.Signal(-1)

// signalOrdinalToOS - ordinal ("получен_сигнал(СигналОС.SIGTERM)") ->
// syscall.Signal, уже правильный для ТЕКУЩЕЙ платформы (в отличие от
// numeric-значений StandardEnums, которые платформо-независимый индекс, не
// настоящий номер сигнала). Заполняется per-OS файлом (signals_linux.go и
// т. д.) - набор существующих на ОС сигналов разный (SIGSTKFLT/SIGPWR нет
// на Darwin, на Windows есть только 13 из всего списка).
var signalOrdinalToOS [signalOrdinalCount]syscall.Signal

var (
	signalFlags     [signalOrdinalCount]atomic.Bool
	signalSetupOnce sync.Once
)

// ensureSignalHandlers - регистрирует signal.Notify ДЛЯ ВСЕХ поддерживаемых
// на этой ОС ordinal'ов сразу, один раз за жизнь программы - вызывается И
// "получен_сигнал", И "отправить_сигнал" ПЕРВЫМ действием. Раньше
// регистрация была ленивой и per-ordinal (внутри "получен_сигнал", только
// на первый вызов ИМЕННО этого сигнала) - живой найденный баг:
// "отправить_сигнал" себе самому РАНЬШЕ, чем хоть раз был вызван
// "получен_сигнал" с тем же сигналом (типичный порядок в самом начале
// "главная"), реально терял сигнал - Go отбрасывает сигнал, для которого на
// момент доставки нет ни одного Notify, а "получен_сигнал" регистрировался
// уже ПОСЛЕ отправки и ждал сигнал, который никогда не придёт
// (TestSignalSendDeliversToOwnProcess, воспроизводится стабильно с
// "-count=3" даже без изменений в этом файле). signal.Notify синхронен -
// после возврата ensureSignalHandlers ЛЮБОЙ дальнейший kill() гарантированно
// увидит уже настроенную подписку, "отправить_сигнал" больше не может
// обогнать её.
func ensureSignalHandlers() {
	signalSetupOnce.Do(func() {
		for idx := range signalOrdinalCount {
			sig := signalOrdinalToOS[idx]
			if sig == sigUnsupported {
				continue
			}
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, sig)
			idx := idx
			go func() {
				<-ch
				signalFlags[idx].Store(true)
			}()
		}
	})
}

// signalReceived - "получен_сигнал(сигнал: СигналОС): Логическое" - взводит
// atomic-флаг НАВСЕГДА при первом получении - не сбрасывается, тем же
// принципом, что и "запрошена_остановка".
func signalReceived(ordinal int64) bool {
	if ordinal == sigOrdSIGKILL || ordinal == sigOrdSIGSTOP {
		panicRuntime("%s", rtmsg.SignalUncatchable)
	}
	if ordinal < 0 || ordinal >= signalOrdinalCount {
		return false
	}
	if signalOrdinalToOS[ordinal] == sigUnsupported {
		return false
	}
	ensureSignalHandlers()
	return signalFlags[ordinal].Load()
}

// signalSend - "отправить_сигнал(pid: Целое, сигнал: СигналОС): Ничто" -
// kill(pid, sig) через signalKill (per-OS - см. signals_linux.go/
// signals_darwin.go/signals_windows.go), не signalReceived - НЕ паникует на
// SIGKILL/SIGSTOP (в отличие от получения, отправить их другому процессу -
// обычное дело, "kill -9"), паникует, если сама отправка не удалась (процесс
// не существует, нет прав, ordinal не поддержан этой ОС - windows умеет
// доставить только 13 из полного списка). ensureSignalHandlers ПЕРЕД
// signalKill - см. её package doc про TOCTOU-гонку, которую это закрывает.
func signalSend(pid, ordinal int64) {
	if ordinal < 0 || ordinal >= signalOrdinalCount {
		panicRuntime("%s", rtmsg.SignalSendFailed)
	}
	sig := signalOrdinalToOS[ordinal]
	if sig == sigUnsupported {
		panicRuntime("%s", rtmsg.SignalSendFailed)
	}
	ensureSignalHandlers()
	if err := signalKill(int(pid), sig); err != nil {
		panicRuntime("%s", rtmsg.SignalSendFailed)
	}
}
