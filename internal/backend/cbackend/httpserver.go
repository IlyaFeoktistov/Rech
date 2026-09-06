package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// httpServerHelpers - "принять_http"/"отправить_http_ответ" (см.
// internal/semantic/httpserver.go, package doc) - минимальный HTTP-сервер
// поверх уже существующих сетевых примитивов (rech_accept/rech_send/
// rech_close_socket, socketHelpers), не отдельный сетевой механизм.
//
// В отличие от HTTP-клиента ("запрос", httpHelpers, rech_recv_until_close -
// читает до закрытия соединения СЕРВЕРОМ), сервер не может ждать закрытия
// соединения КЛИЕНТОМ - клиент (браузер/curl) держит соединение открытым,
// ожидая ответ. rech_recv_http_request поэтому читает по-другому: сначала
// байты заголовков (до "\r\n\r\n"), затем либо ровно Content-Length байт
// тела (0, если заголовка Content-Length нет - обычный случай для GET без
// тела), либо, если вместо него "Transfer-Encoding: chunked" - чанки до
// терминирующего (нулевого) чанка включительно (docs/ROADMAP.md, HTTP
// highload пункт 4, chunked.go) - rech_http_content_length/
// rech_http_is_chunked ищут нужный заголовок в уже полученных байтах
// регистронезависимо вручную (без <ctype.h> - однократный ручной
// case-fold дешевле нового заголовка ради одной функции).
//
// httpServerHelpersCode - была константой, стала функцией параметром
// maxRequestBytes (ir.Program.HTTPMaxRequestBytes, "http.
// макс_размер_запроса_байт" файла проекта) - тот же приём, что и
// threadPoolCode (threadpool.go): значение запекается препроцессорным
// макросом (RECH_HTTP_MAX_REQUEST_BYTES), а не читается как переменная
// рантайма, потому что для одной программы оно ровно одно и известно уже
// на этапе кодогенерации. rech_recv_http_request проверяет его дважды: как
// только известна Content-Length - сравнивается СУММА "уже прочитанные
// заголовки + заявленное тело" с пределом (не одна Content-Length -
// огромные заголовки сами по себе, без всякого тела, тоже обязаны быть
// отклонены), не читая тело вовсе, если сумма уже больше предела; и на
// каждой итерации накопления заголовков ДО "\r\n\r\n" - клиент, никогда не
// заканчивающий заголовки, иначе рос бы в памяти без предела тем же самым
// буфером, что и настоящее тело.
func httpServerHelpersCode(maxRequestBytes int) string {
	return fmt.Sprintf(`#define RECH_HTTP_MAX_REQUEST_BYTES %[11]d

static long long rech_http_content_length(const char *buf, const char *end) {
    static const char want[] = "content-length";
    for (const char *p = buf; p + 14 <= end; p++) {
        int match = 1;
        for (int i = 0; i < 14; i++) {
            char c = p[i];
            if (c >= 'A' && c <= 'Z') c = (char)(c + 32);
            if (c != want[i]) { match = 0; break; }
        }
        if (match) {
            const char *q = p + 14;
            while (q < end && (*q == ':' || *q == ' ' || *q == '\t')) q++;
            return strtoll(q, NULL, 10);
        }
    }
    return 0;
}

// rech_path_hex_digit/rech_path_url_decode - процент-декодирование СЕГМЕНТА
// пути (до "?", если есть) - настоящие HTTP-клиенты (curl, браузеры) сами
// percent-encode нелатинские байты в пути (RFC 3986), в отличие от
// query-строки "+" здесь НЕ означает пробел (это не
// application/x-www-form-urlencoded) - поэтому отдельная пара функций, не
// переиспользование rech_qp_url_decode ниже (queryParamsHelper) - тот
// специфичен для query-строки и не всегда даже эмитируется (только если
// программа вызвала "параметры_пути" хоть раз), а декодирование пути нужно
// ВСЕГДА, как только используется "принять_http".
static int rech_path_hex_digit(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

static char *rech_path_url_decode(const char *s, size_t len) {
    char *out = malloc(len + 1);
    size_t o = 0;
    for (size_t i = 0; i < len; i++) {
        if (s[i] == '%%' && i + 2 < len) {
            int hi = rech_path_hex_digit(s[i + 1]);
            int lo = rech_path_hex_digit(s[i + 2]);
            if (hi >= 0 && lo >= 0) {
                out[o++] = (char)((hi << 4) | lo);
                i += 2;
                continue;
            }
        }
        out[o++] = s[i];
    }
    out[o] = '\0';
    return out;
}

static char *rech_recv_http_request(rech_socket_t s) {
    size_t cap = 4096, len = 0;
    char *buf = malloc(cap);
    char *body_start = NULL;
    long long content_length = 0;
    int is_chunked = 0;
    char *dechunked = NULL;
    long long dechunked_len = 0;
    for (;;) {
        buf[len] = '\0';
        if (body_start == NULL) {
            char *sep = strstr(buf, "\r\n\r\n");
            if (sep) {
                body_start = sep + 4;
                is_chunked = rech_http_is_chunked(buf, sep);
                if (!is_chunked) {
                    content_length = rech_http_content_length(buf, sep);
                    // Заголовки + тело ВМЕСТЕ против предела - не только
                    // content_length: заголовки САМИ ПО СЕБЕ уже могут
                    // превышать предел (огромные cookie/токены в
                    // заголовках, Content-Length при этом мал или
                    // отсутствует вовсе) - без этого слагаемого такой
                    // запрос прошёл бы мимо проверки ниже целиком.
                    // Chunked-путь (is_chunked) сюда не входит - заранее
                    // заявленной длины тела нет вовсе (чанки сами решают,
                    // сколько их будет), общая проверка len ниже страхует
                    // ОБА пути одинаково.
                    long long header_bytes = (long long)(body_start - buf);
                    if (header_bytes + content_length > RECH_HTTP_MAX_REQUEST_BYTES) {
                        free(buf);
                        rech_panic("%[12]s");
                    }
                }
            } else if ((long long)len >= RECH_HTTP_MAX_REQUEST_BYTES) {
                free(buf);
                rech_panic("%[12]s");
            }
        }
        if (body_start != NULL) {
            if (is_chunked) {
                long long avail = (long long)(len - (size_t)(body_start - buf));
                if (rech_http_try_dechunk(body_start, avail, &dechunked, &dechunked_len)) {
                    break;
                }
            } else {
                long long have = (long long)(len - (size_t)(body_start - buf));
                if (have >= content_length) {
                    break;
                }
            }
        }
        // Общий предел на ВЕСЬ принятый буфер (заголовки + тело, чанковано
        // оно или нет) - для content_length-пути он избыточен (уже
        // проверен выше при разборе заголовков), для chunked-пути - это
        // ЕДИНСТВЕННАЯ защита от неограниченного роста (заранее известной
        // длины тела нет).
        if ((long long)len >= RECH_HTTP_MAX_REQUEST_BYTES) {
            free(buf);
            rech_panic("%[12]s");
        }
        if (len + 1 >= cap) {
            cap *= 2;
            buf = realloc(buf, cap);
        }
        long long n = rech_socket_read_raw(s, buf + len, (long long)(cap - len - 1));
        if (n < 0) {
            free(buf);
            rech_panic("%[10]s");
        }
        if (n == 0) {
            break;
        }
        len += (size_t)n;
    }
    buf[len] = '\0';
    if (is_chunked) {
        if (dechunked == NULL) {
            // Соединение оборвалось/закрылось раньше терминирующего
            // (нулевого) чанка - тем же best-effort принципом, что и
            // ранний обрыв на content_length-пути (n == 0 просто
            // прерывает цикл без паники) - берём то, что успело
            // разобраться к этому моменту, а не паникуем повторно.
            long long avail = (long long)(len - (size_t)(body_start - buf));
            if (!rech_http_try_dechunk(body_start, avail, &dechunked, &dechunked_len)) {
                dechunked = rech_dup("");
                dechunked_len = 0;
            }
        }
        // Пересобрать буфер так, чтобы rech_http_accept_request (которая
        // ищет "\r\n\r\n" и берёт всё после нЕГО как тело) увидела уже
        // ДЕКОДИРОВАННОЕ тело вместо чанкованного потока - заголовки (до
        // body_start) остаются как есть, хвост заменяется целиком.
        size_t header_part_len = (size_t)(body_start - buf);
        char *combined = malloc(header_part_len + (size_t)dechunked_len + 1);
        memcpy(combined, buf, header_part_len);
        memcpy(combined + header_part_len, dechunked, (size_t)dechunked_len);
        combined[header_part_len + dechunked_len] = '\0';
        free(buf);
        free(dechunked);
        return combined;
    }
    return buf;
}

static %[7]s rech_http_accept_request(rech_socket_t s) {
    char *raw = rech_recv_http_request(s);
    %[7]s req;
    req.%[3]s = rech_dup("");
    req.%[5]s = 0;
    req.%[1]s = rech_dup("");
    req.%[2]s = rech_dup("");
    req.%[4]s = rech_dup("");
    req.%[6]s = rech_dup("");

    char *line_end = strstr(raw, "\r\n");
    char *sp1 = strchr(raw, ' ');
    if (sp1 && line_end && sp1 < line_end) {
        size_t method_len = (size_t)(sp1 - raw);
        char *method = malloc(method_len + 1);
        memcpy(method, raw, method_len);
        method[method_len] = '\0';
        req.%[1]s = method;

        char *sp2 = strchr(sp1 + 1, ' ');
        if (sp2 && sp2 < line_end) {
            size_t path_len = (size_t)(sp2 - (sp1 + 1));
            char *raw_path = malloc(path_len + 1);
            memcpy(raw_path, sp1 + 1, path_len);
            raw_path[path_len] = '\0';
            // Декодируется только СЕГМЕНТ пути (до "?") - query-строка
            // остаётся как есть, её отдельно декодирует rech_query_params
            // (rech_qp_url_decode) по своим правилам ("+" == пробел, чего у
            // сегмента пути нет).
            char *q = strchr(raw_path, '?');
            size_t seg_len = q ? (size_t)(q - raw_path) : path_len;
            char *decoded_seg = rech_path_url_decode(raw_path, seg_len);
            if (q) {
                size_t decoded_len = strlen(decoded_seg);
                size_t suffix_len = strlen(q);
                char *combined = malloc(decoded_len + suffix_len + 1);
                memcpy(combined, decoded_seg, decoded_len);
                memcpy(combined + decoded_len, q, suffix_len + 1);
                free(decoded_seg);
                free(raw_path);
                req.%[2]s = combined;
            } else {
                free(raw_path);
                req.%[2]s = decoded_seg;
            }
        }
    }

    char *hdr_start = line_end ? line_end + 2 : raw;
    char *body_sep = strstr(raw, "\r\n\r\n");
    if (body_sep && body_sep >= hdr_start) {
        size_t header_len = (size_t)(body_sep - hdr_start);
        char *headers = malloc(header_len + 1);
        memcpy(headers, hdr_start, header_len);
        headers[header_len] = '\0';
        req.%[4]s = headers;
        req.%[6]s = rech_dup(body_sep + 4);
    }
    free(raw);
    return req;
}

// "Connection: keep-alive" (не "close") - сервер всегда ГОТОВ обслужить
// следующий запрос на этом же соединении - решает, действительно ли
// читать следующий запрос или закрыть сокет, вызывающий Rech-код (см.
// examples/проекты/http_сервер, "обработать_соединение" - проверяет
// "Connection" ВХОДЯЩЕГО запроса через "значение_заголовка"), не рантайм.
static void rech_http_send_response(rech_socket_t s, %[8]s resp) {
    size_t body_len = strlen(resp.%[6]s);
    const char *hdr_sep = resp.%[4]s[0] ? "\r\n" : "";
    int need = snprintf(NULL, 0, "HTTP/1.1 %%lld OK\r\n%%s%%sConnection: keep-alive\r\nContent-Length: %%lld\r\n\r\n",
        (long long)resp.%[9]s, resp.%[4]s, hdr_sep, (long long)body_len);
    char *head = malloc((size_t)need + 1);
    snprintf(head, (size_t)need + 1, "HTTP/1.1 %%lld OK\r\n%%s%%sConnection: keep-alive\r\nContent-Length: %%lld\r\n\r\n",
        (long long)resp.%[9]s, resp.%[4]s, hdr_sep, (long long)body_len);
    char *full = malloc((size_t)need + body_len + 1);
    memcpy(full, head, (size_t)need);
    memcpy(full + need, resp.%[6]s, body_len);
    free(head);
    rech_send(s, full);
    free(full);
}
`, mangleField("метод"), mangleField("путь"), mangleField("адрес"), mangleField("заголовки"),
		mangleField("порт"), mangleField("тело"), mangleType("Запрос"), mangleType("Ответ"),
		mangleField("код"), rtmsg.HTTPServerRecvFailed, maxRequestBytes, rtmsg.HTTPServerRequestTooLarge)
}

// queryParamsDictType - см. internal/semantic/httpserver.go,
// registerHTTPServerFuncs - имя generic-инстанциации, принудительно
// зарегистрированной для "параметры_пути" (попадает в prog.Generics тем же
// путём, что и любая generic-коллекция, реально написанная пользователем -
// buildIR не различает "пользователь написал" от "builtin зарегистрировал")
// - typedef/методы уже эмитятся общим циклом по prog.Generics раньше этого
// блока.
const queryParamsDictType = "Словарь<Строка, Строка>"

// queryParamsHelper - "параметры_пути" (см. internal/semantic/
// httpserver.go, package doc) - разбирает query-строку ("?a=1&b=2" - часть
// до "?", если есть, игнорируется) в Словарь<Строка, Строка> с процент-
// декодированием (rech_qp_url_decode - тот же алгоритм, что
// internal/interp/httpserver.go, urlDecode, должны оставаться
// согласованными).
//
// **Найденный и исправленный баг**: "rech_query_params" возвращал
// Словарь<Строка, Строка> ПО ЗНАЧЕНИЮ (`out = {0}; ...; return out;`), а
// ЛЮБОЙ вызывающий код (VarDecl/присваивание) ждёт указатель -
// generic-коллекции ссылочные ВЕЗДЕ в cbackend (entityIsRef/
// genericCtorCCode, collections_ccode.go). Это единственное исключение,
// собранное вручную, а не общим genericCtorCCode - несовпадение давало
// "incompatible types when initializing type ... * using type ..." у
// настоящего cc при ЛЮБОМ вызове "параметры_пути", воспроизводится и без
// рефлексии в программе (в отличие от того, как это было первоначально
// описано в docs/ROADMAP.md - Шаг 41 связал баг именно с рефлексией,
// вероятно, по конкретному примеру, где он был впервые замечен, а не по
// действительной причине). Исправлено - GC_MALLOC вместо стековой
// структуры, возврат указателя.
var queryParamsHelper = fmt.Sprintf(`static int rech_qp_hex_digit(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

static char *rech_qp_url_decode(const char *s, long long len) {
    char *out = malloc((size_t)len + 1);
    long long o = 0;
    for (long long i = 0; i < len; i++) {
        if (s[i] == '+') {
            out[o++] = ' ';
        } else if (s[i] == '%%' && i + 2 < len) {
            int hi = rech_qp_hex_digit(s[i + 1]);
            int lo = rech_qp_hex_digit(s[i + 2]);
            if (hi >= 0 && lo >= 0) {
                out[o++] = (char)((hi << 4) | lo);
                i += 2;
            } else {
                out[o++] = s[i];
            }
        } else {
            out[o++] = s[i];
        }
    }
    out[o] = '\0';
    return out;
}

static %[1]s *rech_query_params(const char *path) {
    // Указатель, не структура по значению - Список/Стопка/Очередь/Словарь
    // ссылочные ВЕЗДЕ в cbackend (см. entityIsRef/genericCtorCCode,
    // collections_ccode.go) - вызывающий код (VarDecl/присваивание,
    // ir_generator.go.cType) уже ожидает "%[1]s *", возврат по значению
    // здесь был бы единственным исключением и давал бы
    // "incompatible types" у настоящего cc.
    %[1]s *out = GC_MALLOC(sizeof(%[1]s));
    const char *q = strchr(path, '?');
    if (!q) {
        return out;
    }
    q++;
    const char *p = q;
    while (*p) {
        const char *pair_start = p;
        const char *eq = NULL;
        while (*p && *p != '&') {
            if (*p == '=' && !eq) {
                eq = p;
            }
            p++;
        }
        char *key = rech_qp_url_decode(pair_start, (long long)((eq ? eq : p) - pair_start));
        const char *val_start = eq ? eq + 1 : p;
        long long val_len = eq ? (long long)(p - eq - 1) : 0;
        char *value = rech_qp_url_decode(val_start, val_len);
        if (key[0]) {
            %[2]s(out, key, value);
        }
        if (*p == '&') {
            p++;
        }
    }
    return out;
}
`, mangleType(queryParamsDictType), mangleMethod(queryParamsDictType, "установить"))
