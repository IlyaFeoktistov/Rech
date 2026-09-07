// Package cbackend, продолжение - "спать_мс" (internal/semantic/process.go) -
// nanosleep (POSIX), не usleep - у usleep() аргумент "мс*1000" мог бы
// превысить 1 000 000 микросекунд (>= 1 секунды), для которых POSIX прямо
// объявляет поведение НЕОПРЕДЕЛЁННЫМ (glibc на практике терпит, но это не
// гарантия) - nanosleep разбирает секунды/наносекунды раздельно, никакого
// подобного ограничения нет. "выйти" не нуждается в собственном C-хелпере
// (голый exit() из уже подключаемого <stdlib.h>, см. builtins_shared.go).
package cbackend

const processHelpers = `#ifndef _WIN32
static void rech_sleep_ms(long long ms) {
    if (ms <= 0) {
        return;
    }
    struct timespec ts;
    ts.tv_sec = ms / 1000;
    ts.tv_nsec = (ms % 1000) * 1000000L;
    nanosleep(&ts, NULL);
}
#else
static void rech_sleep_ms(long long ms) {
    if (ms <= 0) {
        return;
    }
    Sleep((DWORD)ms);
}
#endif

`
