package cbackend

import "strings"

// libraryInitCode - инициализация библиотеки (разделяемой, статической,
// закрытого пакета): у неё нет main(), который в программе зовёт
// rech_init_statics()/rech_init_globals() (entity_codegen.go), поэтому
// то же самое делает функция с атрибутом constructor - загрузчик (или
// стартовый код программы-потребителя, если архив вшит при линковке)
// вызывает её до main() потребителя. Без неё статические поля и переменные
// модуля библиотеки оставались нулями. "" - не библиотека или
// инициализировать нечего.
func libraryInitCode(ctx *irGenCtx) string {
	if !ctx.libMode || (!ctx.usesStaticInit && !ctx.usesGlobalInit) {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("__attribute__((constructor)) static void rech_init_library(void) {\n")
	if ctx.usesGC {
		sb.WriteString("    GC_INIT();\n")
	}
	if ctx.usesStaticInit {
		sb.WriteString("    rech_init_statics();\n")
	}
	if ctx.usesGlobalInit {
		sb.WriteString("    rech_init_globals();\n")
	}
	sb.WriteString("}\n\n")
	return sb.String()
}
