//go:build windows

package interp

import (
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// canRead - Windows не знает poll(2) (см. package doc
// nonblocking_read_unix.go). Два РАЗНЫХ вида хендлов нужно обрабатывать
// по-разному - GetFileType их различает:
//
//   - Консольный ввод (stdin в интерактивном терминале - именно ради него и
//     заведён "можно_прочитать_байт", см. examples/основы/клавиши.rech) -
//     windows.WaitForSingleObject реально становится сигнальным при новом
//     событии ввода, настоящее ожидание готовности, не заглушка.
//
//   - Пайп/обычный файл (в т. ч. ЛЮБОЙ os.Pipe() - используется тестами
//     этого файла) - у СИНХРОННОГО (не overlapped) хендла нет сигнального
//     состояния "данные пришли" вообще, WaitForSingleObject на нём либо не
//     поддержан, либо (что и подтвердил живой прогон на Windows CI) молча
//     ждёт до истечения ВСЕГО таймаута и возвращает WAIT_TIMEOUT
//     БЕЗУСЛОВНО, даже когда данные уже в пайпе - "можно_прочитать_байт"
//     был неработоспособен для пайпов на Windows целиком (не
//     останавливался на false раньше срока, а именно никогда не видел
//     готовые данные). Единственный документированный способ проверить
//     готовность пайпа без overlapped I/O - PeekNamedPipe (недостаточно
//     обёрнут в golang.org/x/sys/windows, вызывается напрямую через
//     kernel32.dll), опрашивается в цикле с коротким интервалом до
//     истечения таймаута - обычный файл на диске считается готовым сразу,
//     чтение из него и так не блокируется сколь-нибудь заметно.
func canRead(f *os.File, timeoutMs int64) bool {
	h := windows.Handle(f.Fd())
	ft, ftErr := windows.GetFileType(h)
	if ftErr == nil && ft == windows.FILE_TYPE_CHAR {
		timeout := uint32(windows.INFINITE)
		if timeoutMs >= 0 {
			timeout = uint32(timeoutMs)
		}
		event, err := windows.WaitForSingleObject(h, timeout)
		return err == nil && event == windows.WAIT_OBJECT_0
	}
	if ftErr == nil && ft != windows.FILE_TYPE_PIPE {
		// Обычный файл на диске - не пайп и не консоль.
		return true
	}

	const pollInterval = 5 * time.Millisecond
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for {
		n, err := peekNamedPipeBytesAvail(h)
		if err != nil {
			// Труба разорвана/закрыта с другой стороны - последующее
			// чтение не заблокируется (вернёт EOF немедленно), значит
			// формально "готова".
			return true
		}
		if n > 0 {
			return true
		}
		if timeoutMs >= 0 && !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(pollInterval)
	}
}

var (
	modKernel32       = windows.NewLazySystemDLL("kernel32.dll")
	procPeekNamedPipe = modKernel32.NewProc("PeekNamedPipe")
)

// peekNamedPipeBytesAvail - PeekNamedPipe(hPipe, NULL, 0, NULL,
// &lpTotalBytesAvail, NULL) - сколько байт можно прочитать из пайпа прямо
// сейчас, не потребляя их (в отличие от настоящего Read).
func peekNamedPipeBytesAvail(h windows.Handle) (uint32, error) {
	var totalBytesAvail uint32
	r1, _, err := procPeekNamedPipe.Call(
		uintptr(h),
		0, 0, 0,
		uintptr(unsafe.Pointer(&totalBytesAvail)),
		0,
	)
	if r1 == 0 {
		return 0, err
	}
	return totalBytesAvail, nil
}
