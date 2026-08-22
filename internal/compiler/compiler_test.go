package compiler

import (
	"os"
	"path/filepath"
	"strings"
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

// TestBuildDirMergesAllRechFiles - "rech build ." (как "go build .") должен
// собрать все ".rech"-файлы каталога как одну единицу компиляции без
// явного импорта (docs/ROADMAP.md, Шаг 11 - это неявное объединение, не
// система импортов).
func TestBuildDirMergesAllRechFiles(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "помощник.rech", "функция подсказка(): Строка\n{\n    вернуть \"привет из другого файла\"\n}\n")
	writeSource(t, dir, "главный.rech", "функция главная\n{\n    вывести подсказка()\n}\n")

	binPath, err := newTestCompiler().Build(dir, true)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	absDir, _ := filepath.Abs(dir)
	wantBin := filepath.Join(dir, filepath.Base(absDir))
	if binPath != wantBin {
		t.Fatalf("ожидался бинарник %q, получено %q", wantBin, binPath)
	}
	cContent, err := os.ReadFile(binPath + ".c")
	if err != nil {
		t.Fatalf("ожидался сохранённый .c: %v", err)
	}
	if !strings.Contains(string(cContent), "rech_podskazka") {
		t.Fatalf("сгенерированный C не содержит функцию из второго файла:\n%s", cContent)
	}
}

func TestBuildDirNoRechFilesFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := newTestCompiler().Build(dir, false); err == nil {
		t.Fatal("ожидалась ошибка - в каталоге нет ни одного .rech файла")
	}
}
