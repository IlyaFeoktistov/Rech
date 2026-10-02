package strutil

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// TextHelpers - "в_верхний_регистр"/"в_нижний_регистр"/"заменить"/
// "начинается_с"/"заканчивается_на". Регистр - посимвольно по таблицам,
// построенным из unicode.ToUpper/unicode.ToLower (те же, что использует
// interp, strings.Map), поэтому оба backend'а меняют регистр одинаково для
// любого алфавита.
// Таблицы строятся один раз по первому запросу (обход всех кодовых точек).
var TextHelpers = sync.OnceValue(func() string { return caseTableCode() + textFuncs })

var textFuncs = fmt.Sprintf(`static size_t rech_utf8_decode(const unsigned char *p, uint32_t *cp) {
    if (p[0] < 0x80) { *cp = p[0]; return 1; }
    if ((p[0] & 0xE0) == 0xC0 && p[1]) { *cp = ((uint32_t)(p[0] & 0x1F) << 6) | (p[1] & 0x3F); return 2; }
    if ((p[0] & 0xF0) == 0xE0 && p[1] && p[2]) { *cp = ((uint32_t)(p[0] & 0x0F) << 12) | ((uint32_t)(p[1] & 0x3F) << 6) | (p[2] & 0x3F); return 3; }
    if (p[1] && p[2] && p[3]) { *cp = ((uint32_t)(p[0] & 0x07) << 18) | ((uint32_t)(p[1] & 0x3F) << 12) | ((uint32_t)(p[2] & 0x3F) << 6) | (p[3] & 0x3F); return 4; }
    *cp = p[0];
    return 1;
}

static size_t rech_utf8_encode(uint32_t cp, char *out) {
    if (cp < 0x80) { out[0] = (char)cp; return 1; }
    if (cp < 0x800) { out[0] = (char)(0xC0 | (cp >> 6)); out[1] = (char)(0x80 | (cp & 0x3F)); return 2; }
    if (cp < 0x10000) { out[0] = (char)(0xE0 | (cp >> 12)); out[1] = (char)(0x80 | ((cp >> 6) & 0x3F)); out[2] = (char)(0x80 | (cp & 0x3F)); return 3; }
    out[0] = (char)(0xF0 | (cp >> 18)); out[1] = (char)(0x80 | ((cp >> 12) & 0x3F)); out[2] = (char)(0x80 | ((cp >> 6) & 0x3F)); out[3] = (char)(0x80 | (cp & 0x3F));
    return 4;
}

static uint32_t rech_case_lookup(const uint32_t *from, const uint32_t *to, size_t n, uint32_t cp) {
    size_t lo = 0, hi = n;
    while (lo < hi) {
        size_t mid = (lo + hi) / 2;
        if (from[mid] == cp) return to[mid];
        if (from[mid] < cp) lo = mid + 1; else hi = mid;
    }
    return cp;
}

static char *rech_string_map_case(const char *s, const uint32_t *from, const uint32_t *to, size_t n) {
    size_t len = strlen(s);
    char *out = malloc(len * 4 + 1);
    size_t o = 0;
    const unsigned char *p = (const unsigned char *)s;
    while (*p) {
        uint32_t cp;
        p += rech_utf8_decode(p, &cp);
        o += rech_utf8_encode(rech_case_lookup(from, to, n, cp), out + o);
    }
    out[o] = '\0';
    return out;
}

static char *rech_string_upper(const char *s) {
    return rech_string_map_case(s, rech_upper_from, rech_upper_to, sizeof(rech_upper_from) / sizeof(rech_upper_from[0]));
}

static char *rech_string_lower(const char *s) {
    return rech_string_map_case(s, rech_lower_from, rech_lower_to, sizeof(rech_lower_from) / sizeof(rech_lower_from[0]));
}

static char *rech_string_replace(const char *s, const char *pattern, const char *with) {
    size_t plen = strlen(pattern);
    if (plen == 0) {
        rech_panic("%s");
    }
    size_t wlen = strlen(with), count = 0;
    for (const char *p = strstr(s, pattern); p; p = strstr(p + plen, pattern)) {
        count++;
    }
    char *out = malloc(strlen(s) + count * wlen + 1);
    char *o = out;
    const char *rest = s;
    for (const char *p = strstr(rest, pattern); p; p = strstr(rest, pattern)) {
        memcpy(o, rest, (size_t)(p - rest));
        o += p - rest;
        memcpy(o, with, wlen);
        o += wlen;
        rest = p + plen;
    }
    strcpy(o, rest);
    return out;
}

static bool rech_string_starts_with(const char *s, const char *prefix) {
    return strncmp(s, prefix, strlen(prefix)) == 0;
}

static bool rech_string_ends_with(const char *s, const char *suffix) {
    size_t n = strlen(s), m = strlen(suffix);
    return m <= n && strcmp(s + n - m, suffix) == 0;
}

`, rtmsg.ReplaceEmptyPattern)

// caseTableCode - C-таблицы rech_upper_from/to и rech_lower_from/to: все
// кодовые точки, у которых unicode.ToUpper/ToLower даёт другую, по
// возрастанию (двоичный поиск в rech_case_lookup).
func caseTableCode() string {
	var up, upTo, low, lowTo []string
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if u := unicode.ToUpper(r); u != r {
			up, upTo = append(up, fmt.Sprint(r)), append(upTo, fmt.Sprint(u))
		}
		if l := unicode.ToLower(r); l != r {
			low, lowTo = append(low, fmt.Sprint(r)), append(lowTo, fmt.Sprint(l))
		}
	}
	arr := func(name string, vals []string) string {
		return fmt.Sprintf("static const uint32_t %s[] = {%s};\n", name, strings.Join(vals, ","))
	}
	return arr("rech_upper_from", up) + arr("rech_upper_to", upTo) + arr("rech_lower_from", low) + arr("rech_lower_to", lowTo) + "\n"
}
