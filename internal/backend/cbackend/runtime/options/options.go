// Package options - "опции_проекта()"/"установить_опции_проекта(текст)"
// (internal/builtins) - секция "опции:" файла проекта (.rechproj,
// internal/project.File.Опции), СЫРОЙ YAML-текст, который программа сама
// разбирает через уже существующее "разобрать YAML ... как <Тип>" (docs/
// SPEC.md, раздел 18) - этот пакет не знает и не должен знать форму
// содержимого, только хранит и отдаёт текст целиком.
//
// "установить_опции_проекта" переопределяет ТОЛЬКО в памяти ТЕКУЩЕГО
// запуска (PHP ini_set-стиль) - никогда не пишет обратно в .rechproj на
// диске; после перезапуска процесса снова используется встроенное при
// сборке значение по умолчанию. Мьютекс - та же лениво инициализируемая
// на Windows CRITICAL_SECTION/pthread_mutex_t с обычным статическим
// инициализатором на POSIX, что и у rech_active_tasks_mutex (runtime/
// concurrency/active_tasks.go) - "опции_проекта" может быть прочитана
// одновременно с "установить_опции_проекта" из другого потока
// (типичный сценарий - HTTP-обработчик читает конфиг, админ-эндпоинт его
// меняет). Своя пара LOCK/UNLOCK-макросов, не общая с
// rech_active_tasks_mutex - разные, независимые критические секции не
// должны делить один мьютекс без причины (не тот же ресурс).
package options

import "fmt"

// Helpers - весь рантайм одним куском, эмитится безусловно один раз, только
// если ctx.usesOptions (программа реально вызывает "опции_проекта"/
// "установить_опции_проекта" хоть где-то). defaultTextCLiteral - "опции:"
// ИЗ .rechproj, уже экранированная в C-строковый литерал ВЫЗЫВАЮЩИМ кодом
// (escapeCString, internal/backend/cbackend/format.go - этот пакет
// специально не зависит от cbackend, см. package doc syncprim/mutex.go про
// тот же принцип у остальных runtime/* пакетов).
func Helpers(defaultTextCLiteral string) string {
	return fmt.Sprintf(`#ifdef _WIN32
static CRITICAL_SECTION rech_options_mutex;
static volatile LONG rech_options_mutex_ready = 0;
static void rech_options_mutex_init(void) {
    if (InterlockedCompareExchange(&rech_options_mutex_ready, 1, 0) == 0) {
        InitializeCriticalSection(&rech_options_mutex);
    }
}
#define RECH_OPTIONS_LOCK() (rech_options_mutex_init(), EnterCriticalSection(&rech_options_mutex))
#define RECH_OPTIONS_UNLOCK() LeaveCriticalSection(&rech_options_mutex)
#else
static pthread_mutex_t rech_options_mutex = PTHREAD_MUTEX_INITIALIZER;
#define RECH_OPTIONS_LOCK() pthread_mutex_lock(&rech_options_mutex)
#define RECH_OPTIONS_UNLOCK() pthread_mutex_unlock(&rech_options_mutex)
#endif

static const char *rech_options_text = "%s";

static const char *rech_options(void) {
    RECH_OPTIONS_LOCK();
    const char *v = rech_options_text;
    RECH_OPTIONS_UNLOCK();
    return v;
}

static void rech_set_options(const char *text) {
    RECH_OPTIONS_LOCK();
    rech_options_text = text;
    RECH_OPTIONS_UNLOCK();
}

`, defaultTextCLiteral)
}
