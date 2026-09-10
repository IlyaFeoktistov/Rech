package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRRecursiveMainCallPassesArgcArgv - "главная()", вызванная
// из своего же тела (или откуда угодно ещё в программе) - манглится в
// настоящий C "main" (mangleName, entryPointName), а его сигнатура ВСЕГДА
// "(int argc, char **argv)", не пустая, даже когда сам Rech-вызов "главная()"
// не передаёт ни одного аргумента (у неё их и не может быть, docs/SPEC.md) -
// без явного проброса "rech_argc, rech_argv" на месте вызова получалось
// "main()" при объявлении "int main(int argc, char **argv)", что cc
// отказывался собирать ("too few arguments to function 'main'") - найдено
// живьём при проверке, можно ли вообще вызвать "главная" из самого же
// Rech-кода (пользователь спросил).
func TestGenerateFromIRRecursiveMainCallPassesArgcArgv(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "главная"}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "main(rech_argc, rech_argv)") {
		t.Fatalf("ожидался вызов main(rech_argc, rech_argv), получено:\n%s", out)
	}
	if strings.Contains(out, "main();") {
		t.Fatalf("не ожидался пустой вызов main() - несовпадение с прототипом main(int, char**), получено:\n%s", out)
	}
}
