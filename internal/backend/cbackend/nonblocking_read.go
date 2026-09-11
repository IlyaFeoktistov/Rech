package cbackend

// nonBlockingReadRuntimeCode - "можно_прочитать_байт(файл: Файл, миллисекунды:
// Целое): Логическое" (docs/SPEC.md) - истина, если из "файл" можно
// прочитать хотя бы один байт без блокировки в течение "миллисекунды" мс
// (poll на POSIX, WaitForSingleObject на Windows) - ложь, если истёк
// таймаут, не пришло ничего. Мотивирующий сценарий - TUI: отличить "просто
// Escape" (одинокий байт 0x1B) от начала многобайтовой ANSI-последовательности
// (стрелки/F-клавиши) без "прочитать", блокирующегося в ожидании следующего
// байта, который может никогда не прийти. Отрицательный "миллисекунды" -
// ждать бесконечно (тот же принцип, что и у "захватить(таймаут_мс: Целое =
// -1)").
//
// Работает с ЛЮБЫМ Файл (не только терминалом) - обычный файл всегда "готов
// к чтению" немедленно (POSIX не блокирует чтение обычных файлов), это
// естественное (не специально обрабатываемое) поведение poll()/
// WaitForSingleObject на таких дескрипторах.
func nonBlockingReadRuntimeCode() string {
	return `static bool rech_can_read(rech_file_t f, long long timeout_ms) {
#ifndef _WIN32
    struct pollfd pfd;
    pfd.fd = fileno(f);
    pfd.events = POLLIN;
    pfd.revents = 0;
    int rv = poll(&pfd, 1, timeout_ms < 0 ? -1 : (int)timeout_ms);
    return rv > 0 && (pfd.revents & POLLIN) != 0;
#else
    HANDLE h = (HANDLE)_get_osfhandle(_fileno(f));
    DWORD rv = WaitForSingleObject(h, timeout_ms < 0 ? INFINITE : (DWORD)timeout_ms);
    return rv == WAIT_OBJECT_0;
#endif
}

`
}
