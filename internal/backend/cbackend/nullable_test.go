package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRNullableScalarEmitsWrapperAndHelpers - "?Целое" -
// typedef struct (тег занятости + значение) и обернуть/пусто/развернуть/??,
// см. package doc internal/backend/cbackend/nullable.go.
func TestGenerateFromIRNullableScalarEmitsWrapperAndHelpers(t *testing.T) {
	nt := ir.NullableType(ir.TypeInt)
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.NullLiteral{Type: nt}},
		&ir.PrintStmt{Value: &ir.NullCheckExpr{Value: &ir.Identifier{Name: "а", Type: nt}, IsNot: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, Nullables: []ir.NullableInstantiation{{Name: nt.Entity, Inner: ir.TypeInt}}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	structName := mangleType(nt.Entity)
	for _, want := range []string{
		"typedef struct {",
		structName + "_wrap(",
		structName + "_empty(",
		structName + "_unwrap(",
		structName + "_coalesce(",
		structName + "_coalesce_n(",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось вхождение %q в сгенерированный C, получено:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "(v_a).has") {
		t.Fatalf("ожидалась проверка .has у \"!= Ничто\", получено:\n%s", out)
	}
}

// TestGenerateFromIRNullableEntityOrderedAfterEntityTypedef - "?Тип" с
// внутренним типом-сущностью нужен C-typedef самой сущности РАНЬШЕ (её
// wrap/unwrap-функции принимают/возвращают "rech_T_X *" по имени) - в
// отличие от "?Целое"/"?Список<T>", которые нужны РАНЬШЕ typedef сущностей
// (сущность может нести nullable-поле по значению) - см. package doc
// generateSource, разбиение nullablesCode/nullablesAfterTypedefs.
func TestGenerateFromIRNullableEntityOrderedAfterEntityTypedef(t *testing.T) {
	entType := ir.EntityType("Пользователь")
	nt := ir.NullableType(entType)
	ent := &ir.Entity{Name: "Пользователь", Fields: []ir.Field{{Name: "имя", Type: ir.TypeString}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "п", Value: &ir.NullLiteral{Type: nt}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Functions: []*ir.Function{main},
		Entities:  []*ir.Entity{ent},
		Nullables: []ir.NullableInstantiation{{Name: nt.Entity, Inner: entType}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	entTypedefIdx := strings.Index(out, "} "+mangleType(ent.Name)+";")
	nullableStructIdx := strings.Index(out, "} "+mangleType(nt.Entity)+";")
	if entTypedefIdx == -1 || nullableStructIdx == -1 || nullableStructIdx < entTypedefIdx {
		t.Fatalf("ожидался typedef сущности \"Пользователь\" РАНЬШЕ обёртки \"?Пользователь\", получено:\n%s", out)
	}
}
