package cbackend

// floatFormatHelper - rech_fmt_float: та же запись дробного, что и
// internal/numfmt.Float в интерпретаторе (кратчайшая, читающаяся обратно в
// то же значение; порядок от -4 до 15 - обычная запись, иначе
// экспонента). Цифры - наименьшая точность %.*e, при которой strtod/strtof
// возвращает то же число. Выводится всегда: печать, "(Строка) x",
// "показать" и сериализаторы JSON/XML/YAML пользуются одним хелпером.
const floatFormatHelper = `#include <stdio.h>
#include <stdlib.h>
/* rech_fmt_float - запись v в buf (не меньше 48 байт); f32 - значение
   Дробное (float): цифры подбираются по точности float. */
static inline const char *rech_fmt_float(char *buf, double v, int f32) {
    char e[40], digits[24], *o = buf;
    const char *s = e;
    int p, nd = 0, exp, i;
    if (v != v) { buf[0] = 'n'; buf[1] = 'a'; buf[2] = 'n'; buf[3] = 0; return buf; }
    if (v > 1.7976931348623157e308) { snprintf(buf, 48, "inf"); return buf; }
    if (v < -1.7976931348623157e308) { snprintf(buf, 48, "-inf"); return buf; }
    for (p = 1; p <= 17; p++) {
        snprintf(e, sizeof e, "%.*e", p - 1, v);
        if (f32 ? strtof(e, NULL) == (float)v : strtod(e, NULL) == v) {
            break;
        }
    }
    if (*s == '-') { *o++ = '-'; s++; }
    while (*s && *s != 'e') {
        if (*s != '.') { digits[nd++] = *s; }
        s++;
    }
    exp = atoi(s + 1);
    while (nd > 1 && digits[nd - 1] == '0') { nd--; }
    if (exp >= -4 && exp < 16) {
        if (exp >= 0) {
            for (i = 0; i <= exp; i++) { *o++ = i < nd ? digits[i] : '0'; }
            if (nd > exp + 1) {
                *o++ = '.';
                for (i = exp + 1; i < nd; i++) { *o++ = digits[i]; }
            }
        } else {
            *o++ = '0'; *o++ = '.';
            for (i = 0; i < -exp - 1; i++) { *o++ = '0'; }
            for (i = 0; i < nd; i++) { *o++ = digits[i]; }
        }
        *o = 0;
        return buf;
    }
    *o++ = digits[0];
    if (nd > 1) {
        *o++ = '.';
        for (i = 1; i < nd; i++) { *o++ = digits[i]; }
    }
    snprintf(o, 8, "e%c%02d", exp < 0 ? '-' : '+', exp < 0 ? -exp : exp);
    return buf;
}

`

// float32Flag - аргумент f32 у rech_fmt_float/rech_cast_float_to_str.
func float32Flag(isFloat32 bool) int {
	if isFloat32 {
		return 1
	}
	return 0
}
