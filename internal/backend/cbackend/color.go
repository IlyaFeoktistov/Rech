package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// colorHelpers - "rgbцвет_из_hex(Строка): RGBЦвет"/"rgbцвет_в_hex(RGBЦвет):
// Строка" - нативная реализация того же самого, что делает Rech-библиотека
// examples/консоль/цвет.rech (её "Цвет"/"цвет_из_hex"/"цвет_в_hex" -
// пользовательские имена, СВОЯ отдельная сущность, не эта - оба имени
// намеренно НЕ переиспользованы здесь, иначе программа, использующая
// библиотеку, столкнулась бы с зарезервированным builtin-именем) - "%02x" в
// sscanf/snprintf сам разбирает/форматирует пару шестнадцатеричных цифр на
// канал, без побайтового Rech-кода. sscanf возвращает число успешно
// разобранных полей - меньше 3 (не 6 шестнадцатеричных цифр подряд, либо не
// hex-символ вовсе) - паника, а не тихо частично заполненный результат.
var colorHelpers = fmt.Sprintf(`static %[1]s rech_hex_to_color(char *hex) {
    const char *s = hex;
    if (s[0] == '#') {
        s++;
    }
    unsigned int r, g, b;
    if (strlen(s) != 6 || sscanf(s, "%%2x%%2x%%2x", &r, &g, &b) != 3) {
        rech_panic("%[5]s");
    }
    %[1]s out;
    out.%[2]s = (unsigned char)r;
    out.%[3]s = (unsigned char)g;
    out.%[4]s = (unsigned char)b;
    return out;
}

static char *rech_color_to_hex(%[1]s c) {
    char *out = malloc(8);
    snprintf(out, 8, "#%%02X%%02X%%02X", c.%[2]s, c.%[3]s, c.%[4]s);
    return out;
}
`, mangleType("RGBЦвет"), mangleField("красный"), mangleField("зелёный"), mangleField("синий"), escapeCString(rtmsg.HexColorInvalidFormat))
