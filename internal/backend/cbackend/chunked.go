// Chunked transfer encoding (RFC 7230 §4.1) - разбор ВХОДЯЩЕГО тела,
// закодированного чанками (нет заголовка Content-Length вовсе, вместо
// него "Transfer-Encoding: chunked") - для HTTP-СЕРВЕРА (принять_http, если
// клиент/прокси шлёт запрос так) и HTTP-КЛИЕНТА (запрос, если отвечающий
// сервер шлёт ответ так) одинаково - docs/ROADMAP.md, HTTP highload
// пункт 4. Кодирование СОБСТВЕННЫХ исходящих сообщений в чанки не нужно -
// и сервер (отправить_http_ответ), и клиент (запрос) уже знают полную
// длину тела заранее и всегда шлют "Content-Length", чанки им ни к чему.
//
// rech_http_is_chunked(buf, end) - ищет "transfer-encoding" среди
// заголовков (buf..end, обычно [начало_ответа, "\r\n\r\n")) регистро-
// независимо вручную, тем же приёмом, что и rech_http_content_length
// (httpserver.go) - и проверяет, есть ли "chunked" где-то в значении этого
// заголовка (до конца строки). Не различает "chunked" как ЕДИНСТВЕННОЕ
// значение от чего-то вроде гипотетического "gzip, chunked" - курируемый
// минимум, не полный RFC-парсер списка кодировок (в этом срезе сервер
// сам никогда не шлёт ничего, кроме "chunked" целиком, если вообще шлёт).
//
// rech_http_dechunk(body) - body уже НАВЕРНЯКА содержит ВЕСЬ чанкованный
// поток целиком, включая терминирующий чанк нулевого размера (HTTP-клиент,
// rech_recv_until_close уже дочитал до закрытия соединения сервером) -
// разбирает все чанки за один проход, останавливаясь на первом
// некорректном/незавершённом чанке (просто прекращает разбор, без паники -
// тот же принцип терпимости, что и у процент-декодирования query-строки:
// то, что успело разобраться, лучше, чем ничего). Трейлеры (заголовки
// после терминирующего чанка) - игнорируются, они не нужны Rech-коду.
//
// rech_http_try_dechunk(body, avail, out, out_len) - ИНКРЕМЕНТАЛЬНЫЙ
// вариант для HTTP-СЕРВЕРА: в отличие от клиента, сервер не может ждать
// закрытия соединения (клиент держит его открытым в ожидании ответа) -
// rech_recv_http_request (httpserver.go) зовёт эту функцию заново на
// КАЖДОЙ итерации накопления тела (avail - сколько байт реально принято к
// этому моменту, buf гарантированно NUL-terminated сразу после avail), она
// возвращает 1, только когда весь чанкованный поток (включая терминатор)
// уже целиком поместился в уже принятые avail байт - иначе 0 (нужно читать
// ещё), не путая "не хватает данных" с "поток некорректен" (тот же
// разбор чанков, что и rech_http_dechunk, просто с явной верхней границей
// вместо NUL-терминатора всей C-строки). Разбирает заново с начала
// КАЖДЫЙ раз - O(n²) в худшем случае (много мелких чанков), но это
// минимальный справочный HTTP-сервер, не потоковый парсер под нагрузку
// (см. package doc httpserver.go), простота важнее.
package cbackend

const chunkedHelpers = `static int rech_http_is_chunked(const char *buf, const char *end) {
    static const char want[] = "transfer-encoding";
    for (const char *p = buf; p + 17 <= end; p++) {
        int match = 1;
        for (int i = 0; i < 17; i++) {
            char c = p[i];
            if (c >= 'A' && c <= 'Z') c = (char)(c + 32);
            if (c != want[i]) { match = 0; break; }
        }
        if (!match) {
            continue;
        }
        const char *q = p + 17;
        while (q < end && (*q == ':' || *q == ' ' || *q == '\t')) q++;
        const char *line_end = q;
        while (line_end < end && !(line_end[0] == '\r' && line_end[1] == '\n')) line_end++;
        static const char chunked_word[] = "chunked";
        for (const char *r = q; r + 7 <= line_end; r++) {
            int m2 = 1;
            for (int i = 0; i < 7; i++) {
                char c = r[i];
                if (c >= 'A' && c <= 'Z') c = (char)(c + 32);
                if (c != chunked_word[i]) { m2 = 0; break; }
            }
            if (m2) {
                return 1;
            }
        }
        return 0;
    }
    return 0;
}

static char *rech_http_dechunk(const char *body) {
    size_t cap = 256, len = 0;
    char *out = malloc(cap);
    const char *p = body;
    for (;;) {
        char *after_size;
        long long size = strtoll(p, &after_size, 16);
        if (after_size == p || size < 0) {
            break;
        }
        const char *line_end = strstr(after_size, "\r\n");
        if (!line_end) {
            break;
        }
        p = line_end + 2;
        if (size == 0) {
            break;
        }
        if (len + (size_t)size + 1 > cap) {
            while (len + (size_t)size + 1 > cap) {
                cap *= 2;
            }
            out = realloc(out, cap);
        }
        memcpy(out + len, p, (size_t)size);
        len += (size_t)size;
        p += size;
        if (p[0] == '\r' && p[1] == '\n') {
            p += 2;
        }
    }
    out[len] = '\0';
    return out;
}

static int rech_http_try_dechunk(const char *body, long long avail, char **out, long long *out_len) {
    const char *end_of_avail = body + avail;
    size_t cap = 256, len = 0;
    char *buf = malloc(cap);
    const char *p = body;
    for (;;) {
        if (p >= end_of_avail) {
            free(buf);
            return 0;
        }
        char *after_size;
        long long size = strtoll(p, &after_size, 16);
        if (after_size == p || after_size > end_of_avail || size < 0) {
            free(buf);
            return 0;
        }
        const char *line_end = NULL;
        for (const char *q = after_size; q + 1 < end_of_avail; q++) {
            if (q[0] == '\r' && q[1] == '\n') {
                line_end = q;
                break;
            }
        }
        if (!line_end) {
            free(buf);
            return 0;
        }
        p = line_end + 2;
        if (size == 0) {
            // Терминирующий чанк - трейлеры (обычно нет ни одного) до
            // финального "\r\n" уже должны были прийти целиком.
            const char *trailer_end = NULL;
            for (const char *q = p; q + 1 < end_of_avail; q++) {
                if (q[0] == '\r' && q[1] == '\n') {
                    trailer_end = q;
                    break;
                }
            }
            if (!trailer_end) {
                free(buf);
                return 0;
            }
            buf[len] = '\0';
            *out = buf;
            *out_len = (long long)len;
            return 1;
        }
        if (p + size + 2 > end_of_avail) {
            free(buf);
            return 0;
        }
        if (len + (size_t)size + 1 > cap) {
            while (len + (size_t)size + 1 > cap) {
                cap *= 2;
            }
            buf = realloc(buf, cap);
        }
        memcpy(buf + len, p, (size_t)size);
        len += (size_t)size;
        p += size;
        if (p[0] == '\r' && p[1] == '\n') {
            p += 2;
        }
    }
}

`
