//go:build windows

package interp

import (
	"os"

	"golang.org/x/sys/windows"
)

// canRead - Windows не знает poll(2) (см. package doc
// nonblocking_read_unix.go) - windows.WaitForSingleObject на самом
// handle'е файла работает для console/pipe-хендлов (stdin в интерактивном
// терминале - именно ради него и заведён "можно_прочитать_байт", см.
// examples/основы/клавиши.rech) настоящим ожиданием готовности, не
// заглушкой: обычный файл на диске Windows считает готовым немедленно (что
// корректно - чтение из него и так не блокируется сколь-нибудь заметно).
func canRead(f *os.File, timeoutMs int64) bool {
	timeout := uint32(windows.INFINITE)
	if timeoutMs >= 0 {
		timeout = uint32(timeoutMs)
	}
	event, err := windows.WaitForSingleObject(windows.Handle(f.Fd()), timeout)
	return err == nil && event == windows.WAIT_OBJECT_0
}
