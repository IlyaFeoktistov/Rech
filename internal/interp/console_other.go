//go:build !windows

package interp

// suppressConsoleInputNoise - см. package doc console_windows.go. На POSIX
// "можно_прочитать_байт" опрашивает poll(2) напрямую на fd (nonblocking_read_
// unix.go) - нет понятия "событие консоли", нечего подавлять.
func suppressConsoleInputNoise(fd int) {}
