// Package numbase, продолжение - см. package doc internal/semantic/
// numbase.go. Разбор префикса - вручную по ДВУМ конкретным символам
// ("0"+x/X, o/O, b/B), не strncasecmp - непортируемая libc-функция (нет
// под этим именем на Windows/MSVC), которой в рантайме до сих пор нигде
// нет (тот же принцип, что и у rech_cookie_value/rech_header_value,
// runtime/httpwire/httpheaders.go, runtime/cookies).
package numbase

import "fmt"

// ToStringHelpers - "целое_в_hex"/"целое_в_oct"/"целое_в_bin" - форматируют
// БИТОВЫЙ ПАТТЕРН n (unsigned long long, не величину - см. package doc
// numbase.go). Двоичная форма - вручную (нет printf-спецификатора для
// base 2, в отличие от "%llx"/"%llo").
const ToStringHelpers = `static char *rech_int_to_hex(long long n) {
    char *out = GC_MALLOC(20);
    snprintf(out, 20, "0x%llx", (unsigned long long)n);
    return out;
}

static char *rech_int_to_oct(long long n) {
    char *out = GC_MALLOC(24);
    snprintf(out, 24, "0o%llo", (unsigned long long)n);
    return out;
}

static char *rech_int_to_bin(long long n) {
    unsigned long long u = (unsigned long long)n;
    char digits[64];
    int i = 64;
    if (u == 0) {
        digits[--i] = '0';
    } else {
        while (u > 0) {
            digits[--i] = (char)('0' + (int)(u & 1));
            u >>= 1;
        }
    }
    char *out = GC_MALLOC((size_t)(64 - i) + 3);
    out[0] = '0';
    out[1] = 'b';
    memcpy(out + 2, digits + i, (size_t)(64 - i));
    out[2 + (64 - i)] = '\0';
    return out;
}

`

// FromStringHelpers - "целое_из_hex"/"целое_из_oct"/"целое_из_bin" -
// необязательный префикс (регистронезависимо - проверка по КОНКРЕТНОЙ
// букве в обоих регистрах, не strncasecmp, см. package doc), затем цифры
// своей системы счисления через strtoull (bit-паттерн, не проверка
// величины) - endptr == digits (ни одной цифры) или *endptr != '\0'
// (лишний символ после цифр) - ошибка, тем же принципом, что и
// rech_read_int (builtins_shared.go). hexMsg/octMsg/binMsg - уже готовые
// шаблоны сообщений об ошибке (см. MsgFormat ниже), вычисленные вызывающей
// стороной.
func FromStringHelpers(hexMsg, octMsg, binMsg string) string {
	return fmt.Sprintf(`static long long rech_int_from_hex(const char *s) {
    const char *digits = s;
    if (digits[0] == '0' && (digits[1] == 'x' || digits[1] == 'X')) {
        digits += 2;
    }
    char *end;
    unsigned long long v = strtoull(digits, &end, 16);
    if (end == digits || *end != '\0') {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[1]s", s);
        rech_panic(msg);
    }
    return (long long)v;
}

static long long rech_int_from_oct(const char *s) {
    const char *digits = s;
    if (digits[0] == '0' && (digits[1] == 'o' || digits[1] == 'O')) {
        digits += 2;
    }
    char *end;
    unsigned long long v = strtoull(digits, &end, 8);
    if (end == digits || *end != '\0') {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[2]s", s);
        rech_panic(msg);
    }
    return (long long)v;
}

static long long rech_int_from_bin(const char *s) {
    const char *digits = s;
    if (digits[0] == '0' && (digits[1] == 'b' || digits[1] == 'B')) {
        digits += 2;
    }
    char *end;
    unsigned long long v = strtoull(digits, &end, 2);
    if (end == digits || *end != '\0') {
        char msg[256];
        snprintf(msg, sizeof(msg), "%[3]s", s);
        rech_panic(msg);
    }
    return (long long)v;
}

`, hexMsg, octMsg, binMsg)
}
