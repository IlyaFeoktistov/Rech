// Бинарно-безопасный HTTP-запрос/ответ - "принять_http_байты"/
// "отправить_http_ответ_байты" (internal/semantic/httpbytes.go) - см. её
// package doc для полного описания решения (отдельные сущности
// "ЗапросБайты"/"ОтветБайты", не поле в уже существующих Запрос/Ответ).
//
// Эмитится ПОСЛЕ genericsCode (typedef/методы Список<Байт>) И ПОСЛЕ
// httpServerHelpersCode (rech_recv_http_request/rech_http_parse_head,
// httpserver.go) - использует и то, и другое.
package cbackend

import "fmt"

// httpBytesRequestType/httpBytesResponseType - см. internal/semantic/
// httpbytes.go, package doc queryParamsDictType (httpserver.go) для того
// же принципа дублирования имени константой в обоих пакетах.
const (
	httpBytesRequestType  = "ЗапросБайты"
	httpBytesResponseType = "ОтветБайты"
)

// httpBytesHelpers - "принять_http_байты"/"отправить_http_ответ_байты".
// "принять_http_байты" переиспользует rech_recv_http_request/
// rech_http_parse_head целиком (httpserver.go) - разница с обычным
// "принять_http" ровно в одном месте: тело строится через rech_bytes_wrap
// с ТОЧНОЙ длиной (raw_len - смещение начала тела), а не rech_dup
// (strlen, обрезающий на первом нулевом байте). "отправить_http_ответ_
// байты" - симметрично "отправить_http_ответ" (httpserver.go), но шлёт
// resp.тело->data/->len через rech_send_raw (builtins_shared.go/tls.go),
// не rech_send (strlen).
func httpBytesHelpers() string {
	reqType := mangleType(httpBytesRequestType)
	respType := mangleType(httpBytesResponseType)
	return fmt.Sprintf(`static %[1]s rech_http_accept_request_bytes(rech_socket_t s) {
    long long raw_len = 0;
    char *raw = rech_recv_http_request(s, &raw_len);
    %[1]s req;
    req.%[3]s = rech_dup("");
    req.%[4]s = 0;
    req.%[7]s = rech_bytes_wrap(NULL, 0);

    const char *body_sep = NULL;
    rech_http_parse_head(raw, &req.%[5]s, &req.%[6]s, &req.%[8]s, &body_sep);
    if (body_sep) {
        long long body_offset = (long long)(body_sep - raw) + 4;
        long long body_len = raw_len - body_offset;
        req.%[7]s = rech_bytes_wrap((const unsigned char *)(body_sep + 4), body_len);
    }
    free(raw);
    return req;
}

static void rech_http_send_response_bytes(rech_socket_t s, %[2]s resp) {
    long long body_len = resp.%[7]s->len;
    const char *hdr_sep = resp.%[8]s[0] ? "\r\n" : "";
    int need = snprintf(NULL, 0, "HTTP/1.1 %%lld OK\r\n%%s%%sConnection: keep-alive\r\nContent-Length: %%lld\r\n\r\n",
        (long long)resp.%[9]s, resp.%[8]s, hdr_sep, body_len);
    char *head = malloc((size_t)need + 1);
    snprintf(head, (size_t)need + 1, "HTTP/1.1 %%lld OK\r\n%%s%%sConnection: keep-alive\r\nContent-Length: %%lld\r\n\r\n",
        (long long)resp.%[9]s, resp.%[8]s, hdr_sep, body_len);
    char *full = malloc((size_t)need + (size_t)body_len);
    memcpy(full, head, (size_t)need);
    if (body_len > 0) {
        memcpy(full + need, resp.%[7]s->data, (size_t)body_len);
    }
    free(head);
    rech_send_raw(s, full, (long long)need + body_len);
    free(full);
}
`, reqType, respType,
		mangleField("адрес"), mangleField("порт"), mangleField("метод"), mangleField("путь"),
		mangleField("тело"), mangleField("заголовки"), mangleField("код"))
}
