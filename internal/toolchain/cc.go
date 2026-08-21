// Package toolchain реализует compiler.Toolchain через системный
// C-компилятор (cc/clang/gcc), которому передаётся сгенерированный код.
package toolchain

import (
	"fmt"
	"os"
	"os/exec"

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
	// #nosec G204 -- пути приходят от локального пользователя, вызывающего свой же компилятор.
	cmd := exec.Command(path, cPath, "-o", binPath)
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
