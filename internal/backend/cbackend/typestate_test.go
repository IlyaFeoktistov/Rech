package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRStateCheckExprEmitsRuntimeHelperCall - ir.StateCheckExpr
// компилируется в вызов rech_require_state с точным смещением поля
// "state_flags" (offsetof), номером бита и уже готовым текстом сообщения -
// см. package doc internal/backend/cbackend/typestate.go.
func TestGenerateFromIRStateCheckExprEmitsRuntimeHelperCall(t *testing.T) {
	polzovatel := &ir.Entity{
		Name:          "Пользователь",
		Fields:        []ir.Field{{Name: "имя", Type: ir.TypeString}},
		StateNames:    []string{"авторизован"},
		HasStateFlags: true,
	}
	// Поле "имя" через проверенное значение - StateCheckExpr composes
	// прозрачно везде, где ожидался бы обычный указатель на сущность
	// (в отличие от прямого "вывести <сущность>" ниже - оно печатает
	// дружелюбный плейсхолдер, не сам код значения, см. package doc
	// printCall/typeEntity - неподходящий сценарий для проверки того, что
	// именно генерирует StateCheckExpr).
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.FieldAccess{
			Receiver: &ir.StateCheckExpr{
				Value:  &ir.Identifier{Name: "п", Type: ir.EntityType("Пользователь")},
				Entity: "Пользователь",
				State:  "авторизован",
			},
			Field: "имя",
			Type:  ir.TypeString,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{polzovatel}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static void *rech_require_state(void *obj, size_t offset, uint64_t bit, const char *msg) {") {
		t.Fatalf("ожидался рантайм-хелпер rech_require_state, получено:\n%s", out)
	}
	if !strings.Contains(out, "(rech_T_polzovatel *)rech_require_state((void *)(v_p), offsetof(rech_T_polzovatel, state_flags), 1ULL << 0,") {
		t.Fatalf("ожидался вызов rech_require_state с offsetof и битом состояния, получено:\n%s", out)
	}
	if !strings.Contains(out, "#include <stddef.h>") {
		t.Fatalf("ожидался #include <stddef.h> (offsetof), получено:\n%s", out)
	}
}

// TestGenerateFromIRHasStateExprReadsFlagDirectly - ir.HasStateExpr, в
// отличие от StateCheckExpr, не проходит через общий рантайм-хелпер
// (Entity уже точно известен на этапе кодогенерации) - прямой typed-каст
// и битовая проверка, без offsetof/rech_require_state.
func TestGenerateFromIRHasStateExprReadsFlagDirectly(t *testing.T) {
	polzovatel := &ir.Entity{
		Name:          "Пользователь",
		Fields:        []ir.Field{{Name: "имя", Type: ir.TypeString}},
		StateNames:    []string{"авторизован"},
		HasStateFlags: true,
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.IfStmt{
			Cond: &ir.HasStateExpr{
				Value:  &ir.Identifier{Name: "п", Type: ir.EntityType("Пользователь")},
				Entity: "Пользователь",
				State:  "авторизован",
			},
			Then: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "да"}}},
		},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{polzovatel}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "((((rech_T_polzovatel *)(v_p))->state_flags & (1ULL << 0)) != 0)") {
		t.Fatalf("ожидалось прямое чтение бита состояния, получено:\n%s", out)
	}
}
