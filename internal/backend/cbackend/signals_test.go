package cbackend

import (
	"strings"
	"testing"
)

// TestGenerateFromIRSignalsEmittedOnlyWhenUsed - "получен_сигнал" должен
// подключить sigaction-рантайм (signalRuntimeCode) только если программа
// реально его вызывает - обычная программа без единого сигнала не должна
// тащить <signal.h>/rech_got_signal вовсе (тот же принцип, что и у
// остального рантайма, эмитируемого условно, не безусловно).
func TestGenerateFromIRSignalsEmittedOnlyWhenUsed(t *testing.T) {
	src := `функция главная
{
    вывести "привет"
}
`
	out := generateFromSrc(t, src)
	if strings.Contains(out, "rech_got_signal") {
		t.Fatalf("rech_got_signal не должен эмитироваться без вызова \"получен_сигнал\":\n%s", out)
	}
}

// TestGenerateFromIRSignalsEmitsSigactionAndPanicGuard - реальный вызов
// "получен_сигнал(СигналОС.SIGTERM)" эмитирует rech_got_signal (sigaction,
// не устаревший signal()) и охрану SIGKILL/SIGSTOP (rtmsg.SignalUncatchable).
func TestGenerateFromIRSignalsEmitsSigactionAndPanicGuard(t *testing.T) {
	src := `функция главная
{
    вывести получен_сигнал(СигналОС.SIGTERM)
}
`
	out := generateFromSrc(t, src)
	if !strings.Contains(out, "rech_got_signal") {
		t.Fatalf("ожидался rech_got_signal, получено:\n%s", out)
	}
	if !strings.Contains(out, "sigaction(") {
		t.Fatalf("ожидалась регистрация через sigaction, получено:\n%s", out)
	}
	if !strings.Contains(out, "получен_сигнал: сигнал нельзя перехватить") {
		t.Fatalf("ожидалась охрана SIGKILL/SIGSTOP (rtmsg.SignalUncatchable), получено:\n%s", out)
	}
}

// TestGenerateFromIRSignalSendEmitsKillAndUnistdH - "отправить_сигнал"
// эмитирует rech_send_signal (kill(2) на POSIX-ветке) и подключает
// <unistd.h> (kill()/pid_t) только под "#ifndef _WIN32" - не безусловно.
func TestGenerateFromIRSignalSendEmitsKillAndUnistdH(t *testing.T) {
	src := `функция главная
{
    отправить_сигнал(1234, СигналОС.SIGTERM)
}
`
	out := generateFromSrc(t, src)
	if !strings.Contains(out, "rech_send_signal") {
		t.Fatalf("ожидался rech_send_signal, получено:\n%s", out)
	}
	if !strings.Contains(out, "kill((pid_t)pid, sig)") {
		t.Fatalf("ожидался вызов kill(), получено:\n%s", out)
	}
	if !strings.Contains(out, "#ifndef _WIN32\n#include <unistd.h>\n#endif") {
		t.Fatalf("ожидался условный #include <unistd.h>, получено:\n%s", out)
	}
}

// TestGenerateFromIRSignalSendNotEmittedWithoutUse - симметрично первому
// тесту файла - без единого вызова "получен_сигнал"/"отправить_сигнал"
// программа не должна тащить rech_send_signal/<unistd.h> вовсе.
func TestGenerateFromIRSignalSendNotEmittedWithoutUse(t *testing.T) {
	src := `функция главная
{
    вывести "привет"
}
`
	out := generateFromSrc(t, src)
	if strings.Contains(out, "rech_send_signal") {
		t.Fatalf("rech_send_signal не должен эмитироваться без вызова \"отправить_сигнал\":\n%s", out)
	}
}
