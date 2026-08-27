// Package toolchain реализует compiler.Toolchain через системный
// C-компилятор (cc/clang/gcc), которому передаётся сгенерированный код.
package toolchain

import (
	"bytes"
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
	//
	src, err := os.ReadFile(cPath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать сгенерированный файл %q: %w", cPath, err)
	}
	args := []string{cPath, "-o", binPath, "-lm"}
	// -lgc - Boehm GC (шаг 12, "сущность"/"класс" - ссылочная семантика по
	// умолчанию, GC_MALLOC в сгенерированном C) - в отличие от -lm, НЕ
	// входит в состав стандартного C-тулчейна ни на одной платформе
	// (проверено эмпирически: ни на свежем Ubuntu, ни на MinGW-w64 её нет
	// без отдельной установки) - линковать безусловно, как -lm, означало
	// бы, что даже программа без единой сущности не соберётся без
	// libgc-dev/bdw-gc/bdwgc. cbackend подключает "#include <gc.h>" в
	// сгенерированном файле, только если в программе есть хотя бы одна
	// ссылочная сущность (ctx.usesGC, см. internal/backend/cbackend/
	// ir_generator.go) - этого же признака в самом файле достаточно, чтобы
	// решить, нужен ли -lgc, без протаскивания отдельного флага через
	// весь internal/compiler.Toolchain только ради этого.
	if bytes.Contains(src, []byte("#include <gc.h>")) {
		args = append(args, "-lgc")
	}
	// -pthread - потоки ОС (шаг 15, "запустить"/"дождаться" -
	// internal/backend/cbackend/concurrency.go) - только на POSIX, где
	// pthread_create и не является частью libc по умолчанию на всех
	// платформах (glibc сама по себе линкует его безусловно с недавних
	// версий, но не гарантированно на каждом POSIX-тулчейне) - на Windows
	// используется CreateThread (kernel32, уже линкуется по умолчанию),
	// поэтому здесь ограничено !windows, а не просто наличием
	// "#include <pthread.h>" в файле (тот присутствует в тексте всегда,
	// когда используются потоки, независимо от платформы - см. package doc
	// cbackend/ir_generator.go).
	if bytes.Contains(src, []byte("#include <pthread.h>")) && runtime.GOOS != "windows" {
		args = append(args, "-pthread")
	}
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
