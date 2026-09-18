//go:build windows

package interp

import "golang.org/x/sys/windows"

// suppressConsoleInputNoise - golang.org/x/term.MakeRaw на Windows (см.
// setRawMode) снимает ENABLE_ECHO_INPUT/ENABLE_PROCESSED_INPUT/
// ENABLE_LINE_INPUT, но НЕ трогает ENABLE_MOUSE_INPUT/ENABLE_WINDOW_INPUT -
// оба обычно включены по умолчанию у реальной консоли. Из-за этого
// "можно_прочитать_байт" (WaitForSingleObject на хэндле stdin,
// nonblocking_read_windows.go) сигнализирует готовность на ЛЮБОЕ событие
// ввода - в т. ч. движение мыши над окном консоли/изменение его размера,
// не только нажатие клавиши - тогда как последующее "прочитать" в итоге
// блокируется на настоящей клавише (ReadConsole молча поглощает не-
// клавишные записи и продолжает ждать). Снято здесь же, отдельно от
// term.MakeRaw - тот принадлежит сторонней библиотеке, дополнять его нельзя.
func suppressConsoleInputNoise(fd int) {
	h := windows.Handle(fd)
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return
	}
	_ = windows.SetConsoleMode(h, mode&^(windows.ENABLE_MOUSE_INPUT|windows.ENABLE_WINDOW_INPUT))
}
