package interp

// Ordinal каждого члена "КодКлавиши" - тот же порядок, что и
// StandardEnums["КодКлавиши"] (internal/semantic/types.go) - синхронизировать
// оба списка при изменении одного, тот же принцип соответствия, что уже
// есть у сигналов (signals.go/signalOrdinalToOS).
//
// readKeyCode(timeoutMs) - платформенная реализация (keycode_linux.go/
// keycode_windows.go/keycode_other.go) - "прочитать_код_клавиши": ждёт
// нажатие клавиши не дольше timeoutMs мс (отрицательное - бесконечно),
// возвращает её ordinal или -1 (ничего не пришло за отведённое время, ИЛИ
// раскладко-независимое чтение недоступно вовсе на этой платформе/без
// нужных прав - см. package doc StandardEnums["КодКлавиши"]) - никогда не
// паникует.
const (
	keyOrdA = iota
	keyOrdB
	keyOrdC
	keyOrdD
	keyOrdE
	keyOrdF
	keyOrdG
	keyOrdH
	keyOrdI
	keyOrdJ
	keyOrdK
	keyOrdL
	keyOrdM
	keyOrdN
	keyOrdO
	keyOrdP
	keyOrdQ
	keyOrdR
	keyOrdS
	keyOrdT
	keyOrdU
	keyOrdV
	keyOrdW
	keyOrdX
	keyOrdY
	keyOrdZ
	keyOrdDigit0
	keyOrdDigit1
	keyOrdDigit2
	keyOrdDigit3
	keyOrdDigit4
	keyOrdDigit5
	keyOrdDigit6
	keyOrdDigit7
	keyOrdDigit8
	keyOrdDigit9
	keyOrdUp
	keyOrdDown
	keyOrdLeft
	keyOrdRight
	keyOrdSpace
	keyOrdEnter
	keyOrdEscape
	keyOrdTab
	keyOrdBackspace
)
