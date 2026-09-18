//go:build !linux && !windows

package interp

// readKeyCode - раскладко-независимое чтение клавиши не реализовано на
// этой платформе (macOS - отдельная задача, требует IOKit HID/NSEvent.
// keyCode, см. package doc keycode.go/StandardEnums["КодКлавиши"]) -
// всегда -1, тем же принципом, что и Linux без прав на /dev/input - не
// паника.
func readKeyCode(timeoutMs int64) int64 {
	return -1
}
