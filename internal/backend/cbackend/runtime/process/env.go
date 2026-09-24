package process

import "fmt"

// EnvHelpers - "получить_переменную_окружения(имя): ?Строка" (internal/semantic/
// env.go) - обёртка над стандартным `getenv` (<stdlib.h>, POSIX/Windows
// CRT) - NULL (переменная не установлена) даёт "?Строка"-Ничто ("_empty"),
// иначе - копия строки ("_wrap") - `getenv` возвращает указатель на
// статический буфер CRT, который следующий вызов `getenv`/`putenv` может
// перезаписать, поэтому результат копируется сразу. Обычный `malloc`, не
// `GC_MALLOC` - тем же принципом, что и у rech_cookie_value (cookies.go):
// программа, читающая только переменную окружения, без единой ссылочной
// сущности, не должна тянуть за собой Boehm GC (<gc.h>).
func EnvHelpers(nullableStrC string) string {
	return fmt.Sprintf(`static %[1]s rech_getenv(const char *name) {
    const char *v = getenv(name);
    if (!v) {
        return %[1]s_empty();
    }
    size_t n = strlen(v);
    char *out = malloc(n + 1);
    memcpy(out, v, n);
    return %[1]s_wrap(rech_text_take(out, n));
}

`, nullableStrC)
}
