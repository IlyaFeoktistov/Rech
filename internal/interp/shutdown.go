package interp

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
)

// shutdownFlag - "запрошена_остановка(): Логическое". Взводится ОДИН раз,
// первым же SIGINT/SIGTERM - обработчик регистрируется лениво (см.
// ensureShutdownSignalHandler), только когда программа реально
// использует "запрошена_остановка" хотя бы раз (та же экономия, что и у
// остального рантайма, эмитируемого условно в cbackend, здесь - просто
// незапущенная горутина). Никакого отдельного колбэка/хука на остановку
// нет - программа сама решает, что делать, когда флаг взведён (обычно -
// выйти из цикла "принять" и доделать cleanup обычным кодом после него,
// см. package doc internal/builtins, "запрошена_остановка").
var (
	shutdownFlag     atomic.Bool
	shutdownHandlers sync.Once
)

func ensureShutdownSignalHandler() {
	shutdownHandlers.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			shutdownFlag.Store(true)
		}()
	})
}

func shutdownRequested() bool {
	ensureShutdownSignalHandler()
	return shutdownFlag.Load()
}
