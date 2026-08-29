package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRGerundStmtWrapsBodyInBraces - GerundStmt эмитится как
// голый "{ ... }" (собственная область видимости C для "результат", ничего
// специального сверх этого) - Body уже несёт VarDecl "результат" + вывести,
// см. package doc internal/semantic/gerund.go.
func TestGenerateFromIRGerundStmtWrapsBodyInBraces(t *testing.T) {
	fn := &ir.Function{
		Name: "сложить", Params: []ir.Param{{Name: "а", Type: ir.TypeInt}, {Name: "б", Type: ir.TypeInt}},
		ReturnType: ir.TypeInt, HasReturn: true,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "+", Left: &ir.Identifier{Name: "а", Type: ir.TypeInt}, Right: &ir.Identifier{Name: "б", Type: ir.TypeInt}, Type: ir.TypeInt}}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.GerundStmt{Body: []ir.Stmt{
			&ir.VarDecl{Name: "результат", Value: &ir.CallExpr{Callee: "сложить", Args: []ir.Expr{&ir.IntLiteral{Value: 2}, &ir.IntLiteral{Value: 3}}, Type: ir.TypeInt, HasReturn: true}},
			&ir.PrintStmt{Value: &ir.Identifier{Name: "результат", Type: ir.TypeInt}},
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"long long v_rezultat_INIT = rech_slozhit(2LL, 3LL);",
		"long long v_rezultat = v_rezultat_INIT;",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRVarDeclSelfReferenceUnderShadowingReadsOuterValue -
// регрессионный тест на находку сессии: "переменная х = х + 1" внутри
// вложенного блока (тот же C-код, что генерирует GerundStmt для
// "результат") при затенении внешней "х" раньше читал ещё не
// проинициализированную (мусорную) внутреннюю "х" - в C имя объявляемой
// переменной входит в область видимости уже в точке её собственного
// объявления, до инициализатора. Не про GerundStmt конкретно - про
// generateIRStmt(*ir.VarDecl) вообще, см. её package doc.
func TestGenerateFromIRVarDeclSelfReferenceUnderShadowingReadsOuterValue(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "х", Value: &ir.IntLiteral{Value: 5}},
		&ir.IfStmt{Cond: &ir.BoolLiteral{Value: true}, Then: []ir.Stmt{
			&ir.VarDecl{Name: "х", Value: &ir.BinaryExpr{Op: "+", Left: &ir.Identifier{Name: "х", Type: ir.TypeInt}, Right: &ir.IntLiteral{Value: 1}, Type: ir.TypeInt}},
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "long long v_h_INIT = (v_h + 1LL);\n") || !strings.Contains(out, "long long v_h = v_h_INIT;\n") {
		t.Fatalf("ожидалось чтение внешней \"х\" через временную переменную ДО объявления внутренней, получено:\n%s", out)
	}
}
