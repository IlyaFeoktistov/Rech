//go:build !windows

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
// examples/основы/сигналы.rech). syscall.Kill/SIGUSR1 - POSIX-only, файл
// собирается только под !windows (сама реализация тоже platform-specific
// - см. signals_windows.go).
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
    вывести ` + "`${" + `получил}\n` + "`" + `
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

// TestSignalReceivedDetectsNewlyAddedSignalName - живая проверка ОДНОГО из
// расширенного набора имён СигналОС (добавлены вместе с "отправить_сигнал")
// - подтверждает, что новая запись signalOrdinalToOS реально ведёт к
// правильному syscall.Signal, не только компилируется.
func TestSignalReceivedDetectsNewlyAddedSignalName(t *testing.T) {
	src := `функция главная
{
    переменная i = 0
    переменная получил = ложь
    пока получил == ложь и i < 2000000000
    {
        i = i + 1
        получил = получен_сигнал(СигналОС.SIGXFSZ)
    }
    вывести ` + "`${" + `получил}\n` + "`" + `
}
`
	go func() {
		time.Sleep(200 * time.Millisecond)
		if err := syscall.Kill(os.Getpid(), syscall.SIGXFSZ); err != nil {
			t.Error(err)
		}
	}()
	out := runSrc(t, src)
	if strings.TrimSpace(out) != "истина" {
		t.Fatalf("ожидался обнаруженный сигнал (\"истина\"), получено %q", out)
	}
}
