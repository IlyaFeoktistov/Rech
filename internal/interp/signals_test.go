package interp

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

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

// TestSignalSendDeliversToOwnProcess - "отправить_сигнал" отправляет
// НАСТОЯЩИЙ сигнал (kill(2), не имитацию) - себе же самому по собственному
// pid, "получен_сигнал" в отдельной горутине обязан его увидеть, тем же
// путём, что настоящая внешняя отправка в TestSignalReceivedDetectsRealSignal
// выше, только источник сигнала - сам Rech-код, а не тестовая горутина.
func TestSignalSendDeliversToOwnProcess(t *testing.T) {
	src := fmt.Sprintf(`функция главная
{
    отправить_сигнал(%d, СигналОС.SIGUSR2)
    переменная i = 0
    переменная получил = ложь
    пока получил == ложь и i < 2000000000
    {
        i = i + 1
        получил = получен_сигнал(СигналОС.SIGUSR2)
    }
    вывести получил
}
`, os.Getpid())
	out := runSrc(t, src)
	if strings.TrimSpace(out) != "истина" {
		t.Fatalf("ожидался обнаруженный сигнал (\"истина\"), получено %q", out)
	}
}

// TestSignalSendInvalidPidPanics - kill() несуществующему pid возвращает
// ESRCH - "отправить_сигнал" обязан паниковать (rtmsg.SignalSendFailed), не
// молчать - иначе программа решила бы, что сигнал доставлен.
func TestSignalSendInvalidPidPanics(t *testing.T) {
	// PID 999999999 - гарантированно не существует (Linux pid_max по
	// умолчанию значительно меньше).
	err := runSrcExpectRuntimeError(t, `функция главная
{
    отправить_сигнал(999999999, СигналОС.SIGTERM)
}
`)
	if err == nil {
		t.Fatal("ожидалась паника у отправить_сигнал несуществующему pid")
	}
	if !strings.Contains(err.Error(), "не удалось отправить сигнал") {
		t.Fatalf("ожидалось сообщение про SignalSendFailed, получено: %v", err)
	}
}
