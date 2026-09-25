package toolchain

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
)

// LibExtension - расширение разделяемой библиотеки на текущей платформе
// (runtime.GOOS - платформа, на которой запущен сам rech, не результат
// кросс-компиляции - тот же принцип, что и у платформенных флагов
// commonLinkFlags/CompileLib).
func (*CC) LibExtension() string {
	switch runtime.GOOS {
	case "windows":
		return ".dll"
	case "darwin":
		return ".dylib"
	default:
		return ".so"
	}
}

// CompileLib компилирует cPath в разделяемую библиотеку libPath (`rech
// build --lib`). В отличие от CompileC, добавляет платформенный флаг сборки
// разделяемой библиотеки и (на Linux/macOS) "-fvisibility=hidden" - по
// умолчанию скрывает КАЖДЫЙ символ, видимым остаётся только то, что
// cbackend явно пометил "__attribute__((visibility("default")))" (функции,
// объявленные "экспортировать" - см. internal/backend/cbackend/header.go,
// exportAttr).
//
// На Windows такого скрытия нет: MinGW ld по умолчанию экспортирует ВСЕ
// глобальные символы DLL, если ни один из них не помечен
// "__declspec(dllexport)" явно (в отличие от Linux/macOS, где видимость
// "default"/"hidden" - обычное дело) - cbackend пока не эмитит
// dllexport-атрибут (видимость на Linux/macOS уже покрывает основной
// сценарий - вызов Rech-библиотеки из C/Python/Go на той же машине
// разработки), значит на Windows в .dll физически видны и функции без
// "экспортировать" тоже - не проблема корректности (заголовок .h всё равно
// объявляет только помеченные "экспортировать" - лишние символы не мешают,
// если потребитель просто не знает их имён).
//
// b.StaticLib - архив ar вместо разделяемой библиотеки (compileStaticLib).
func (*CC) CompileLib(cPath, libPath string, b compiler.CBuild) error {
	if b.StaticLib {
		return compileStaticLib(cPath, libPath, b)
	}
	src, err := os.ReadFile(cPath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать сгенерированный файл %q: %w", cPath, err)
	}
	return runCC(b, cPath, libPath, append(LibModeFlags(runtime.GOOS), defaultLinkFlags(src, b)...))
}

// ConsumerLinkFlags - встроенные флаги линковки программы cPath (те же,
// что поставил бы CompileC): их архив статической библиотеки не несёт,
// их добавляет программа-потребитель.
func (*CC) ConsumerLinkFlags(cPath string, b compiler.CBuild) ([]string, error) {
	src, err := os.ReadFile(cPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать сгенерированный файл %q: %w", cPath, err)
	}
	return defaultLinkFlags(src, b), nil
}

// compileStaticLib - "<cc> [флаги C] -c <cPath> -o <obj>", затем все
// глобальные символы, кроме b.Exports, делаются локальными
// (localizeSymbols), затем "<ar> rcs <libPath> <obj>". Флаги линковки к
// архиву не относятся.
func compileStaticLib(cPath, libPath string, b compiler.CBuild) error {
	cc, err := findCCompiler(b.CCompiler)
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(cPath, filepath.Ext(cPath))
	objPath := base + ".o"
	args := append(append([]string{}, b.CFlags...), "-c", cPath, "-o", objPath)
	if err := runTool(cc, args...); err != nil {
		return err
	}
	if objPath, err = localizeSymbols(objPath, base, b.Exports, runtime.GOOS); err != nil {
		return err
	}
	ar, err := findTool("AR", "ar")
	if err != nil {
		return err
	}
	return runTool(ar, "rcs", libPath, objPath)
}

// localizeSymbols делает локальными все глобальные символы объектного
// файла objPath, кроме exports, и возвращает путь к результату. Рантайм
// Rech в сгенерированном C - не только static-функции: без этого его
// символы в архиве совпали бы с рантаймом программы-потребителя
// ("multiple definition"). Разделяемой библиотеке это не нужно - там
// видимость решает -fvisibility=hidden (LibModeFlags).
//
// macOS - "ld -r -exported_symbols_list" (C-имена там с "_" впереди),
// остальные - "objcopy --keep-global-symbols" (binutils, MinGW тоже).
func localizeSymbols(objPath, base string, exports []string, goos string) (string, error) {
	listPath := base + ".exports"
	prefix := ""
	if goos == "darwin" {
		prefix = "_"
	}
	var list strings.Builder
	for _, name := range exports {
		list.WriteString(prefix + name + "\n")
	}
	if err := os.WriteFile(listPath, []byte(list.String()), 0o644); err != nil {
		return "", fmt.Errorf("не удалось записать %s: %w", listPath, err)
	}
	if goos == "darwin" {
		ld, err := findTool("LD", "ld")
		if err != nil {
			return "", err
		}
		out := base + ".local.o"
		return out, runTool(ld, "-r", "-exported_symbols_list", listPath, "-o", out, objPath)
	}
	objcopy, err := findTool("OBJCOPY", "objcopy")
	if err != nil {
		return "", err
	}
	return objPath, runTool(objcopy, "--keep-global-symbols="+listPath, objPath)
}

// findTool - программа сборки статической библиотеки: переменная окружения
// env, иначе name из PATH (ar/objcopy/ld лежат рядом с gcc/clang, в том
// числе в MinGW).
func findTool(env, name string) (string, error) {
	if v := os.Getenv(env); v != "" {
		name = v
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("не найден %q для статической библиотеки (задайте %s или установите binutils): %w", name, env, err)
	}
	return path, nil
}

// runTool запускает внешнюю программу сборки, вывод - в тексте ошибки.
func runTool(path string, args ...string) error {
	// #nosec G204 -- пути приходят от локального пользователя, вызывающего свой же компилятор.
	output, err := exec.Command(path, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s завершился с ошибкой: %w\n%s", path, err, string(output))
	}
	return nil
}

// LibModeFlags - флаги сборки разделяемой библиотеки на goos (сборка.тип:
// библиотека / rech build --lib).
func LibModeFlags(goos string) []string {
	switch goos {
	case "windows":
		return []string{"-shared"}
	case "darwin":
		return []string{"-dynamiclib", "-fvisibility=hidden"}
	default:
		return []string{"-shared", "-fPIC", "-fvisibility=hidden"}
	}
}
