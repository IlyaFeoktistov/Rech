//go:build unix

package interp

import (
	"os"

	"golang.org/x/sys/unix"
)

// canRead - "можно_прочитать_байт(файл: Файл, миллисекунды: Целое): Логическое" -
// unix.Poll вместо os.File.SetReadDeadline: обычные *os.File (stdin/файлы,
// открытые "открыть"/"открыть_для_записи") НЕ поддерживают дедлайны в Go
// ("file type does not support deadline") - они не заведены во внутренний
// netpoller рантайма, в отличие от сокетов - poll(2) напрямую работает с
// ЛЮБЫМ файловым дескриптором, тот же путь, что и cbackend (rech_can_read,
// package doc nonblocking_read.go). Отрицательный timeoutMs - ждать
// бесконечно (unix.Poll(-1) - тот же принцип, что и у "poll" POSIX).
func canRead(f *os.File, timeoutMs int64) bool {
	timeout := -1
	if timeoutMs >= 0 {
		timeout = int(timeoutMs)
	}
	fds := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, timeout)
	return err == nil && n > 0 && fds[0].Revents&unix.POLLIN != 0
}
