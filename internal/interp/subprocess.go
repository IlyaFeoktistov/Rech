package interp

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// spawnProcess - "запустить_процесс(команда, аргументы): Процесс" -
// неблокирующий spawn БЕЗ системной оболочки: exec.Command резолвит
// "команда" через PATH и запускает её с готовым списком аргументов
// напрямую (execve/CreateProcess), без единого "sh -c" - тем же принципом,
// что execvp в cbackend (см. package doc internal/backend/cbackend/
// subprocess.go). Дочерний процесс наследует стандартные потоки ЭТОЙ
// программы (виден в том же терминале) - "выполнить_команду" ниже, для
// сравнения, ПЕРЕХВАТЫВАЕТ их.
//
// cmd.Start() возвращает ошибку сразу, если программа не найдена или не
// удалось создать процесс - до "дождаться_процесса" ждать не нужно (в
// отличие от cbackend, где то же самое требует отдельного трюка с
// самопайпом - Start() уже делает это внутри стандартной библиотеки Go).
func spawnProcess(command string, args []string) Value {
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.ProcessSpawnFailed)
	}
	return processValue(int64(cmd.Process.Pid))
}

// processValue строит значение сущности "Процесс" (только поле "pid") -
// общий хвост "запустить_процесс" и "подключиться_к_процессу" (internal/
// interp/procmem_linux.go|procmem_windows.go|procmem_other.go) - обеим
// нужен один и тот же непрозрачный дескриптор, разница только в том, как
// pid был получен (спавн против уже существующего процесса).
func processValue(pid int64) Value {
	fields := map[string]Value{"pid": intValue(pid)}
	return Value{Type: ir.EntityType("Процесс"), Obj: &entityObj{typeName: "Процесс", fields: fields}}
}

// waitProcess - "дождаться_процесса(процесс): Целое" - блокируется до
// завершения РАНЕЕ запущенного "запустить_процесс" (по его pid, не по
// сохранённому дескриптору - os.FindProcess(pid).Wait() на POSIX работает
// для любого pid, реально являющегося ПРЯМЫМ потомком текущего процесса,
// каким spawnProcess его и создал; на Windows FindProcess открывает
// хендл заново по pid). Обязателен, чтобы завершившийся потомок не
// оставался зомби вплоть до выхода самой программы (POSIX) - см. package
// doc registerSubprocessFuncs.
//
// exitCode - код возврата, если процесс завершился обычным путём
// (WIFEXITED/WEXITSTATUS), либо 128+номер сигнала, если он был убит
// необработанным сигналом (WIFSIGNALED/WTERMSIG) - тем же соглашением, что
// и обычная POSIX-оболочка ($? после "kill"), и тем же кодом, что и
// rech_wait_process в cbackend (subprocess.go, package doc там).
// internal/interp/signals.go уже полагается на syscall.SIGXXX без единой
// windows-оговорки - тот же класс ограничения, эта функция ничего не
// меняет по кросс-компиляции под Windows.
func waitProcess(process Value) Value {
	pid := int(process.Obj.fields["pid"].I)
	proc, err := os.FindProcess(pid)
	if err != nil {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.SubprocessWaitFailed)
	}
	state, err := proc.Wait()
	if err != nil {
		throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.SubprocessWaitFailed)
	}
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return intValue(int64(128 + int(ws.Signal())))
	}
	return intValue(int64(state.ExitCode()))
}

// shellNameAndFlag - имя оболочки и её флаг "выполнить одну команду и
// выйти" для текущей платформы - "sh -c" везде, кроме Windows ("cmd /C"),
// тем же выбором, что и cbackend (subprocessRuntimeCode).
func shellNameAndFlag() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// runCommand - "выполнить_команду(команда, каталог): РезультатКоманды" -
// блокирующий запуск ЧЕРЕЗ системную оболочку (в отличие от
// spawnProcess) - понимает пайпы/"&&"/подстановку, как в терминале.
// "каталог" - рабочий каталог ИМЕННО этой команды, "" - унаследовать
// текущий рабочий каталог программы (exec.Cmd.Dir с пустой строкой уже
// означает ровно это, отдельная ветка не нужна). Вывод и ошибка
// перехватываются РАЗДЕЛЬНО (Stdout/Stderr - разные bytes.Buffer, не один
// общий) - см. commandResultFields.
//
// Паника - только если саму команду не удалось запустить (например, самой
// оболочки нет в PATH - редкость на любой настоящей системе); ненулевой
// код возврата ЗАПУЩЕННОЙ команды - обычный результат, не паника (иначе
// "git status" вне репозитория нельзя было бы разобрать без "попытаться").
func runCommand(command, dir string) Value {
	shell, flag := shellNameAndFlag()
	cmd := exec.Command(shell, flag, command)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			throwRuntimeError(rtmsg.ErrorProcess, "%s", rtmsg.CommandRunFailed)
		}
		exitCode = exitErr.ExitCode()
	}
	fields := map[string]Value{
		"вывод":        stringValue(decodeShellOutput(stdout.Bytes())),
		"ошибка":       stringValue(decodeShellOutput(stderr.Bytes())),
		"код_возврата": intValue(int64(exitCode)),
	}
	return Value{Type: ir.EntityType("РезультатКоманды"), Obj: &entityObj{typeName: "РезультатКоманды", fields: fields}}
}
