package cbackend

import (
	"strings"
	"testing"
)

// TestGenerateFromIRSubprocessNotEmittedWithoutUse - "запустить_процесс"/
// "дождаться_процесса"/"выполнить_команду" не должны тащить
// subprocessRuntimeCode/условные #include вовсе, если программа не вызвала
// ни одну из трёх - тот же принцип, что и у usesSignals (signals_test.go).
func TestGenerateFromIRSubprocessNotEmittedWithoutUse(t *testing.T) {
	out := generateFromSrc(t, `функция главная
{
    вывести "привет"
}
`)
	if strings.Contains(out, "rech_spawn_process") || strings.Contains(out, "rech_run_command") {
		t.Fatalf("subprocessRuntimeCode не должен эмитироваться без вызова: \n%s", out)
	}
}

// TestGenerateFromIRSpawnProcessEmitsSelfPipe - "запустить_процесс" эмитирует
// rech_spawn_process, включая самопайп с FD_CLOEXEC (см. package doc
// subprocess.go про синхронную передачу ошибки execvp родителю) - и
// условные POSIX-заголовки (fcntl.h/sys/wait.h), не безусловно.
func TestGenerateFromIRSpawnProcessEmitsSelfPipe(t *testing.T) {
	src := `функция главная
{
    Список<Строка> аргументы = новый Список<Строка>()
    вывести запустить_процесс("ls", аргументы).pid
}
`
	out := generateFromSrc(t, src)
	if !strings.Contains(out, "rech_spawn_process") {
		t.Fatalf("ожидался rech_spawn_process, получено:\n%s", out)
	}
	if !strings.Contains(out, "FD_CLOEXEC") {
		t.Fatalf("ожидался самопайп с FD_CLOEXEC, получено:\n%s", out)
	}
	if !strings.Contains(out, "#include <fcntl.h>") {
		t.Fatalf("ожидался условный #include <fcntl.h>, получено:\n%s", out)
	}
}

// TestGenerateFromIRWaitProcessEmitsWaitpid - "дождаться_процесса" эмитирует
// rech_wait_process с WIFEXITED/WIFSIGNALED (128+сигнал - соглашение
// POSIX-оболочки, см. package doc).
func TestGenerateFromIRWaitProcessEmitsWaitpid(t *testing.T) {
	src := `функция главная
{
    Список<Строка> аргументы = новый Список<Строка>()
    Процесс п = запустить_процесс("ls", аргументы)
    вывести дождаться_процесса(п)
}
`
	out := generateFromSrc(t, src)
	if !strings.Contains(out, "rech_wait_process") {
		t.Fatalf("ожидался rech_wait_process, получено:\n%s", out)
	}
	if !strings.Contains(out, "WIFSIGNALED(status)") {
		t.Fatalf("ожидался разбор завершения сигналом (WIFSIGNALED), получено:\n%s", out)
	}
}

// TestGenerateFromIRRunCommandEmitsDualPipePoll - "выполнить_команду"
// эмитирует rech_run_command с раздельным чтением stdout/stderr через
// poll() (не последовательным - см. package doc subprocess.go про дедлок).
func TestGenerateFromIRRunCommandEmitsDualPipePoll(t *testing.T) {
	src := `функция главная
{
    РезультатКоманды р = выполнить_команду("ls", "/tmp")
    вывести р.вывод
}
`
	out := generateFromSrc(t, src)
	if !strings.Contains(out, "rech_run_command") {
		t.Fatalf("ожидался rech_run_command, получено:\n%s", out)
	}
	if !strings.Contains(out, "poll(fds,") {
		t.Fatalf("ожидался poll() над обоими пайпами, получено:\n%s", out)
	}
	if !strings.Contains(out, "#include <string.h>") {
		t.Fatalf("ожидался #include <string.h> (memcpy), получено:\n%s", out)
	}
}
