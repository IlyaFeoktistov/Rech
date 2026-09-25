package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// entryPrologue - начало тела "главная" после инициализации рантайма:
// параметр "главная(имя: Список<Строка>)" получает аргументы командной
// строки без имени программы (rech_main_args). C-сигнатура точки входа
// при этом остаётся "(int argc, char **argv)".
func entryPrologue(ctx *irGenCtx, fn *ir.Function) string {
	if len(fn.Params) == 0 {
		return ""
	}
	p := fn.Params[0]
	ctx.mainArgsType = p.Type.Entity
	ctx.usesText = true
	return fmt.Sprintf("    %s %s = rech_main_args();\n", fromIRType(p.Type).cType(ctx), mangleVar(p.Name))
}

// mainArgsHelper - rech_main_args: Список<Строка> из rech_argv[1..]
// (argsGlobals - на Windows уже UTF-8 через CommandLineToArgvW), каждый
// аргумент - валидный текст (rech_text_z), как любая строка извне.
func mainArgsHelper(listType, addMethod string) string {
	return fmt.Sprintf(`static %[1]s *rech_main_args(void) {
    %[1]s *out = GC_MALLOC(sizeof(%[1]s));
    for (int i = 1; i < rech_argc; i++) {
        %[2]s(out, rech_text_z(rech_argv[i]));
    }
    return out;
}

`, listType, addMethod)
}
