package interp

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestSignalReceivedDetectsRealSignal - настоящая отправка SIGUSR1 самому
// себе процессу (syscall.Kill, не только структурная проверка) - флаг
// обязан взвестись, пока программа поллит его в цикле, без единой паузы
// самой по себе (без sleep-примитива в языке - см. package doc
// examples/основы/сигналы.rech).
func TestSignalReceivedDetectsRealSignal(t *testing.T) {
	src := `функция главная
{
    переменная i = 0
    переменная получил = ложь
    пока получил == ложь и i < 2000000000
    {
        i = i + 1
        получил = получен_сигнал(СигналОС.SIGUSR1)
    }
    вывести получил
}
`
	go func() {
		time.Sleep(200 * time.Millisecond)
		if err := syscall.Kill(os.Getpid(), syscall.SIGUSR1); err != nil {
			t.Error(err)
		}
	}()
	out := runSrc(t, src)
	if strings.TrimSpace(out) != "истина" {
		t.Fatalf("ожидался обнаруженный сигнал (\"истина\"), получено %q", out)
	}
}

// TestSignalReceivedSIGKILLPanics - SIGKILL/SIGSTOP нельзя перехватить ни
// при каких условиях (POSIX) - "получен_сигнал" с ними обязан паниковать,
// а не молча регистрировать обработчик, который никогда не сработает.
func TestSignalReceivedSIGKILLPanics(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `функция главная
{
    вывести получен_сигнал(СигналОС.SIGKILL)
}
`)
	if err == nil {
		t.Fatal("ожидалась паника у получен_сигнал(SIGKILL)")
	}
	if !strings.Contains(err.Error(), "сигнал нельзя перехватить") {
		t.Fatalf("ожидалось сообщение про SignalUncatchable, получено: %v", err)
	}
}
