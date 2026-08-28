package toolchain

import (
	"fmt"
	"os"
	"runtime"
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
func (*CC) CompileLib(cPath, libPath string, extraLibs []string) error {
	src, err := os.ReadFile(cPath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать сгенерированный файл %q: %w", cPath, err)
	}
	var args []string
	switch runtime.GOOS {
	case "windows":
		args = []string{cPath, "-o", libPath, "-shared"}
	case "darwin":
		args = []string{cPath, "-o", libPath, "-dynamiclib", "-fvisibility=hidden"}
	default:
		args = []string{cPath, "-o", libPath, "-shared", "-fPIC", "-fvisibility=hidden"}
	}
	args = append(args, commonLinkFlags(src)...)
	args = append(args, libFlags(extraLibs)...)
	return runCC(args)
}
