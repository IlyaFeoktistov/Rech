// Разбор multipart/form-data - см. package doc internal/semantic/
// multipart.go для полного описания решения (курируемый минимум, не
// полный RFC-парсер). Эмитится ПОСЛЕ genericsCode (typedef/метод
// "добавить" Список<ЧастьФормы>, зарегистрирован в prog.Generics
// принудительно) И ПОСЛЕ bytesWrapHelper (rech_bytes_wrap).
package multipart

import "fmt"

// FormPartType/ResultType - см. internal/semantic/multipart.go, package
// doc queryParamsDictType (httpserver.go) для того же принципа
// дублирования имени константой в обоих пакетах.
const (
	FormPartType = "ЧастьФормы"
	ResultType   = "Список<ЧастьФормы>"
)

// Helpers - "разобрать_multipart(тело, заголовки): Список<ЧастьФормы>".
// "Список<T>" над сущностью (в отличие от "Список<Байт>") хранит элементы
// ПО ЗНАЧЕНИЮ (тот же приём, что и regex.go, rech_regex_find_all -
// "Совпадение item; ...; добавить(result, item)") - не нужен отдельный
// конструктор "ЧастьФормы", просто локальная переменная-структура на
// каждую часть. Параметры - уже вычисленные вызывающей стороной
// (mangleType/mangleField/mangleMethod - внутренняя, неэкспортированная
// инфраструктура cbackend, этот пакет от неё не зависит).
func Helpers(byteType, partType, resultType, nameField, filenameField, contentTypeField, dataField, addMethod, missingBoundaryMsg string) string {
	return fmt.Sprintf(`// rech_memfind - ищет needle (needlelen байт) в haystack (haylen байт) -
// собственная реализация, не memmem (GNU-расширение, не гарантирован на
// MinGW/Windows) - NULL, если не найдено. needlelen <= 0 - совпадает с
// haystack сразу (тот же принцип, что и у memmem).
static const unsigned char *rech_memfind(const unsigned char *haystack, long long haylen, const unsigned char *needle, long long needlelen) {
    if (needlelen <= 0) {
        return haystack;
    }
    for (long long i = 0; i + needlelen <= haylen; i++) {
        if (memcmp(haystack + i, needle, (size_t)needlelen) == 0) {
            return haystack + i;
        }
    }
    return NULL;
}

// rech_multipart_extract_boundary - ищет "content-type:" в headers
// (текст заголовков всего запроса) регистронезависимо (тот же приём, что
// и rech_http_is_chunked, httpserver.go) и возвращает значение параметра
// "boundary=" из НЕГО (кавычки/пробелы вокруг значения сняты) - NULL, если
// Content-Type не найден вовсе или не несёт "boundary=".
static char *rech_multipart_extract_boundary(const char *headers) {
    static const char want[] = "content-type:";
    size_t hlen = strlen(headers);
    const char *found = NULL;
    for (size_t i = 0; i + 13 <= hlen; i++) {
        int match = 1;
        for (int j = 0; j < 13; j++) {
            char c = headers[i + j];
            if (c >= 'A' && c <= 'Z') c = (char)(c + 32);
            if (c != want[j]) { match = 0; break; }
        }
        if (match) { found = headers + i + 13; break; }
    }
    if (!found) {
        return NULL;
    }
    const char *line_end = strstr(found, "\r\n");
    size_t line_len = line_end ? (size_t)(line_end - found) : strlen(found);

    static const char bword[] = "boundary=";
    const char *bpos = NULL;
    for (size_t i = 0; i + 9 <= line_len; i++) {
        int match = 1;
        for (int j = 0; j < 9; j++) {
            char c = found[i + j];
            if (c >= 'A' && c <= 'Z') c = (char)(c + 32);
            if (c != bword[j]) { match = 0; break; }
        }
        if (match) { bpos = found + i + 9; break; }
    }
    if (!bpos) {
        return NULL;
    }
    const char *val_end = found + line_len;
    const char *semi = NULL;
    for (const char *p = bpos; p < val_end; p++) {
        if (*p == ';') { semi = p; break; }
    }
    const char *end = semi ? semi : val_end;
    while (bpos < end && (*bpos == ' ' || *bpos == '"')) bpos++;
    while (end > bpos && (end[-1] == ' ' || end[-1] == '"')) end--;
    size_t len = (size_t)(end - bpos);
    if (len == 0) {
        return NULL;
    }
    char *out = malloc(len + 1);
    memcpy(out, bpos, len);
    out[len] = '\0';
    return out;
}

// rech_multipart_disposition_param - ищет параметр param="значение" в
// блоке заголовков ОДНОЙ части формы (Content-Disposition: form-data;
// name="фото"; filename="кот.jpg") регистронезависимо по имени параметра
// - пустая rech_dup(""), если не нашёлся (обычное дело для "filename" у
// текстового поля формы).
static char *rech_multipart_disposition_param(const char *part_headers, const char *param) {
    size_t plen = strlen(part_headers);
    size_t wantlen = strlen(param) + 2;
    char *want = malloc(wantlen + 1);
    size_t wi = 0;
    for (size_t i = 0; param[i]; i++) {
        char c = param[i];
        if (c >= 'A' && c <= 'Z') c = (char)(c + 32);
        want[wi++] = c;
    }
    want[wi++] = '=';
    want[wi++] = '"';
    want[wi] = '\0';
    const char *found = NULL;
    for (size_t i = 0; i + wantlen <= plen; i++) {
        int match = 1;
        for (size_t j = 0; j < wantlen; j++) {
            char c = part_headers[i + j];
            if (c >= 'A' && c <= 'Z') c = (char)(c + 32);
            if (c != want[j]) { match = 0; break; }
        }
        if (match) { found = part_headers + i + wantlen; break; }
    }
    free(want);
    if (!found) {
        return rech_dup("");
    }
    const char *end = strchr(found, '"');
    if (!end) {
        return rech_dup("");
    }
    size_t len = (size_t)(end - found);
    char *out = malloc(len + 1);
    memcpy(out, found, len);
    out[len] = '\0';
    return out;
}

// rech_multipart_part_content_type - "Content-Type" ИМЕННО этой части
// формы (не всего запроса) - пустая rech_dup(""), если не указан (обычное
// текстовое поле формы).
static char *rech_multipart_part_content_type(const char *part_headers) {
    static const char want[] = "content-type:";
    size_t plen = strlen(part_headers);
    const char *found = NULL;
    for (size_t i = 0; i + 13 <= plen; i++) {
        int match = 1;
        for (int j = 0; j < 13; j++) {
            char c = part_headers[i + j];
            if (c >= 'A' && c <= 'Z') c = (char)(c + 32);
            if (c != want[j]) { match = 0; break; }
        }
        if (match) { found = part_headers + i + 13; break; }
    }
    if (!found) {
        return rech_dup("");
    }
    while (*found == ' ' || *found == '\t') found++;
    const char *line_end = strstr(found, "\r\n");
    size_t len = line_end ? (size_t)(line_end - found) : strlen(found);
    char *out = malloc(len + 1);
    memcpy(out, found, len);
    out[len] = '\0';
    return out;
}

static %[3]s *rech_parse_multipart(%[1]s *body, const char *headers) {
    char *boundary = rech_multipart_extract_boundary(headers);
    if (!boundary) {
        rech_panic("%[9]s");
    }
    size_t blen = strlen(boundary);
    long long delim_len = (long long)blen + 2;
    unsigned char *delim = malloc((size_t)delim_len);
    delim[0] = '-';
    delim[1] = '-';
    memcpy(delim + 2, boundary, blen);
    free(boundary);

    %[3]s *result = GC_MALLOC(sizeof(%[3]s));

    const unsigned char *data = body->data;
    long long total = body->len;
    const unsigned char *end_of_body = data + total;
    const unsigned char *found = rech_memfind(data, total, delim, delim_len);
    if (!found) {
        free(delim);
        return result;
    }
    const unsigned char *pos = found + delim_len;

    static const unsigned char hdrterm[] = {'\r', '\n', '\r', '\n'};
    for (;;) {
        if (pos + 2 <= end_of_body && pos[0] == '-' && pos[1] == '-') {
            break;
        }
        if (pos + 2 <= end_of_body && pos[0] == '\r' && pos[1] == '\n') {
            pos += 2;
        }
        const unsigned char *hdr_end = rech_memfind(pos, (long long)(end_of_body - pos), hdrterm, 4);
        if (!hdr_end) {
            break;
        }
        size_t part_hdr_len = (size_t)(hdr_end - pos);
        char *part_headers = malloc(part_hdr_len + 1);
        memcpy(part_headers, pos, part_hdr_len);
        part_headers[part_hdr_len] = '\0';

        const unsigned char *content = hdr_end + 4;
        const unsigned char *next = rech_memfind(content, (long long)(end_of_body - content), delim, delim_len);
        const unsigned char *data_end;
        const unsigned char *rest;
        if (!next) {
            data_end = end_of_body;
            rest = NULL;
        } else {
            data_end = next;
            rest = next + delim_len;
        }
        if (data_end - content >= 2 && data_end[-2] == '\r' && data_end[-1] == '\n') {
            data_end -= 2;
        }
        long long data_len = (long long)(data_end - content);

        %[2]s item;
        item.%[4]s = rech_multipart_disposition_param(part_headers, "name");
        item.%[5]s = rech_multipart_disposition_param(part_headers, "filename");
        item.%[6]s = rech_multipart_part_content_type(part_headers);
        item.%[7]s = rech_bytes_wrap(content, data_len);
        %[8]s(result, item);

        free(part_headers);
        if (!rest) {
            break;
        }
        pos = rest;
    }

    free(delim);
    return result;
}
`, byteType, partType, resultType, nameField, filenameField, contentTypeField, dataField, addMethod, missingBoundaryMsg)
}
