// Package toolchain реализует compiler.Toolchain через системный
// C-компилятор (cc/clang/gcc), которому передаётся сгенерированный код.
package toolchain

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
)

// CC вызывает системный C-компилятор для сборки нативного бинарника.
type CC struct{}

// NewCC - провайдер для Wire.
func NewCC() compiler.Toolchain { return &CC{} }

// CompileC компилирует cPath в исполняемый файл binPath.
func (*CC) CompileC(cPath, binPath string) error {
	path, err := findCCompiler()
	if err != nil {
		return err
	}
	// -lm - математика (шаг 8, internal/builtins: корень/степень/округлить/
	// потолок/пол/модуль) может быть не слинкована без этого флага в libc,
	// не предоставляющей math.h-функции сама по себе; передаётся всегда, а
	// не только когда программа реально использует эти функции - линковка
	// неиспользуемой библиотеки безвредна, а решать здесь "нужна ли она"
	// означало бы протаскивать это знание из cbackend через весь конвейер
	// компиляции только ради этого одного флага.
	args := []string{cPath, "-o", binPath, "-lm"}
	if runtime.GOOS == "windows" {
		// -lshell32 - CommandLineToArgvW (шаг 8, internal/backend/cbackend.
		// argFuncHelper/rech_init_args): argv, который сам даёт CRT на
		// Windows, - в ANSI-кодовой странице процесса, не UTF-8, поэтому
		// "аргумент"/"количество_аргументов" перечитывают его сами через
		// CommandLineToArgvW - без -lshell32 сборка не слинкуется. runtime.
		// GOOS здесь - платформа, на которой на самом деле запущен сам
		// rech (не кросс-компиляция C-вывода) - тот же компилятор cc и
		// собирает, и запускается на одной и той же ОС.
		// -lws2_32 - Winsock2 (шаг 8, internal/backend/cbackend.
		// socketHelpers): сокеты на Windows - отдельная библиотека, не
		// часть обычной libc, как BSD-сокеты на POSIX. Тот же принцип, что
		// и у -lm выше - всегда, безвредно при отсутствии реального
		// использования сокетов в программе.
		args = append(args, "-lshell32", "-lws2_32")
	}
	// #nosec G204 -- пути приходят от локального пользователя, вызывающего свой же компилятор.
	cmd := exec.Command(path, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s завершился с ошибкой: %w\n%s", path, err, string(out))
	}
	return nil
}

// findCCompiler ищет системный компилятор C: сначала переменную окружения
// CC, затем стандартные имена в PATH.
func findCCompiler() (string, error) {
	if cc := os.Getenv("CC"); cc != "" {
		if path, err := exec.LookPath(cc); err == nil {
			return path, nil
		}
	}
	for _, candidate := range []string{"cc", "clang", "gcc"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("не найден C-компилятор (проверялись $CC, cc, clang, gcc) - Rech v0.1 транспилирует в C и требует системный компилятор для финальной сборки")
}
