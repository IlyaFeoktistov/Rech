// Общий разбор заголовков HTTP - "значение_заголовка(заголовки, имя)"
// (internal/semantic/httpheaders.go) - не привязан к Запрос/Ответ, обе
// сущности несут "заголовки" тем же форматом строки ("имя: значение\r\n...").
// Совпадение имени - ТОЧНОЕ, с учётом регистра, тем же принципом, что и
// rech_cookie_value (cookies.go, см. её package doc - case-insensitive
// потребовал бы непортируемой libc-функции вроде strncasecmp, которой в
// рантайме до сих пор нигде нет).
package httpwire

import "fmt"

// HeaderValueHelper - C-рантайм "значение_заголовка(заголовки, имя)".
func HeaderValueHelper(nullableStrC string) string {
	return fmt.Sprintf(`static %[1]s rech_header_value(const char *headers, const char *name) {
    size_t name_len = strlen(name);
    const char *line = headers;
    while (line != NULL) {
        const char *line_end = strstr(line, "\r\n");
        size_t line_len = line_end ? (size_t)(line_end - line) : strlen(line);
        if (line_len > name_len && line[name_len] == ':' && strncmp(line, name, name_len) == 0) {
            const char *p = line + name_len + 1;
            const char *stop = line + line_len;
            while (p < stop && *p == ' ') p++;
            while (stop > p && *(stop - 1) == ' ') stop--;
            size_t val_len = (size_t)(stop - p);
            char *result = malloc(val_len + 1);
            memcpy(result, p, val_len);
            result[val_len] = '\0';
            return %[1]s_wrap(result);
        }
        if (!line_end) break;
        line = line_end + 2;
    }
    return %[1]s_empty();
}
`, nullableStrC)
}
