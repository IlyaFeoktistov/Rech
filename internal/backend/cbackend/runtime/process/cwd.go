package process

import "fmt"

// CwdHelper - "текущий_каталог(): Строка" (internal/builtins) - абсолютный
// путь рабочего каталога процесса. getcwd (POSIX) заполняет уже выделенный
// буфер фиксированного размера - для типичных путей 4096 байт с запасом
// (PATH_MAX на Linux обычно 4096); GetCurrentDirectoryW (Windows) - то же
// самое через MAX_PATH, но UTF-16, поэтому результат перекодируется в
// UTF-8 тем же принципом, что и rech_utf8_to_wide в обратную сторону
// (fileHelpers, internal/backend/cbackend/builtins_shared.go) - обычный
// узкий GetCurrentDirectoryA вернул бы путь в ANSI-кодовой странице
// процесса, а не UTF-8. Оба пути копируются в буфер ровно нужного
// размера (malloc, не GC_MALLOC - тем же принципом, что и rech_getenv,
// runtime/process/env.go: чистое строковое значение без единой ссылочной
// сущности не должно тянуть за собой Boehm GC). cwdFailedMsg -
// rtmsg.CwdFailed, уже экранированное в C-строковый литерал вызывающим
// кодом (escapeCString) - этот пакет специально не зависит от cbackend,
// тем же принципом, что и у options.Helpers.
func CwdHelper(cwdFailedMsgCLiteral string) string {
	return fmt.Sprintf(`static char *rech_cwd(void) {
#ifdef _WIN32
    wchar_t wbuf[MAX_PATH];
    DWORD wlen = GetCurrentDirectoryW(MAX_PATH, wbuf);
    if (wlen == 0 || wlen >= MAX_PATH) {
        rech_error_as(RECH_ERR_FILE, "%[1]s");
    }
    int ulen = WideCharToMultiByte(CP_UTF8, 0, wbuf, -1, NULL, 0, NULL, NULL);
    char *out = malloc((size_t)ulen);
    WideCharToMultiByte(CP_UTF8, 0, wbuf, -1, out, ulen, NULL, NULL);
    return out;
#else
    char buf[4096];
    if (!getcwd(buf, sizeof(buf))) {
        rech_error_as(RECH_ERR_FILE, "%[1]s");
    }
    size_t n = strlen(buf);
    char *out = malloc(n + 1);
    memcpy(out, buf, n + 1);
    return out;
#endif
}

`, cwdFailedMsgCLiteral)
}
