//go:build windows

package interp

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// keyOrdinalByVK - lookup-таблица virtual-key код (winuser.h) -> ordinal
// КодКлавиши (тот же принцип, что и keyOrdinalByEvdevCode на Linux) - VK_A..
// VK_Z/VK_0..VK_9 у стандартных layout-DLL Windows совпадают с ASCII-кодом
// буквы/цифры НЕЗАВИСИМО от текущей раскладки (см. package doc
// StandardEnums["КодКлавиши"] - та же причина, по которой GetAsyncKeyState
// (VK_W) в играх работает на любой раскладке).
var keyOrdinalByVK = map[uint16]int64{
	'A': keyOrdA, 'B': keyOrdB, 'C': keyOrdC, 'D': keyOrdD, 'E': keyOrdE,
	'F': keyOrdF, 'G': keyOrdG, 'H': keyOrdH, 'I': keyOrdI, 'J': keyOrdJ,
	'K': keyOrdK, 'L': keyOrdL, 'M': keyOrdM, 'N': keyOrdN, 'O': keyOrdO,
	'P': keyOrdP, 'Q': keyOrdQ, 'R': keyOrdR, 'S': keyOrdS, 'T': keyOrdT,
	'U': keyOrdU, 'V': keyOrdV, 'W': keyOrdW, 'X': keyOrdX, 'Y': keyOrdY,
	'Z': keyOrdZ,
	'0': keyOrdDigit0, '1': keyOrdDigit1, '2': keyOrdDigit2, '3': keyOrdDigit3,
	'4': keyOrdDigit4, '5': keyOrdDigit5, '6': keyOrdDigit6, '7': keyOrdDigit7,
	'8': keyOrdDigit8, '9': keyOrdDigit9,
	0x26: keyOrdUp, 0x28: keyOrdDown, 0x25: keyOrdLeft, 0x27: keyOrdRight, // VK_UP/DOWN/LEFT/RIGHT
	0x20: keyOrdSpace, 0x0D: keyOrdEnter, 0x1B: keyOrdEscape, // VK_SPACE/RETURN/ESCAPE
	0x09: keyOrdTab, 0x08: keyOrdBackspace, // VK_TAB/BACK
}

const keyEventType = 0x0001 // KEY_EVENT (wincon.h)

// keyEventRecord - KEY_EVENT_RECORD (wincon.h), только поля, нужные здесь -
// смещения совпадают с реальным макетом C-структуры (BOOL(4) + три WORD(2)
// + WCHAR(2) + DWORD(4), 16 байт), unicodeChar/controlKeyState не читаются.
type keyEventRecord struct {
	bKeyDown          int32
	wRepeatCount      uint16
	wVirtualKeyCode   uint16
	wVirtualScanCode  uint16
	unicodeChar       uint16
	dwControlKeyState uint32
}

// inputRecord - INPUT_RECORD (wincon.h) - только ветка KeyEvent нужна здесь
// (EventType выбирает, какой вариант объединения валиден - остальные
// молча игнорируются readKeyCode ниже).
type inputRecord struct {
	eventType uint16
	_         uint16 // паддинг до 4-байтной границы (union внутри требует его из-за DWORD-полей)
	keyEvent  keyEventRecord
}

var (
	modKernel32ReadConsole = windows.NewLazySystemDLL("kernel32.dll")
	procReadConsoleInputW  = modKernel32ReadConsole.NewProc("ReadConsoleInputW")
)

func readConsoleInput(h windows.Handle) (inputRecord, bool) {
	var rec inputRecord
	var eventsRead uint32
	r1, _, _ := procReadConsoleInputW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&rec)),
		1,
		uintptr(unsafe.Pointer(&eventsRead)),
	)
	return rec, r1 != 0 && eventsRead > 0
}

// readKeyCode - "прочитать_код_клавиши(миллисекунды): КодКлавиши" на
// Windows - см. package doc keycode.go про сам контракт и StandardEnums
// ["КодКлавиши"] про то, почему именно ReadConsoleInput/wVirtualKeyCode.
// Требует консольный stdin ("установить_сырой_режим(истина)" уже снимает
// ENABLE_MOUSE_INPUT/ENABLE_WINDOW_INPUT - см. suppressConsoleInputNoise -
// без этого тут пришлось бы самим пропускать намного больше не-клавишных
// записей) - на перенаправленном в файл/пайп stdin WaitForSingleObject
// ниже никогда не станет сигнальным, "прочитать_код_клавиши" тогда просто
// ждёт до истечения таймаута и возвращает -1, тем же поведением, что и
// "можно_прочитать_байт" на таком же хэндле.
func readKeyCode(timeoutMs int64) int64 {
	h, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return -1
	}

	var deadline time.Time
	if timeoutMs >= 0 {
		deadline = time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	}

	for {
		timeout := uint32(windows.INFINITE)
		if timeoutMs >= 0 {
			left := time.Until(deadline)
			if left <= 0 {
				return -1
			}
			timeout = uint32(left.Milliseconds())
		}

		event, err := windows.WaitForSingleObject(h, timeout)
		if err != nil || event != windows.WAIT_OBJECT_0 {
			return -1
		}

		rec, ok := readConsoleInput(h)
		if !ok {
			return -1
		}
		if rec.eventType != keyEventType || rec.keyEvent.bKeyDown == 0 {
			continue
		}
		if ord, ok := keyOrdinalByVK[rec.keyEvent.wVirtualKeyCode]; ok {
			return ord
		}
	}
}
