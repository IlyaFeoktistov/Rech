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
