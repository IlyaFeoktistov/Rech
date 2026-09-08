package interp

import (
	"fmt"
	"strings"
	"testing"
)

// TestSpawnProcessAndWaitExitCode - "запустить_процесс"/"дождаться_процесса"
// на настоящем короткоживущем процессе ("true"/"false" - стандартные
// POSIX-утилиты с гарантированным кодом возврата) - подтверждает, что pid
// реальный (waitpid по нему успешен) и код возврата извлечён корректно, не
// только что структура полей компилируется.
func TestSpawnProcessAndWaitExitCode(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want string
	}{
		{"true", "0"},
		{"false", "1"},
	} {
		src := fmt.Sprintf(`функция главная
{
    переменная аргументы: Список<Строка> = новый Список<Строка>()
    переменная процесс: Процесс = запустить_процесс(%q, аргументы)
    переменная код: Целое = дождаться_процесса(процесс)
    вывести код
}
`, tc.cmd)
		out := runSrc(t, src)
		if strings.TrimSpace(out) != tc.want {
			t.Fatalf("%s: ожидался код %s, получено %q", tc.cmd, tc.want, out)
		}
	}
}

// TestSpawnProcessThenSignalThenWait - живая проверка всей цепочки, ради
// которой заведена сущность "Процесс": запустить долгоживущий процесс,
// получить pid, отправить ему сигнал уже существующей "отправить_сигнал",
// дождаться и убедиться, что код возврата отражает именно этот сигнал
// (128+номер, соглашение POSIX-оболочки).
func TestSpawnProcessThenSignalThenWait(t *testing.T) {
	src := `функция главная
{
    переменная аргументы: Список<Строка> = новый Список<Строка>()
    аргументы.добавить("30")
    переменная процесс: Процесс = запустить_процесс("sleep", аргументы)
    отправить_сигнал(процесс.pid, СигналОС.SIGTERM)
    переменная код: Целое = дождаться_процесса(процесс)
    вывести код
}
`
	out := runSrc(t, src)
	if strings.TrimSpace(out) != "143" {
		t.Fatalf("ожидался код 143 (128+SIGTERM), получено %q", out)
	}
}

// TestSpawnProcessMissingCommandPanics - execvp/CreateProcess не находит
// программу по имени - "запустить_процесс" обязан паниковать СИНХРОННО
// (см. package doc internal/backend/cbackend/subprocess.go про самопайп),
// не запускать сломанный процесс молча.
func TestSpawnProcessMissingCommandPanics(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `функция главная
{
    переменная аргументы: Список<Строка> = новый Список<Строка>()
    вывести запустить_процесс("такой_программы_точно_нет_нигде_12345", аргументы).pid
}
`)
	if err == nil {
		t.Fatal("ожидалась паника у запустить_процесс с несуществующей программой")
	}
	if !strings.Contains(err.Error(), "не удалось запустить процесс") {
		t.Fatalf("ожидалось сообщение про ProcessSpawnFailed, получено: %v", err)
	}
}

// TestRunCommandCapturesOutputSeparately - "выполнить_команду" разделяет
// stdout/stderr на разные поля (не объединяет их, как "2>&1") - оба потока
// одновременно достаточно большие, чтобы упасть в дедлок при наивном
// последовательном чтении сначала одного, потом другого.
func TestRunCommandCapturesOutputSeparately(t *testing.T) {
	out := runSrc(t, `функция главная
{
    переменная р: РезультатКоманды = выполнить_команду("echo вывод && echo ошибка 1>&2")
    вывести р.вывод
    вывести р.ошибка
    вывести р.код_возврата
}
`)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	want := []string{"вывод", "", "ошибка", "", "0"}
	if len(lines) != len(want) {
		t.Fatalf("ожидалось %d строк, получено %d: %q", len(want), len(lines), out)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Fatalf("строка %d: ожидалось %q, получено %q (весь вывод %q)", i, w, lines[i], out)
		}
	}
}

// TestRunCommandNonzeroExitIsNotPanic - ненулевой код возврата ЗАПУЩЕННОЙ
// команды - обычное значение поля, не паника (иначе "git status" вне
// репозитория нельзя было бы разобрать без "попытаться").
func TestRunCommandNonzeroExitIsNotPanic(t *testing.T) {
	out := runSrc(t, `функция главная
{
    переменная р: РезультатКоманды = выполнить_команду("exit 7")
    вывести р.код_возврата
}
`)
	if strings.TrimSpace(out) != "7" {
		t.Fatalf("ожидался код возврата 7, получено %q", out)
	}
}

// TestRunCommandRespectsWorkingDirectory - второй аргумент "каталог" -
// рабочий каталог ИМЕННО этой команды, не всей программы (pwd внутри
// команды обязан отразить его).
func TestRunCommandRespectsWorkingDirectory(t *testing.T) {
	out := runSrc(t, `функция главная
{
    переменная р: РезультатКоманды = выполнить_команду("pwd", "/tmp")
    вывести р.вывод
}
`)
	if strings.TrimSpace(out) != "/tmp" {
		t.Fatalf("ожидался вывод \"/tmp\", получено %q", out)
	}
}
