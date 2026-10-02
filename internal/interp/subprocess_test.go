package interp

import (
	"fmt"
	"runtime"
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
    Список<Строка> аргументы = новый Список<Строка>()
    Процесс процесс = запустить_процесс(%q, аргументы)
    Целое код = дождаться_процесса(процесс)
    вывести `+"`${"+`код}\n`+"`"+`
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
    Список<Строка> аргументы = новый Список<Строка>()
    аргументы.добавить("30")
    Процесс процесс = запустить_процесс("sleep", аргументы)
    отправить_сигнал(процесс.pid, СигналОС.SIGTERM)
    Целое код = дождаться_процесса(процесс)
    вывести ` + "`${" + `код}\n` + "`" + `
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
    Список<Строка> аргументы = новый Список<Строка>()
    вывести `+"`${"+`запустить_процесс("такой_программы_точно_нет_нигде_12345", аргументы).pid}\n`+"`"+`
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
	// Без пробелов вокруг "&&" - у cmd.exe (Windows) команда до разделителя
	// "&&" вбирает пробел ПЕРЕД ним буквально в вывод "echo" (обычный
	// вывод стал бы "вывод ", с хвостовым пробелом) - "sh" (Unix) разделяет
	// команды по "&&" одинаково что с пробелами, что без.
	out := runSrc(t, `функция главная
{
    РезультатКоманды р = выполнить_команду("echo вывод&&echo ошибка 1>&2")
    вывести `+"`${"+`р.вывод}\n`+"`"+`
    вывести `+"`${"+`р.ошибка}\n`+"`"+`
    вывести `+"`${"+`р.код_возврата}\n`+"`"+`
}
`)
	// cmd.exe (Windows) завершает строки "echo" через "\r\n", sh - через
	// "\n" - само разделение stdout/stderr (то, что здесь проверяется) от
	// вида перевода строки не зависит. "1>&2" в исходной команде тоже
	// требует пробела ПЕРЕД собой (иначе слился бы с "ошибка" в одно
	// слово) - тот же квирк cmd.exe, что и у "&&" (буквально включает этот
	// пробел в вывод "echo"), но убрать пробел здесь, в отличие от "&&",
	// нельзя - только хвостовой пробел на строке проверке не важен.
	out = strings.ReplaceAll(out, "\r", "")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	want := []string{"вывод", "", "ошибка", "", "0"}
	if len(lines) != len(want) {
		t.Fatalf("ожидалось %d строк, получено %d: %q", len(want), len(lines), out)
	}
	for i, w := range want {
		if strings.TrimRight(lines[i], " ") != w {
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
    РезультатКоманды р = выполнить_команду("exit 7")
    вывести `+"`${"+`р.код_возврата}\n`+"`"+`
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
	dir := t.TempDir()
	// "pwd" - не команда cmd.exe вовсе на Windows (нет такого исполняемого
	// файла по умолчанию), там та же роль - у встроенного "cd" без
	// аргументов. "/tmp" как литерал тоже не годится - невалидный рабочий
	// каталог для CreateProcess на Windows (нет буквы диска), поэтому
	// каталог - настоящий t.TempDir(), тот же принцип, что и в
	// project_test.go (TestLoadLibraryPathsResolvesRelativeToProjectDir).
	pwdCmd := "pwd"
	if runtime.GOOS == "windows" {
		pwdCmd = "cd"
	}
	out := runSrc(t, fmt.Sprintf(`функция главная
{
    РезультатКоманды р = выполнить_команду(%q, %q)
    вывести `+"`${"+`р.вывод}\n`+"`"+`
}
`, pwdCmd, dir))
	if strings.TrimSpace(out) != dir {
		t.Fatalf("ожидался вывод %q, получено %q", dir, out)
	}
}
