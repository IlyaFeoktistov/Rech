package cbackend

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// decimalRuntime - "Десятичное" на C: тот же алгоритм, что
// internal/decimal (мантисса до 2^96 - 1, масштаб 0..28, однократное
// банковское округление; промежуточные значения - 192-битные целые
// rech_u192). Любое изменение правила - в обоих местах.
var decimalRuntime = strings.NewReplacer(
	"RECH_DEC_OVERFLOW", panicCallC(rtmsg.PanicEntity, `"`+escapeCString(rtmsg.DecimalOverflow)+`"`),
	"RECH_DEC_DIVZERO", panicCallC(rtmsg.PanicDivisionByZero, `"`+escapeCString(rtmsg.DivisionByZero)+`"`),
	"RECH_DEC_PARSE_FAIL(msg)", errorCallC(rtmsg.ErrorParse, "msg"),
	"RECH_DEC_BADCAST(msg)", panicCallC(rtmsg.PanicBadCast, "msg"),
).Replace(decimalRuntimeText)

// decimalTypedef - тип rech_dec: выводится всегда, в начале файла (поле
// Десятичное у структуры объявляется раньше любого хелпера); нулевое
// значение - 0.
const decimalTypedef = "typedef struct { uint64_t lo; uint32_t hi; uint8_t scale; uint8_t neg; } rech_dec;\n"

const decimalRuntimeText = `#include <string.h>
#include <stdlib.h>
typedef struct { uint64_t w[3]; } rech_u192;

static rech_u192 rech_u192_of(rech_dec d) { rech_u192 r = {{d.lo, d.hi, 0}}; return r; }
static int rech_u192_zero(rech_u192 a) { return (a.w[0] | a.w[1] | a.w[2]) == 0; }
static int rech_u192_cmp(rech_u192 a, rech_u192 b) {
    for (int i = 2; i >= 0; i--) {
        if (a.w[i] != b.w[i]) { return a.w[i] < b.w[i] ? -1 : 1; }
    }
    return 0;
}
static rech_u192 rech_u192_add(rech_u192 a, rech_u192 b) {
    unsigned __int128 c = 0;
    for (int i = 0; i < 3; i++) { c += (unsigned __int128)a.w[i] + b.w[i]; a.w[i] = (uint64_t)c; c >>= 64; }
    return a;
}
static rech_u192 rech_u192_sub(rech_u192 a, rech_u192 b) {
    uint64_t borrow = 0;
    for (int i = 0; i < 3; i++) {
        uint64_t x = a.w[i] - b.w[i] - borrow;
        borrow = (a.w[i] < b.w[i] || (a.w[i] == b.w[i] && borrow)) ? 1 : 0;
        a.w[i] = x;
    }
    return a;
}
static rech_u192 rech_u192_mul_small(rech_u192 a, uint64_t m) {
    unsigned __int128 c = 0;
    for (int i = 0; i < 3; i++) { c += (unsigned __int128)a.w[i] * m; a.w[i] = (uint64_t)c; c >>= 64; }
    return a;
}
static rech_u192 rech_u192_pow10_mul(rech_u192 a, int k) {
    while (k-- > 0) { a = rech_u192_mul_small(a, 10); }
    return a;
}
static rech_u192 rech_u192_div10(rech_u192 a, int *digit) {
    unsigned __int128 rem = 0;
    for (int i = 2; i >= 0; i--) {
        unsigned __int128 cur = (rem << 64) | a.w[i];
        a.w[i] = (uint64_t)(cur / 10);
        rem = cur % 10;
    }
    *digit = (int)rem;
    return a;
}
static int rech_u192_fits96(rech_u192 a) { return a.w[2] == 0 && a.w[1] <= 0xFFFFFFFFu; }
static rech_dec rech_dec_make(rech_u192 m, int scale, int neg) {
    rech_dec d;
    d.lo = m.w[0]; d.hi = (uint32_t)m.w[1]; d.scale = (uint8_t)scale;
    d.neg = (uint8_t)(rech_u192_zero(m) ? 0 : neg);
    return d;
}
static rech_u192 rech_u192_inc(rech_u192 a) { rech_u192 one = {{1, 0, 0}}; return rech_u192_add(a, one); }

/* rech_dec_norm - точная величина n / 10^scale -> Десятичное (масштаб <= 28,
   мантисса <= 96 бит), отбрасываемые цифры - однократное банковское
   округление: последняя отброшенная цифра и "были ли ненулевые ниже". */
static rech_dec rech_dec_norm(rech_u192 n, int scale, int neg) {
    int k0 = scale > 28 ? scale - 28 : 0, k = 0, last = 0, sticky = 0;
    rech_u192 t = n;
    for (;;) {
        rech_u192 r = t;
        if (k > 0 && (last > 5 || (last == 5 && (sticky || (t.w[0] & 1))))) { r = rech_u192_inc(t); }
        if (k >= k0 && rech_u192_fits96(r)) { return rech_dec_make(r, scale - k, neg); }
        if (k >= k0 && scale - k <= 0) { RECH_DEC_OVERFLOW; }
        sticky |= last != 0;
        t = rech_u192_div10(t, &last);
        k++;
    }
}

static void rech_dec_align(rech_dec a, rech_dec b, rech_u192 *am, rech_u192 *bm, int *s) {
    *s = a.scale > b.scale ? a.scale : b.scale;
    *am = rech_u192_pow10_mul(rech_u192_of(a), *s - a.scale);
    *bm = rech_u192_pow10_mul(rech_u192_of(b), *s - b.scale);
}
static rech_dec rech_dec_add(rech_dec a, rech_dec b) {
    rech_u192 am, bm; int s;
    rech_dec_align(a, b, &am, &bm, &s);
    if (a.neg == b.neg) { return rech_dec_norm(rech_u192_add(am, bm), s, a.neg); }
    if (rech_u192_cmp(am, bm) >= 0) { return rech_dec_norm(rech_u192_sub(am, bm), s, a.neg); }
    return rech_dec_norm(rech_u192_sub(bm, am), s, b.neg);
}
static rech_dec rech_dec_neg(rech_dec a) {
    if ((a.lo | a.hi) != 0) { a.neg = !a.neg; }
    return a;
}
static rech_dec rech_dec_sub(rech_dec a, rech_dec b) { return rech_dec_add(a, rech_dec_neg(b)); }
static rech_dec rech_dec_mul(rech_dec a, rech_dec b) {
    rech_u192 am = rech_u192_of(a);
    rech_u192 p = rech_u192_mul_small(am, b.lo);
    rech_u192 h = rech_u192_mul_small(am, b.hi);
    rech_u192 hs = {{0, h.w[0], h.w[1]}};
    return rech_dec_norm(rech_u192_add(p, hs), a.scale + b.scale, a.neg != b.neg);
}
static rech_dec rech_dec_div(rech_dec a, rech_dec b) {
    unsigned __int128 bm = ((unsigned __int128)b.hi << 64) | b.lo;
    unsigned __int128 am = ((unsigned __int128)a.hi << 64) | a.lo;
    unsigned __int128 max96 = ((unsigned __int128)1 << 96) - 1;
    if (bm == 0) { RECH_DEC_DIVZERO; }
    unsigned __int128 q = am / bm, r = am % bm;
    int scale = (int)a.scale - (int)b.scale;
    while (scale < 0) {
        q = q * 10 + (r * 10) / bm; r = (r * 10) % bm; scale++;
        if (q > max96) { RECH_DEC_OVERFLOW; }
    }
    while (r != 0 && scale < 28) {
        unsigned __int128 d = (r * 10) / bm;
        if (q * 10 + d > max96) { break; }
        q = q * 10 + d; r = (r * 10) % bm; scale++;
    }
    if (r != 0) {
        unsigned __int128 twice = r << 1;
        if (twice > bm || (twice == bm && (q & 1))) { q++; }
    }
    rech_u192 n = {{(uint64_t)q, (uint64_t)(q >> 64), 0}};
    return rech_dec_norm(n, scale, a.neg != b.neg);
}
static rech_u192 rech_u192_shl1(rech_u192 a) {
    a.w[2] = (a.w[2] << 1) | (a.w[1] >> 63);
    a.w[1] = (a.w[1] << 1) | (a.w[0] >> 63);
    a.w[0] <<= 1;
    return a;
}
static rech_dec rech_dec_rem(rech_dec a, rech_dec b) {
    rech_u192 am, bm, r = {{0, 0, 0}}; int s;
    if ((b.lo | b.hi) == 0) { RECH_DEC_DIVZERO; }
    rech_dec_align(a, b, &am, &bm, &s);
    for (int i = 191; i >= 0; i--) {
        r = rech_u192_shl1(r);
        r.w[0] |= (am.w[i / 64] >> (i % 64)) & 1;
        if (rech_u192_cmp(r, bm) >= 0) { r = rech_u192_sub(r, bm); }
    }
    return rech_dec_norm(r, s, a.neg);
}
static int rech_dec_cmp(rech_dec a, rech_dec b) {
    rech_u192 am, bm; int s, c;
    if (a.neg != b.neg) { return a.neg ? -1 : 1; }
    rech_dec_align(a, b, &am, &bm, &s);
    c = rech_u192_cmp(am, bm);
    return a.neg ? -c : c;
}
static rech_dec rech_dec_round(rech_dec d, long long places) {
    if (places < 0) { places = 0; }
    if (places > 28) { places = 28; }
    if (d.scale <= places) { return d; }
    rech_u192 t = rech_u192_of(d);
    int last = 0, sticky = 0;
    for (int k = d.scale - (int)places; k > 0; k--) { sticky |= last != 0; t = rech_u192_div10(t, &last); }
    if (last > 5 || (last == 5 && (sticky || (t.w[0] & 1)))) { t = rech_u192_inc(t); }
    return rech_dec_make(t, (int)places, d.neg);
}
static rech_dec rech_dec_from_int(long long v) {
    rech_u192 m = {{v < 0 ? (uint64_t)0 - (uint64_t)v : (uint64_t)v, 0, 0}};
    return rech_dec_make(m, 0, v < 0);
}
static rech_dec rech_dec_from_uint(unsigned long long v) {
    rech_u192 m = {{v, 0, 0}};
    return rech_dec_make(m, 0, 0);
}
/* rech_dec_to_text - запись с масштабом ("1.10", "-0.25"); buf >= 48. */
static const char *rech_dec_to_text(char *buf, rech_dec d) {
    char digits[40]; int n = 0, i, s = d.scale, o = 0, digit;
    rech_u192 t = rech_u192_of(d);
    do { t = rech_u192_div10(t, &digit); digits[n++] = (char)('0' + digit); } while (!rech_u192_zero(t));
    while (n <= s) { digits[n++] = '0'; }
    if (d.neg) { buf[o++] = '-'; }
    for (i = n - 1; i >= 0; i--) {
        buf[o++] = digits[i];
        if (i == s && s > 0) { buf[o++] = '.'; }
    }
    buf[o] = 0;
    return buf;
}
/* rech_dec_parse - "-123.4500", "1e-5"; 0 - не число. В 192 бита
   помещаются 57 значащих цифр: дальше целая часть - уже переполнение, а
   После 57 значащих цифр остальные отбрасываются (отброшенная цифра целой
   части - сдвиг масштаба) с признаком "остаток не ноль" (sticky).
   Мантисса результата - не больше 29 цифр, так что последняя сохранённая
   цифра стоит минимум на 28 знаков правее места округления, и ненулевой
   остаток, помеченный в ней, меняет только решение о ничьей - результат тот
   же, что у точного internal/decimal.Parse. */
static int rech_dec_parse(const char *s, rech_dec *out) {
    rech_u192 n = {{0, 0, 0}};
    int neg = 0, frac = 0, point = 0, any = 0, sig = 0, exp = 0, sticky = 0, dropped = 0, last = 0, scale;
    if (*s == '-' || *s == '+') { neg = *s == '-'; s++; }
    for (; *s && *s != 'e' && *s != 'E'; s++) {
        if (*s == '.') {
            if (point) { return 0; }
            point = 1;
            continue;
        }
        if (*s < '0' || *s > '9') { return 0; }
        any = 1;
        if (sig >= 57) {
            if (!point) { dropped++; }
            sticky |= *s != '0';
            continue;
        }
        if (point) { frac++; }
        if (sig > 0 || *s != '0') {
            sig++;
            last = *s - '0';
            n = rech_u192_add(rech_u192_mul_small(n, 10), (rech_u192){{(uint64_t)last, 0, 0}});
        }
    }
    if (!any) { return 0; }
    if (sticky && last == 0) { n = rech_u192_add(n, (rech_u192){{1, 0, 0}}); }
    if (*s) {
        int eneg = 0, ed = 0;
        s++;
        if (*s == '-' || *s == '+') { eneg = *s == '-'; s++; }
        for (; *s; s++) {
            if (*s < '0' || *s > '9' || ++ed > 4) { return 0; }
            exp = exp * 10 + (*s - '0');
        }
        if (ed == 0) { return 0; }
        if (eneg) { exp = -exp; }
    }
    scale = frac - dropped - exp;
    if (scale < 0) {
        if (!rech_u192_zero(n)) {
            if (sig - scale > 58) { RECH_DEC_OVERFLOW; }
            n = rech_u192_pow10_mul(n, -scale);
        }
        scale = 0;
    }
    *out = rech_dec_norm(n, scale, neg);
    return 1;
}
static rech_dec rech_dec_from_text(const char *s, const char *type_name) {
    rech_dec d;
    if (!rech_dec_parse(s, &d)) {
        char msg[256];
        snprintf(msg, sizeof(msg), "каст (%s): \"%s\" не является числом", type_name, s);
        RECH_DEC_PARSE_FAIL(msg);
    }
    return d;
}
/* f32 - значение из 32-битного дробного: запись по его точности (0.1, а не
   0.10000000149011612 расширенного до double). */
static rech_dec rech_dec_from_double(double v, int f32) {
    char buf[48];
    rech_dec d;
    if (v != v || v > 1.7976931348623157e308 || v < -1.7976931348623157e308) {
        char msg[128];
        snprintf(msg, sizeof(msg), "каст (Десятичное): \"%s\" не является числом", rech_fmt_float(buf, v, f32));
        RECH_DEC_BADCAST(msg);
    }
    rech_dec_parse(rech_fmt_float(buf, v, f32), &d);
    return d;
}
static double rech_dec_to_double(rech_dec d) {
    char buf[48];
    return strtod(rech_dec_to_text(buf, d), NULL);
}
static long long rech_dec_to_int(rech_dec d, const char *type_name) {
    rech_u192 t = rech_u192_of(d);
    int digit;
    for (int k = d.scale; k > 0; k--) { t = rech_u192_div10(t, &digit); }
    if (t.w[2] != 0 || t.w[1] != 0 || t.w[0] > (d.neg ? (uint64_t)1 << 63 : (uint64_t)INT64_MAX)) {
        char buf[48], msg[160];
        snprintf(msg, sizeof(msg), "каст (%s): %s не помещается в тип", type_name, rech_dec_to_text(buf, d));
        RECH_DEC_BADCAST(msg);
    }
    return d.neg ? (long long)(0 - t.w[0]) : (long long)t.w[0];
}

`
