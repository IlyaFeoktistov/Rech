package toolchain

import "github.com/IlyaFeoktistov/Rech/internal/compiler"

// Флаги режима сборки (compiler.CBuild.Release) - добавляются к встроенным
// флагам gcc/clang; заданные в проекте "сборка.флаги_c"/"флаги_линковки"
// заменяют встроенные целиком, вместе с этими.
//
// Отладка: -Og - оптимизации, которые не мешают отладчику (переменные и
// шаги по строкам на месте), -g - отладочная информация; вместе с #line в
// сгенерированном C (cbackend) gdb/lldb показывают исходник Rech.
// Релиз: -O2, без отладочной информации, таблица символов вырезается при
// линковке (-s) - в программе не остаётся имён функций и путей исходников.

var (
	debugCFlags   = []string{"-Og", "-g"}
	releaseCFlags = []string{"-O2"}
)

// modeCFlags - флаги C режима сборки b.
func modeCFlags(b compiler.CBuild) []string {
	if b.Release {
		return releaseCFlags
	}
	return debugCFlags
}

// releaseLinkFlags - флаги линковки релиза для целевой ОС goos. На macOS
// ld64 считает -s устаревшим и игнорирует его с предупреждением -
// отладочной информации там и так нет (без -g), флаг не нужен.
func releaseLinkFlags(goos string) []string {
	if goos == "darwin" {
		return nil
	}
	return []string{"-s"}
}
