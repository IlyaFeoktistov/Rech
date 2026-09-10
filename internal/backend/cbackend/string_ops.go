package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// stringOpsHelpers - "длина_строки"/"подстрока"/"позиция_в_строке"/"обрезать"
// (internal/builtins.All) - индексация по КОДОВЫМ ТОЧКАМ Unicode, не
// байтам (тем же принципом, что и "Символ" - см. package doc
// rech_cast_str_to_char, cast.go): строка UTF-8 - переменной ширины на
// байт, поэтому "начало"/"длина"/позиция считаются в code point'ах через
// ручной подсчёт ведущих байт (не продолжающих, `(b & 0xC0) != 0x80`), не
// через strlen (тот считает БАЙТЫ). "обрезать" - исключение: убираемые
// символы (' '/'\t'/'\n'/'\r') все однобайтовые в UTF-8 и никогда не
// встречаются продолжающим байтом многобайтовой последовательности,
// поэтому голое byte-сканирование границ безопасно, strlen здесь уместна.
var stringOpsHelpers = fmt.Sprintf(`// rech_utf8_byte_offset - байтовое смещение начала code point'а с
// индексом codepointIndex (0-based) - NULL, если строка короче.
static const char *rech_utf8_byte_offset(const char *s, long long codepointIndex) {
    long long n = 0;
    const unsigned char *p = (const unsigned char *)s;
    while (*p) {
        if ((*p & 0xC0) != 0x80) {
            if (n == codepointIndex) {
                return (const char *)p;
            }
            n++;
        }
        p++;
    }
    if (n == codepointIndex) {
        return (const char *)p;
    }
    return NULL;
}

static long long rech_string_length(const char *s) {
    long long n = 0;
    for (const unsigned char *p = (const unsigned char *)s; *p; p++) {
        if ((*p & 0xC0) != 0x80) {
            n++;
        }
    }
    return n;
}

static char *rech_substring(const char *s, long long start, long long length) {
    if (start < 0 || length < 0) {
        rech_panic("%[1]s");
    }
    const char *begin = rech_utf8_byte_offset(s, start);
    const char *end = begin ? rech_utf8_byte_offset(s, start + length) : NULL;
    if (!begin || !end) {
        rech_panic("%[1]s");
    }
    size_t len = (size_t)(end - begin);
    char *out = malloc(len + 1);
    memcpy(out, begin, len);
    out[len] = '\0';
    return out;
}

// rech_string_find - позиция (в code point'ах) первого вхождения needle в
// s, -1, если не найдено - strstr работает по байтам, найденное смещение
// переводится в code point'ы подсчётом ведущих байт ДО него (сами
// s/needle - валидный UTF-8, поэтому найденная граница всегда совпадает с
// границей code point'а, дробных half-symbol совпадений не бывает).
static long long rech_string_find(const char *s, const char *needle) {
    const char *found = strstr(s, needle);
    if (!found) {
        return -1;
    }
    long long idx = 0;
    for (const unsigned char *p = (const unsigned char *)s; p < (const unsigned char *)found; p++) {
        if ((*p & 0xC0) != 0x80) {
            idx++;
        }
    }
    return idx;
}

static bool rech_is_trim_space(char c) {
    return c == ' ' || c == '\t' || c == '\n' || c == '\r';
}

static char *rech_string_trim(const char *s) {
    const char *start = s;
    while (*start && rech_is_trim_space(*start)) {
        start++;
    }
    const char *end = start + strlen(start);
    while (end > start && rech_is_trim_space(*(end - 1))) {
        end--;
    }
    size_t len = (size_t)(end - start);
    char *out = malloc(len + 1);
    memcpy(out, start, len);
    out[len] = '\0';
    return out;
}
`, rtmsg.SubstringInvalidRange)
