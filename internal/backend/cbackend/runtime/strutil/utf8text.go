package strutil

// TextCode - C-версия правила "Строка - валидный UTF-8 без NUL"
// (internal/textutf8 - Go-версия, оба backend'а дают одинаковый
// результат, TestTextCodeMatchesGo): rech_text_take/rech_text_z чинят
// байты, пришедшие извне, rech_utf8_incomplete_tail - хвост, оборванный
// концом куска при чтении кусками. Нужны <stdlib.h>/<string.h>.
const TextCode = `#include <stdint.h>

/* rech_utf8_seq - длина валидного символа в начале p (1..4), 0 - байт
   невалиден (или NUL), -1 - оборванная, но пока валидная
   последовательность (не хватило n). Границы - как у Go utf8.DecodeRune. */
static int rech_utf8_seq(const unsigned char *p, size_t n) {
    unsigned char c = p[0];
    int len;
    unsigned char lo = 0x80, hi = 0xBF;
    if (c == 0) {
        return 0;
    }
    if (c < 0x80) {
        return 1;
    }
    if (c >= 0xC2 && c <= 0xDF) {
        len = 2;
    } else if (c == 0xE0) {
        len = 3;
        lo = 0xA0;
    } else if ((c >= 0xE1 && c <= 0xEC) || c == 0xEE || c == 0xEF) {
        len = 3;
    } else if (c == 0xED) {
        len = 3;
        hi = 0x9F;
    } else if (c == 0xF0) {
        len = 4;
        lo = 0x90;
    } else if (c >= 0xF1 && c <= 0xF3) {
        len = 4;
    } else if (c == 0xF4) {
        len = 4;
        hi = 0x8F;
    } else {
        return 0;
    }
    for (int i = 1; i < len; i++) {
        if ((size_t)i >= n) {
            return -1;
        }
        unsigned char b = p[i];
        if (i == 1 ? (b < lo || b > hi) : (b < 0x80 || b > 0xBF)) {
            return 0;
        }
    }
    return len;
}

static int rech_text_valid(const unsigned char *p, size_t n) {
    for (size_t i = 0; i < n;) {
        int k = rech_utf8_seq(p + i, n - i);
        if (k <= 0) {
            return 0;
        }
        i += (size_t)k;
    }
    return 1;
}

/* rech_text_fixed - новый буфер: p[0..n) с U+FFFD вместо каждого
   невалидного байта и NUL. */
static char *rech_text_fixed(const unsigned char *p, size_t n) {
    char *out = malloc(n * 3 + 1);
    size_t o = 0;
    for (size_t i = 0; i < n;) {
        int k = rech_utf8_seq(p + i, n - i);
        if (k <= 0) {
            out[o++] = (char)0xEF;
            out[o++] = (char)0xBF;
            out[o++] = (char)0xBD;
            i++;
            continue;
        }
        memcpy(out + o, p + i, (size_t)k);
        o += (size_t)k;
        i += (size_t)k;
    }
    out[o] = '\0';
    return out;
}

/* rech_text_take - buf (malloc, ёмкость >= n + 1, владение передаётся) как
   валидный текст: тот же buf, если чинить нечего. */
static char *rech_text_take(char *buf, size_t n) {
    if (rech_text_valid((const unsigned char *)buf, n)) {
        buf[n] = '\0';
        return buf;
    }
    char *out = rech_text_fixed((const unsigned char *)buf, n);
    free(buf);
    return out;
}

/* rech_text_z - C-строка, пришедшая извне (не своя), как валидный текст:
   та же строка, если чинить нечего. */
static const char *rech_text_z(const char *s) {
    if (s == NULL) {
        return "";
    }
    size_t n = strlen(s);
    if (rech_text_valid((const unsigned char *)s, n)) {
        return s;
    }
    return rech_text_fixed((const unsigned char *)s, n);
}

/* rech_utf8_incomplete_tail - сколько последних байт (0..3) - начало
   символа, оборванное концом буфера (не заменяются, а откладываются до
   следующего чтения). */
static size_t rech_utf8_incomplete_tail(const unsigned char *p, size_t n) {
    for (size_t k = 1; k <= 3 && k <= n; k++) {
        unsigned char c = p[n - k];
        if (c < 0x80) {
            return 0;
        }
        if (c >= 0xC0) {
            return rech_utf8_seq(p + n - k, k) == -1 ? k : 0;
        }
    }
    return 0;
}

/* Отложенные байты оборванного символа при чтении файла кусками
   (прочитать(файл, n)): следующее чтение того же файла любым способом
   начинается с них, закрытие файла их отбрасывает. Таблица общая, под
   спин-блокировкой (файлы читаются из разных потоков). */
typedef struct {
    void *h;
    unsigned char b[3];
    int n;
    int pos;
} rech_pend_t;
static rech_pend_t *rech_pend_tab = NULL;
static int rech_pend_len = 0, rech_pend_cap = 0;
static volatile int rech_pend_count = 0;
static volatile int rech_pend_lock = 0;

static void rech_pend_acquire(void) {
    while (__sync_lock_test_and_set(&rech_pend_lock, 1)) {
    }
}

static void rech_pend_release(void) {
    __sync_lock_release(&rech_pend_lock);
}

static void rech_pend_put(void *h, const unsigned char *b, int n) {
    rech_pend_acquire();
    if (rech_pend_len == rech_pend_cap) {
        rech_pend_cap = rech_pend_cap ? rech_pend_cap * 2 : 8;
        rech_pend_tab = realloc(rech_pend_tab, sizeof(rech_pend_t) * (size_t)rech_pend_cap);
    }
    rech_pend_t *e = &rech_pend_tab[rech_pend_len++];
    e->h = h;
    memcpy(e->b, b, (size_t)n);
    e->n = n;
    e->pos = 0;
    rech_pend_count++;
    rech_pend_release();
}

/* rech_pend_next - следующий отложенный байт h, -1 - нет. */
static int rech_pend_next(void *h) {
    if (rech_pend_count == 0) {
        return -1;
    }
    int c = -1;
    rech_pend_acquire();
    for (int i = 0; i < rech_pend_len; i++) {
        rech_pend_t *e = &rech_pend_tab[i];
        if (e->h != h) {
            continue;
        }
        c = e->b[e->pos++];
        if (e->pos == e->n) {
            rech_pend_tab[i] = rech_pend_tab[--rech_pend_len];
            rech_pend_count--;
        }
        break;
    }
    rech_pend_release();
    return c;
}

static void rech_pend_drop(void *h) {
    while (rech_pend_next(h) != -1) {
    }
}

/* rech_file_read_line - строка f до '\n' (без "\n" и завершающих '\r')
   как валидный текст; NUL внутри строки не обрывает её (читается
   побайтово, stdio буферизует). *ok == 0 - конец файла, ничего не
   прочитано. Отложенные байты файла (rech_pend_*) - первыми. */
static char *rech_file_read_line(FILE *f, int *ok) {
    size_t cap = 128, len = 0;
    char *buf = malloc(cap);
    int c, got = 0;
    for (;;) {
        c = rech_pend_next(f);
        if (c == -1) {
            c = getc(f);
        }
        if (c == EOF) {
            break;
        }
        got = 1;
        if (c == '\n') {
            break;
        }
        if (len + 1 >= cap) {
            cap *= 2;
            buf = realloc(buf, cap);
        }
        buf[len++] = (char)c;
    }
    if (!got) {
        free(buf);
        *ok = 0;
        return NULL;
    }
    while (len > 0 && buf[len - 1] == '\r') {
        len--;
    }
    *ok = 1;
    return rech_text_take(buf, len);
}

`
