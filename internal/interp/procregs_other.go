//go:build !linux && !windows

package interp

import "github.com/IlyaFeoktistov/Rech/internal/rtmsg"

// Платформы, кроме Linux/Windows - см. package doc internal/interp/
// procmem_other.go (тот же принцип - собирается, но паникует при первой
// же попытке реально что-то сделать).
func procRegsGet(_ int64) map[string]int64 {
	throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcRegsReadFailed)
	return nil
}
func procRegsSet(_ int64, _ map[string]int64) {
	throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcRegsWriteFailed)
}
func procBreakpointSet(_ int64, _ int64) {
	throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcBreakpointSetFailed)
}
func procBreakpointRemove(_ int64, _ int64) { panicRuntime("%s", rtmsg.ProcBreakpointRemoveFailed) }
func procResume(_ int64) int64 {
	throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcResumeFailed)
	return -1
}
