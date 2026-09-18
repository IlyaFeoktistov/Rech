//go:build linux

package interp

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// keyOrdinalByEvdevCode - lookup-таблица evdev KEY_*-код (linux/input-
// event-codes.h) -> ordinal КодКлавиши (тот же принцип, что и другие
// курируемые map'ы "внешний код -> внутренний индекс" в этом компиляторе,
// см. feedback_prefer_lookup_tables_over_switch) - коды здесь ЗНАЧЕНИЯ
// ядра, не зависят от раскладки (в отличие от того, что видит терминал).
var keyOrdinalByEvdevCode = map[uint16]int64{
	30: keyOrdA, 48: keyOrdB, 46: keyOrdC, 32: keyOrdD, 18: keyOrdE,
	33: keyOrdF, 34: keyOrdG, 35: keyOrdH, 23: keyOrdI, 36: keyOrdJ,
	37: keyOrdK, 38: keyOrdL, 50: keyOrdM, 49: keyOrdN, 24: keyOrdO,
	25: keyOrdP, 16: keyOrdQ, 19: keyOrdR, 31: keyOrdS, 20: keyOrdT,
	22: keyOrdU, 47: keyOrdV, 17: keyOrdW, 45: keyOrdX, 21: keyOrdY,
	44: keyOrdZ,
	11: keyOrdDigit0, 2: keyOrdDigit1, 3: keyOrdDigit2, 4: keyOrdDigit3,
	5: keyOrdDigit4, 6: keyOrdDigit5, 7: keyOrdDigit6, 8: keyOrdDigit7,
	9: keyOrdDigit8, 10: keyOrdDigit9,
	103: keyOrdUp, 108: keyOrdDown, 105: keyOrdLeft, 106: keyOrdRight,
	57: keyOrdSpace, 28: keyOrdEnter, 1: keyOrdEscape, 15: keyOrdTab,
	14: keyOrdBackspace,
}

const (
	evKey      = 0x01
	keyMaxBits = 0x2ff // KEY_MAX (linux/input-event-codes.h)
	keyABit    = 30    // KEY_A - выбрана как "точно есть только у настоящей клавиатуры"
)

// input_event (linux/input.h) на 64-битном Linux - struct timeval (два
// long, по 8 байт каждый на этой платформе) + type/code (по 2 байта) +
// value (4 байта) = 24 байта, естественно выровнено на 8 (без хвостового
// паддинга) - тот же макет, которым уже пользуются существующие Go-
// биндинги evdev.
type linuxInputEvent struct {
	Sec, Usec  int64
	Type, Code uint16
	Value      int32
}

const linuxInputEventSize = 24

var (
	keyboardFD       = -1
	keyboardFDLookup sync.Once
)

// ioc - формула кодирования запроса ioctl (asm-generic/ioctl.h), нужна
// EVIOCGBIT (нет готовой обёртки в golang.org/x/sys/unix для этого evdev-
// специфичного запроса, в отличие от TIOCGWINSZ, использованного в
// "ширина_терминала").
func ioc(dir, typ, nr, size uint) uint {
	const (
		nrShift   = 0
		typeShift = nrShift + 8
		sizeShift = typeShift + 8
		dirShift  = sizeShift + 14
	)
	return dir<<dirShift | typ<<typeShift | nr<<nrShift | size<<sizeShift
}

func eviocgbit(ev, size uint) uint {
	const iocRead = 2
	return ioc(iocRead, 'E', 0x20+ev, size)
}

// isKeyboardDevice - EVIOCGBIT(EV_KEY, ...) на fd, проверка бита KEY_A -
// power/consumer-control устройства (тоже видны под /dev/input/eventN)
// вообще не поддерживают алфавитные клавиши, только настоящая клавиатура
// сообщает этот бит.
func isKeyboardDevice(fd int) bool {
	bits := make([]byte, keyMaxBits/8+1)
	req := eviocgbit(evKey, uint(len(bits)))
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(unsafe.Pointer(&bits[0])))
	if errno != 0 {
		return false
	}
	return bits[keyABit/8]&(1<<(keyABit%8)) != 0
}

// findKeyboardFD - сканирует /dev/input/event* один раз за жизнь
// программы (sync.Once, тот же принцип, что и ensureSignalHandlers в
// signals.go) - нет ни одного доступного устройства (обычная ситуация без
// прав root/группы "input", ВСЕГДА так внутри WSL2 - см. package doc
// StandardEnums["КодКлавиши"]) - keyboardFD остаётся -1 навсегда, "прочитать_
// код_клавиши" просто всегда возвращает -1, без паники.
func findKeyboardFD() {
	entries, err := os.ReadDir("/dev/input")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) < 5 || entry.Name()[:5] != "event" {
			continue
		}
		fd, err := unix.Open(filepath.Join("/dev/input", entry.Name()), unix.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		if isKeyboardDevice(fd) {
			keyboardFD = fd
			return
		}
		unix.Close(fd)
	}
}

// readKeyCode - "прочитать_код_клавиши(миллисекунды): КодКлавиши" на
// Linux - см. package doc keycode.go про сам контракт и StandardEnums
// ["КодКлавиши"] про то, почему именно evdev.
func readKeyCode(timeoutMs int64) int64 {
	keyboardFDLookup.Do(findKeyboardFD)
	if keyboardFD < 0 {
		return -1
	}

	var deadline time.Time
	if timeoutMs >= 0 {
		deadline = time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	}

	for {
		remaining := -1
		if timeoutMs >= 0 {
			left := time.Until(deadline)
			if left <= 0 {
				return -1
			}
			remaining = int(left.Milliseconds())
		}

		fds := []unix.PollFd{{Fd: int32(keyboardFD), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, remaining)
		if err != nil || n <= 0 || fds[0].Revents&unix.POLLIN == 0 {
			return -1
		}

		buf := make([]byte, linuxInputEventSize)
		read, err := unix.Read(keyboardFD, buf)
		if err != nil || read != linuxInputEventSize {
			return -1
		}
		ev := decodeLinuxInputEvent(buf)
		if ev.Type != evKey || (ev.Value != 1 && ev.Value != 2) {
			continue
		}
		if ord, ok := keyOrdinalByEvdevCode[ev.Code]; ok {
			return ord
		}
		// Клавиша без члена КодКлавиши (F-клавиши, модификаторы и т. п.) -
		// не ошибка, просто продолжить ждать в пределах оставшегося
		// таймаута следующее, более узнаваемое, событие.
	}
}

func decodeLinuxInputEvent(buf []byte) linuxInputEvent {
	return linuxInputEvent{
		Sec:   int64(binary.LittleEndian.Uint64(buf[0:8])),
		Usec:  int64(binary.LittleEndian.Uint64(buf[8:16])),
		Type:  binary.LittleEndian.Uint16(buf[16:18]),
		Code:  binary.LittleEndian.Uint16(buf[18:20]),
		Value: int32(binary.LittleEndian.Uint32(buf[20:24])),
	}
}
