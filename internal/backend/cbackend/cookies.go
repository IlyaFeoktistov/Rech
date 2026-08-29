// Cookies поверх уже существующего HTTP (Шаг 9) - "значение_куки"/
// "строка_куки", чистые функции над строкой заголовков (internal/semantic/
// cookies.go). Совпадение имени заголовка ("Cookie:"/"Set-Cookie:") -
// точное, с учётом регистра (реальные HTTP-клиенты/браузеры всегда
// присылают именно так) - без страховки на нестандартный регистр
// (case-insensitive сравнение потребовало бы непортируемой libc-функции
// вроде strncasecmp, которой в рантайме компилятора до сих пор нигде не
// было). Значение cookie не URL-декодируется/кодируется - берётся и
// пишется как есть, той же осознанной простотой, что и остальной
// минимальный HTTP-клиент этого компилятора (сравнимо с "chunked-кодирование
// не поддерживается" - честно задокументированное ограничение, не тихий
// пробел).
package cbackend

import "fmt"

// cookieHelpers - rech_cookie_value/rech_cookie_string одним куском,
// эмитится безусловно, только если ctx.usesCookies (см. ir_generator.go) -
// сразу после typedefs (ссылается на уже готовый rech_T_n_stroka и
// rech_T_opcii_kuki, mangleType/mangleField вычисляют настоящие C-имена,
// не только "Строка"/"ОпцииКуки" - несколько десятков строк, тот же
// принцип, что datetimeHelpers). rech_str_concat (usesConcat, взводится
// сразу при регистрации вызова "строка_куки" в builtinCallCode) - обычный
// malloc, не GC_MALLOC (ни она, ни rech_cookie_value ниже не требуют
// Boehm GC вовсе - тот же принцип, что и у остальной работы со строками в
// этом рантайме).
func cookieHelpers() string {
	nullableStr := mangleType("?Строка")
	opts := mangleType("ОпцииКуки")
	path := mangleField("путь")
	domain := mangleField("домен")
	maxAge := mangleField("максимальный_возраст")
	httpOnly := mangleField("только_http")
	secure := mangleField("безопасный")
	sameSite := mangleField("тот_же_сайт")

	return fmt.Sprintf(`// значение_куки(запрос, имя) - разбирает заголовок "Cookie: a=1; b=2" (если
// он есть) и ищет пару с именем name.
static %[1]s rech_cookie_value(const char *headers, const char *name) {
    size_t name_len = strlen(name);
    const char *line = headers;
    while (line != NULL) {
        const char *line_end = strstr(line, "\r\n");
        size_t line_len = line_end ? (size_t)(line_end - line) : strlen(line);
        if (line_len >= 7 && strncmp(line, "Cookie:", 7) == 0) {
            const char *p = line + 7;
            const char *stop = line + line_len;
            while (p < stop && *p == ' ') p++;
            while (p < stop) {
                const char *eq = memchr(p, '=', (size_t)(stop - p));
                if (!eq) break;
                const char *val_start = eq + 1;
                const char *sep = memchr(val_start, ';', (size_t)(stop - val_start));
                const char *val_end = sep ? sep : stop;
                if ((size_t)(eq - p) == name_len && strncmp(p, name, name_len) == 0) {
                    size_t val_len = (size_t)(val_end - val_start);
                    char *result = malloc(val_len + 1);
                    memcpy(result, val_start, val_len);
                    result[val_len] = '\0';
                    return %[1]s_wrap(result);
                }
                p = sep ? sep + 1 : stop;
                while (p < stop && *p == ' ') p++;
            }
        }
        if (!line_end) break;
        line = line_end + 2;
    }
    return %[1]s_empty();
}

// строка_куки(имя, значение, опции) - строит одну строку "Set-Cookie: ..."
// (без завершающего "\r\n" - тем же принципом, что и "заголовки" вообще,
// см. package doc internal/semantic/httpserver.go, docs/SPEC.md).
static char *rech_cookie_string(const char *name, const char *value, %[2]s opts) {
    char *out = rech_str_concat(rech_str_concat(rech_str_concat("Set-Cookie: ", name), "="), value);
    if (opts.%[3]s[0] != '\0') {
        out = rech_str_concat(rech_str_concat(out, "; Path="), opts.%[3]s);
    }
    if (opts.%[4]s[0] != '\0') {
        out = rech_str_concat(rech_str_concat(out, "; Domain="), opts.%[4]s);
    }
    if (opts.%[5]s.has) {
        char buf[32];
        snprintf(buf, sizeof(buf), "%%lld", opts.%[5]s.value);
        out = rech_str_concat(rech_str_concat(out, "; Max-Age="), buf);
    }
    if (opts.%[6]s) {
        out = rech_str_concat(out, "; HttpOnly");
    }
    if (opts.%[7]s) {
        out = rech_str_concat(out, "; Secure");
    }
    if (opts.%[8]s[0] != '\0') {
        out = rech_str_concat(rech_str_concat(out, "; SameSite="), opts.%[8]s);
    }
    return out;
}
`, nullableStr, opts, path, domain, maxAge, httpOnly, secure, sameSite)
}
