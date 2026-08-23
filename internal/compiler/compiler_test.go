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

// exeAppendingToolchain имитирует mingw gcc на Windows - сам добавляет
// ".exe" к выходному файлу, даже если -o дан без расширения.
type exeAppendingToolchain struct{}

func (exeAppendingToolchain) CompileC(cPath, binPath string) error {
	return os.WriteFile(binPath+".exe", []byte{}, 0o755)
}

func newExeAppendingTestCompiler() *Compiler {
	return NewCompiler(NewFrontend(), NewBackend(), exeAppendingToolchain{})
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

// TestBuildProducesExecutableBinaryAtSourcePath - compileProgram собирает
// через временный ASCII-путь (Windows mingw gcc/ld ненадёжно обрабатывают
// кириллицу в аргументах командной строки - см. moveFile/compileProgram) и
// переносит готовый бинарник на настоящее место обычной файловой операцией
// - право на выполнение (0o755, не 0o644 у обычных файлов) должно
// сохраниться после переноса, а во временном каталоге ничего не должно
// остаться "утёкшим" в каталог с исходником.
func TestBuildProducesExecutableBinaryAtSourcePath(t *testing.T) {
	dir := t.TempDir()
	srcPath := writeSource(t, dir, "программа.rech", helloSource)

	binPath, err := newTestCompiler().Build(srcPath, false)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("бинарник не найден по %q: %v", binPath, err)
	}
	if info.Mode()&0o100 == 0 {
		t.Fatalf("бинарник %q не исполняемый: mode=%v", binPath, info.Mode())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("не удалось прочитать каталог: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "программа.rech" && e.Name() != "программа" {
			t.Fatalf("неожиданный файл %q утёк в каталог с исходником", e.Name())
		}
	}
}

// TestBuildPreservesExeSuffixFromToolchain - если системный компилятор сам
// приписал ".exe" к бинарнику (mingw gcc на Windows), готовый файл должен
// оказаться на настоящем месте с ТЕМ ЖЕ ".exe", а не без него - explorer/
// cmd.exe/сторонние скрипты на Windows умеют запускать именно .exe-файл по
// имени, не файл без расширения (см. compileProgram).
func TestBuildPreservesExeSuffixFromToolchain(t *testing.T) {
	dir := t.TempDir()
	srcPath := writeSource(t, dir, "прог.rech", helloSource)

	binPath, err := newExeAppendingTestCompiler().Build(srcPath, false)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	wantPath := filepath.Join(dir, "прог.exe")
	if binPath != wantPath {
		t.Fatalf("ожидался путь %q, получено %q", wantPath, binPath)
	}
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("бинарник не найден по %q: %v", binPath, err)
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

// TestBuildFileWithUseImportsFunction - сквозная проверка Шага 11
// ("использовать") через тот же Build, которым пользуется `rech build
// <файл>`: файл helper объявлен отдельно, entry импортирует из него ровно
// одно имя и вызывает его.
func TestBuildFileWithUseImportsFunction(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "помощник.rech", "функция подсказка(): Строка\n{\n    вернуть \"привет\"\n}\n")
	entry := writeSource(t, dir, "главный.rech", "использовать подсказка из \"помощник\"\n\nфункция главная\n{\n    вывести подсказка()\n}\n")

	binPath, err := newTestCompiler().Build(entry, true)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	cContent, err := os.ReadFile(binPath + ".c")
	if err != nil {
		t.Fatalf("ожидался сохранённый .c: %v", err)
	}
	if !strings.Contains(string(cContent), "podskazka") {
		t.Fatalf("сгенерированный C не содержит импортированную функцию:\n%s", cContent)
	}
}

// TestBuildFileWithUseRejectsUnimportedName - тот же сценарий, но главный
// файл ссылается на имя, которое он не импортировал - настоящая видимость
// (не как у режима каталога ниже), должна быть ошибкой компиляции.
func TestBuildFileWithUseRejectsUnimportedName(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "помощник.rech", "функция подсказка(): Строка\n{\n    вернуть \"привет\"\n}\nфункция другая(): Строка\n{\n    вернуть \"другая\"\n}\n")
	entry := writeSource(t, dir, "главный.rech", "использовать подсказка из \"помощник\"\n\nфункция главная\n{\n    вывести другая()\n}\n")

	if _, err := newTestCompiler().Build(entry, false); err == nil {
		t.Fatal("ожидалась ошибка видимости - \"другая\" не импортирована")
	}
}

// TestBuildDirRejectsUse - файл каталога с "использовать" не должен молча
// смешиваться с "весь каталог - одна единица компиляции" (см.
// docs/ROADMAP.md, Шаг 11).
func TestBuildDirRejectsUse(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "помощник.rech", "функция подсказка(): Строка\n{\n    вернуть \"привет\"\n}\n")
	writeSource(t, dir, "главный.rech", "использовать подсказка из \"помощник\"\n\nфункция главная\n{\n    вывести подсказка()\n}\n")

	_, err := newTestCompiler().Build(dir, false)
	if err == nil {
		t.Fatal("ожидалась ошибка - \"использовать\" не поддерживается в режиме каталога")
	}
	if !strings.Contains(err.Error(), "использовать") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
}
