// Общий разбор заголовков HTTP - "значение_заголовка(заголовки, имя)" и
// "значения_заголовка(заголовки, имя)" (internal/semantic/httpheaders.go) -
// не привязаны к Запрос/Ответ, обе сущности несут "заголовки" тем же
// форматом строки ("имя: значение\r\n...").
//
// Совпадение имени - БЕЗ УЧЁТА РЕГИСТРА (RFC 7230 §3.2 - имя поля
// заголовка регистронезависимо) - до 2026-09-20 было точным, с учётом
// регистра (тот же принцип, что у rech_cookie_value, cookies.go) - живая
// находка при аудите "поддерживаются ли ВСЕ заголовки": реальные клиенты
// обычно шлют канонический регистр, но не обязаны (curl/HTTP-библиотеки
// нередко шлют строчными). Ручной ASCII case-fold (не strncasecmp -
// непортируемая libc-функция, которой в рантайме компилятора нигде не
// было) - тот же приём, что уже применялся в rech_http_content_length/
// rech_http_is_chunked (httpserver.go) для поиска "Content-Length"/
// "Transfer-Encoding" - здесь просто обобщён на произвольное имя.
package httpwire

import "fmt"

// HeaderLineMatchesHelper - общая часть значение_заголовка/значения_заголовка -
// эмитируется РОВНО ОДИН раз, если использована ХОТЬ ОДНА из двух (иначе
// HeaderValueHelper/HeaderValuesHelper вместе дали бы повторное
// определение "rech_header_line_matches" - обычную ошибку компиляции C) -
// см. вызывающую сторону (ir_generator.go). Сравнение регистронезависимое,
// "line_len > name_len" (не ">="), чтобы гарантировать байт ':' сразу
// после имени.
func HeaderLineMatchesHelper() string {
	return `static int rech_header_line_matches(const char *line, size_t line_len, const char *name, size_t name_len) {
    if (line_len <= name_len || line[name_len] != ':') {
        return 0;
    }
    for (size_t i = 0; i < name_len; i++) {
        char a = line[i], b = name[i];
        if (a >= 'A' && a <= 'Z') a = (char)(a + 32);
        if (b >= 'A' && b <= 'Z') b = (char)(b + 32);
        if (a != b) {
            return 0;
        }
    }
    return 1;
}

`
}

// HeaderValueHelper - C-рантайм "значение_заголовка(заголовки, имя): ?Строка" -
// ПЕРВОЕ совпадение по порядку появления, Ничто, если такого заголовка нет.
// Требует HeaderLineMatchesHelper (эмитирована раньше, вызывающей стороной).
func HeaderValueHelper(nullableStrC string) string {
	return fmt.Sprintf(`static %[1]s rech_header_value(const char *headers, const char *name) {
    size_t name_len = strlen(name);
    const char *line = headers;
    while (line != NULL) {
        const char *line_end = strstr(line, "\r\n");
        size_t line_len = line_end ? (size_t)(line_end - line) : strlen(line);
        if (rech_header_line_matches(line, line_len, name, name_len)) {
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

// HeaderValuesHelper - C-рантайм "значения_заголовка(заголовки, имя):
// Список<Строка>" - ВСЕ совпадения по порядку появления (пустой список, а
// не Ничто/паника, если ни одного) - настоящие HTTP-заголовки МОГУТ
// повторяться (`X-Forwarded-For`, несколько `Accept` и т. п.).
// mangledType/addMethod - Список<Строка>, уже вычисленные вызывающей
// стороной (mangleType/mangleMethod - внутренняя, неэкспортированная
// инфраструктура cbackend, этот пакет от неё не зависит) - тот же приём,
// что и у stringsplit.Helper. Требует HeaderLineMatchesHelper (эмитирована
// раньше, вызывающей стороной).
func HeaderValuesHelper(mangledType, addMethod string) string {
	return fmt.Sprintf(`static %[1]s *rech_header_values(const char *headers, const char *name) {
    %[1]s *out = GC_MALLOC(sizeof(%[1]s));
    size_t name_len = strlen(name);
    const char *line = headers;
    while (line != NULL) {
        const char *line_end = strstr(line, "\r\n");
        size_t line_len = line_end ? (size_t)(line_end - line) : strlen(line);
        if (rech_header_line_matches(line, line_len, name, name_len)) {
            const char *p = line + name_len + 1;
            const char *stop = line + line_len;
            while (p < stop && *p == ' ') p++;
            while (stop > p && *(stop - 1) == ' ') stop--;
            size_t val_len = (size_t)(stop - p);
            char *result = malloc(val_len + 1);
            memcpy(result, p, val_len);
            result[val_len] = '\0';
            %[2]s(out, result);
        }
        if (!line_end) break;
        line = line_end + 2;
    }
    return out;
}
`, mangledType, addMethod)
}
