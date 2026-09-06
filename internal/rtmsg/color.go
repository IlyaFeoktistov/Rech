package rtmsg

// HexColorInvalidFormat - сообщение при "rgbцвет_из_hex", если строка не
// "RRGGBB"/"#RRGGBB" (не 6 шестнадцатеричных цифр подряд после
// необязательного "#", либо среди них есть не-hex символ) - общий текст,
// один и тот же на обоих backend'ах (internal/backend/cbackend/color.go,
// internal/interp/color.go).
const HexColorInvalidFormat = "rgbцвет_из_hex: ожидалось 6 шестнадцатеричных цифр (\"RRGGBB\", опционально с \"#\")"
