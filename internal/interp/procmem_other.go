//go:build !linux && !windows

package interp

import "github.com/IlyaFeoktistov/Rech/internal/rtmsg"

// Платформы, кроме Linux/Windows (macOS и т. д.) - собираются, но паникуют
// при первой же попытке реально подключиться к чужому процессу, тем же
// принципом, что и "отправить_сигнал" с сигналом, которого нет на текущей
// ОС (signals_darwin.go, sigUnsupported) - см. package doc internal/
// backend/cbackend/procmem.go.
func procAttach(_ int64) { throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcAttachFailed) }
func procDetach(_ int64) { throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcDetachFailed) }
func procRead(_ int64, _ int64, _ int64) []byte {
	throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcMemReadFailed)
	return nil
}
func procWrite(_ int64, _ int64, _ []byte) {
	throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcMemWriteFailed)
}
func procFind(_ int64, _ []byte) []int64 {
	throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcAttachFailed)
	return nil
}
