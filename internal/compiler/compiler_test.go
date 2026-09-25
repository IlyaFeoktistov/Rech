package compiler

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeToolchain имитирует успешную сборку C - создаёт пустой файл на месте
// бинарника, не вызывая настоящий cc/gcc/clang (тест не должен зависеть от
// того, установлен ли системный компилятор).
type fakeToolchain struct{}

func (fakeToolchain) CompileC(cPath, binPath string, b CBuild) error {
	return os.WriteFile(binPath, []byte{}, 0o755)
}

func (fakeToolchain) CompileLib(cPath, libPath string, b CBuild) error {
	return os.WriteFile(libPath, []byte{}, 0o755)
}

func (fakeToolchain) LibExtension() string                               { return ".so" }
func (fakeToolchain) ConsumerLinkFlags(string, CBuild) ([]string, error) { return nil, nil }

func newTestCompiler() *Compiler {
	return NewCompiler(NewFrontend(), NewBackend(), fakeToolchain{})
}

// capturingToolchain записывает CBuild.Libs/LibPaths, переданные
// CompileC/CompileLib - проверяет, что Compiler действительно прокидывает
// оба списка до Toolchain, а не теряет их по дороге.
type capturingToolchain struct {
	gotLibs     *[]string
	gotLibPaths *[]string
}

func (c capturingToolchain) CompileC(cPath, binPath string, b CBuild) error {
	*c.gotLibs = b.Libs
	if c.gotLibPaths != nil {
		*c.gotLibPaths = b.LibPaths
	}
	return os.WriteFile(binPath, []byte{}, 0o755)
}

func (c capturingToolchain) CompileLib(cPath, libPath string, b CBuild) error {
	*c.gotLibs = b.Libs
	if c.gotLibPaths != nil {
		*c.gotLibPaths = b.LibPaths
	}
	return os.WriteFile(libPath, []byte{}, 0o755)
}

func (capturingToolchain) LibExtension() string                               { return ".so" }
func (capturingToolchain) ConsumerLinkFlags(string, CBuild) ([]string, error) { return nil, nil }

// exeAppendingToolchain имитирует mingw gcc на Windows - сам добавляет
// ".exe" к выходному файлу, даже если -o дан без расширения.
type exeAppendingToolchain struct{}

func (exeAppendingToolchain) CompileC(cPath, binPath string, b CBuild) error {
	return os.WriteFile(binPath+".exe", []byte{}, 0o755)
}

func (exeAppendingToolchain) CompileLib(cPath, libPath string, b CBuild) error {
	return os.WriteFile(libPath, []byte{}, 0o755)
}

func (exeAppendingToolchain) LibExtension() string                               { return ".so" }
func (exeAppendingToolchain) ConsumerLinkFlags(string, CBuild) ([]string, error) { return nil, nil }

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

	if _, err := newTestCompiler().Build(srcPath, false, nil, nil); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	cPath := filepath.Join(dir, "прог.c")
	if _, err := os.Stat(cPath); !os.IsNotExist(err) {
		t.Fatalf("ожидалось, что %s удалён по умолчанию, stat вернул: %v", cPath, err)
	}
}

// TestBuildPassesExtraLibsToToolchain - "сборка.библиотеки" файла проекта
// (internal/project.File.Библиотеки) должен дойти до Toolchain.CompileC
// как есть - см. cmd/rech/main.go, где этот список читается из .rechproj.
func TestBuildPassesExtraLibsToToolchain(t *testing.T) {
	dir := t.TempDir()
	srcPath := writeSource(t, dir, "прог.rech", helloSource)

	var gotLibs []string
	comp := NewCompiler(NewFrontend(), NewBackend(), capturingToolchain{gotLibs: &gotLibs})
	if _, err := comp.Build(srcPath, false, []string{"curl", "ssl"}, nil); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(gotLibs) != 2 || gotLibs[0] != "curl" || gotLibs[1] != "ssl" {
		t.Fatalf("Toolchain.CompileC получил extraLibs=%v, ожидалось [curl ssl]", gotLibs)
	}
}

const libSource = "экспортировать функция сумма(а: Целое, б: Целое): Целое\n{\n    вернуть а + б\n}\n"

// TestBuildLibProducesLibraryAndHeader - BuildLib собирает разделяемую
// библиотеку (fakeToolchain - см. выше, без вызова настоящего cc) и
// заголовок рядом с исходником, без требования "главная" - см. package doc
// Compiler.BuildLib.
func TestBuildLibProducesLibraryAndHeader(t *testing.T) {
	dir := t.TempDir()
	srcPath := writeSource(t, dir, "либа.rech", libSource)

	libPath, hPath, _, err := newTestCompiler().BuildLib(srcPath, "", "", false, nil, nil)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if filepath.Ext(libPath) != ".so" {
		t.Fatalf("ожидалось расширение .so у %q", libPath)
	}
	if _, err := os.Stat(libPath); err != nil {
		t.Fatalf("библиотека не найдена по %q: %v", libPath, err)
	}
	header, err := os.ReadFile(hPath)
	if err != nil {
		t.Fatalf("заголовок не найден по %q: %v", hPath, err)
	}
	if !strings.Contains(string(header), "сумма") {
		t.Fatalf("ожидался прототип сумма (настоящее имя) в заголовке, получено:\n%s", header)
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

	binPath, err := newTestCompiler().Build(srcPath, false, nil, nil)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	info, err := os.Stat(binPath)
	if err != nil {
		t.Fatalf("бинарник не найден по %q: %v", binPath, err)
	}
	// Windows не знает Unix-бита "исполняемый" вовсе - os.FileInfo.Mode() у
	// ЛЮБОГО обычного файла там всегда без бит владельца/группы/прочих на
	// запуск (CreateProcess решает, можно ли запустить файл, по содержимому
	// PE-заголовка, не по битам режима и не по расширению) - на этой ОС
	// проверять нечего, сама проверка бессмысленна.
	if runtime.GOOS != "windows" && info.Mode()&0o100 == 0 {
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

	binPath, err := newExeAppendingTestCompiler().Build(srcPath, false, nil, nil)
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

	if _, err := newTestCompiler().Build(srcPath, true, nil, nil); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	cPath := filepath.Join(dir, "прог.c")
	if _, err := os.Stat(cPath); err != nil {
		t.Fatalf("ожидалось, что %s сохранён с --keep-c, stat вернул: %v", cPath, err)
	}
}

// TestBuildDirMergesAllRechFiles - "rech build ." (как "go build .") должен
// собрать все ".rech"-файлы каталога как одну единицу компиляции без
// явного импорта - это неявное объединение, не система импортов.
func TestBuildDirMergesAllRechFiles(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "помощник.rech", "функция подсказка(): Строка\n{\n    вернуть \"привет из другого файла\"\n}\n")
	writeSource(t, dir, "главный.rech", "функция главная\n{\n    вывести подсказка()\n}\n")

	binPath, err := newTestCompiler().Build(dir, true, nil, nil)
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

// TestBuildDirMergesEnums - регрессионный тест: parseDir строит merged
// *ast.Program явным перечислением полей (см. тот же класс бага,
// зафиксированный в internal/module.TestLinkPreservesEnums) - "Enums"
// сначала не было в этом списке.
func TestBuildDirMergesEnums(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "цвета.rech", "перечисление Цвет { Красный, Зелёный }\n")
	writeSource(t, dir, "главный.rech", "функция главная\n{\n    вывести Цвет.Зелёный\n}\n")

	binPath, err := newTestCompiler().Build(dir, true, nil, nil)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	cContent, err := os.ReadFile(binPath + ".c")
	if err != nil {
		t.Fatalf("ожидался сохранённый .c: %v", err)
	}
	if !strings.Contains(string(cContent), "1LL") {
		t.Fatalf("сгенерированный C не содержит разрешённое значение перечисления (Зелёный == 1):\n%s", cContent)
	}
}

func TestBuildDirNoRechFilesFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := newTestCompiler().Build(dir, false, nil, nil); err == nil {
		t.Fatal("ожидалась ошибка - в каталоге нет ни одного .rech файла")
	}
}

// TestBuildDirAmbiguousExtensionSynonymsFails - "главный.rech" и
// "главный.речь" одновременно в одном каталоге - явная ошибка
// неоднозначности (module.CheckNoAmbiguousExtensions), а не молчаливое
// объединение обоих файлов (которое дало бы куда менее понятную ошибку
// "дубль функции главная").
func TestBuildDirAmbiguousExtensionSynonymsFails(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "главный.rech", "функция главная\n{\n    вывести 1\n}\n")
	writeSource(t, dir, "главный.речь", "функция главная\n{\n    вывести 1\n}\n")

	if _, err := newTestCompiler().Build(dir, false, nil, nil); err == nil {
		t.Fatal("ожидалась ошибка неоднозначности расширений")
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

	binPath, err := newTestCompiler().Build(entry, true, nil, nil)
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

	if _, err := newTestCompiler().Build(entry, false, nil, nil); err == nil {
		t.Fatal("ожидалась ошибка видимости - \"другая\" не импортирована")
	}
}

// TestBuildDirRejectsUse - файл каталога с "использовать" не должен молча
// смешиваться с "весь каталог - одна единица компиляции".
func TestBuildDirRejectsUse(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "помощник.rech", "функция подсказка(): Строка\n{\n    вернуть \"привет\"\n}\n")
	writeSource(t, dir, "главный.rech", "использовать подсказка из \"помощник\"\n\nфункция главная\n{\n    вывести подсказка()\n}\n")

	_, err := newTestCompiler().Build(dir, false, nil, nil)
	if err == nil {
		t.Fatal("ожидалась ошибка - \"использовать\" не поддерживается в режиме каталога")
	}
	if !strings.Contains(err.Error(), "использовать") {
		t.Fatalf("неожиданный текст ошибки: %v", err)
	}
}
