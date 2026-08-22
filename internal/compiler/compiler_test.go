package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeToolchain имитирует успешную сборку C - создаёт пустой файл на месте
// бинарника, не вызывая настоящий cc/gcc/clang (тест не должен зависеть от
// того, установлен ли системный компилятор).
type fakeToolchain struct{}

func (fakeToolchain) CompileC(cPath, binPath string) error {
	return os.WriteFile(binPath, []byte{}, 0o755)
}

func newTestCompiler() *Compiler {
	return NewCompiler(NewFrontend(), NewBackend(), fakeToolchain{})
}

func writeSource(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("не удалось записать исходник: %v", err)
	}
	return path
}

const helloSource = "функция главная\n{\n    вывести \"ок\"\n}\n"

func TestBuildDeletesCByDefault(t *testing.T) {
	dir := t.TempDir()
	srcPath := writeSource(t, dir, "прог.rech", helloSource)

	if _, err := newTestCompiler().Build(srcPath, false); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	cPath := filepath.Join(dir, "прог.c")
	if _, err := os.Stat(cPath); !os.IsNotExist(err) {
		t.Fatalf("ожидалось, что %s удалён по умолчанию, stat вернул: %v", cPath, err)
	}
}

func TestBuildKeepsCWhenRequested(t *testing.T) {
	dir := t.TempDir()
	srcPath := writeSource(t, dir, "прог.rech", helloSource)

	if _, err := newTestCompiler().Build(srcPath, true); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	cPath := filepath.Join(dir, "прог.c")
	if _, err := os.Stat(cPath); err != nil {
		t.Fatalf("ожидалось, что %s сохранён с --keep-c, stat вернул: %v", cPath, err)
	}
}
