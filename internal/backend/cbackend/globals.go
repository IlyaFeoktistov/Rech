package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// Переменные уровня модуля (ir.Program.Globals) - глобальные C-переменные
// под тем же именем, что и локальная переменная (mangleVar): обращение и
// присваивание в IR - обычные Identifier/Assign, локальная переменная или
// параметр с тем же именем перекрывает глобальную по правилам областей
// видимости C, как и в Rech. Значения присваиваются один раз в
// rech_init_globals(), до первой инструкции "главная" (после статических
// полей сущностей, см. statics.go), в порядке объявления.

// globalVarsDecl - "static Тип v_имя;" на каждую переменную модуля.
func globalVarsDecl(ctx *irGenCtx, prog *ir.Program) string {
	var sb strings.Builder
	for _, g := range prog.Globals {
		fmt.Fprintf(&sb, "static %s %s;\n", fromIRType(g.Type).cType(ctx), mangleVar(g.Name))
	}
	if sb.Len() > 0 {
		sb.WriteString("\n")
	}
	return sb.String()
}

// globalVarsInitCode - "static void rech_init_globals(void) { ... }", ""
// без переменных модуля. Каждое значение - обычное присваивание
// (generateIRStmt): временные переменные цепочек и номер строки для стека
// вызовов - как у инструкции тела функции.
func globalVarsInitCode(ctx *irGenCtx, prog *ir.Program) (string, error) {
	if len(prog.Globals) == 0 {
		return "", nil
	}
	var body strings.Builder
	for _, g := range prog.Globals {
		if err := generateIRStmt(&body, ctx, &ir.Assign{Name: g.Name, Value: g.Value, Line: g.Line}, 1); err != nil {
			return "", fmt.Errorf("переменная модуля %q: %w", g.Name, err)
		}
	}
	return "static void rech_init_globals(void) {\n" + body.String() + "}\n\n", nil
}
