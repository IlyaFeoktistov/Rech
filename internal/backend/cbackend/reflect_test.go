package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRTypeOfPrimitiveEmitsDescriptor - "тип Целое" - адрес
// статического дескриптора примитива, эмитится безусловно при
// UsesReflection (даже без единой сущности в ReflectTargets).
func TestGenerateFromIRTypeOfPrimitiveEmitsDescriptor(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "v_t_INIT = (&rech_type_meta_celoe);") {
		t.Fatalf("ожидался адрес дескриптора Целое, получено:\n%s", out)
	}
	if !strings.Contains(out, `static const rech_type_meta_t rech_type_meta_celoe = {"Целое", NULL, 0, NULL, 0, NULL, 0};`) {
		t.Fatalf("ожидался статический дескриптор примитива Целое, получено:\n%s", out)
	}
}

// TestGenerateFromIRBoxPrimitiveCast - "(Значение) 5" - инлайновый
// struct-литерал rech_any_t, без рантайм-хелпера (примитив).
func TestGenerateFromIRBoxPrimitiveCast(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 5}, To: ir.TypeAny}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(rech_any_t){.type = &rech_type_meta_celoe, .v.i = (long long)(5LL)}") {
		t.Fatalf("ожидалось боксирование Целое в rech_any_t, получено:\n%s", out)
	}
}

// TestGenerateFromIRAsExprUsesCheckedUnbox - "значение как Целое" -
// rech_unbox_check (паника при несовпадении типа) перед чтением union.
func TestGenerateFromIRAsExprUsesCheckedUnbox(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "коробка", Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 5}, To: ir.TypeAny}},
		&ir.PrintStmt{Value: &ir.AsExpr{Value: &ir.Identifier{Name: "коробка", Type: ir.TypeAny}, To: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "((long long)(rech_unbox_check(v_korobka, &rech_type_meta_celoe)).v.i)") {
		t.Fatalf("ожидался rech_unbox_check перед распаковкой, получено:\n%s", out)
	}
}

// TestGenerateFromIRAnyMethodsDispatchToReflectHelpers - методы "Значение"
// генерируются как вызовы отдельных рантайм-хелперов, не через обычный
// mangleMethod-путь сущности/интерфейса.
func TestGenerateFromIRAnyMethodsDispatchToReflectHelpers(t *testing.T) {
	recv := &ir.Identifier{Name: "коробка", Type: ir.TypeAny}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "коробка", Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 5}, To: ir.TypeAny}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "тип_значения", Type: ir.TypeTypeInfo, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "получить_поле", Args: []ir.Expr{&ir.StringLiteral{Value: "имя"}}, Type: ir.TypeAny, HasReturn: true}},
		&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{Receiver: recv, Method: "установить_поле", Args: []ir.Expr{&ir.StringLiteral{Value: "имя"}, &ir.CastExpr{Value: &ir.IntLiteral{Value: 1}, To: ir.TypeAny}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(v_korobka).type") {
		t.Fatalf("ожидалось чтение .type у тип_значения, получено:\n%s", out)
	}
	if !strings.Contains(out, `rech_reflect_get_field(v_korobka, "имя")`) {
		t.Fatalf("ожидался вызов rech_reflect_get_field, получено:\n%s", out)
	}
	if !strings.Contains(out, `rech_reflect_set_field(v_korobka, "имя",`) {
		t.Fatalf("ожидался вызов rech_reflect_set_field, получено:\n%s", out)
	}
}

// TestGenerateFromIRTypeInfoMethodsDispatchToReflectHelpers - методы "Тип"
// (имя/число_полей/...) читают поля дескриптора напрямую или через
// индексные хелперы rech_field_at/rech_method_at/rech_param_at.
func TestGenerateFromIRTypeInfoMethodsDispatchToReflectHelpers(t *testing.T) {
	recv := &ir.Identifier{Name: "т", Type: ir.TypeTypeInfo}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.TypeInt}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "имя", Type: ir.TypeString, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "число_полей", Type: ir.TypeInt, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{Receiver: recv, Method: "имя_поля", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.TypeString, HasReturn: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}, UsesReflection: true})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(v_t)->name") {
		t.Fatalf("ожидалось чтение ->name, получено:\n%s", out)
	}
	if !strings.Contains(out, "(v_t)->field_count") {
		t.Fatalf("ожидалось чтение ->field_count, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_field_at(v_t, 0LL)->name") {
		t.Fatalf("ожидался вызов rech_field_at, получено:\n%s", out)
	}
}

// TestGenerateFromIREntityReflectTargetGeneratesAccessorsAndDescriptor -
// сущность из prog.ReflectTargets получает геттер/сеттер поля, инвокер
// метода, обёртку конструктора и собственный дескриптор типа.
func TestGenerateFromIREntityReflectTargetGeneratesAccessorsAndDescriptor(t *testing.T) {
	entity := &ir.Entity{
		Name:   "Точка",
		Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}},
		Methods: []*ir.Method{
			{Name: "координата", ReturnType: ir.TypeInt, HasReturn: true,
				Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.FieldAccess{Receiver: &ir.ThisExpr{Entity: "Точка"}, Field: "x", Type: ir.TypeInt}}}},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.TypeOfExpr{Type: ir.EntityType("Точка")}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main},
		UsesReflection: true, ReflectTargets: []ir.Type{ir.EntityType("Точка")},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static rech_any_t rech_reflect_get_tochka_x(void *self) {") {
		t.Fatalf("ожидался геттер поля x, получено:\n%s", out)
	}
	if !strings.Contains(out, "static void rech_reflect_set_tochka_x(void *self, rech_any_t v) {") {
		t.Fatalf("ожидался сеттер поля x, получено:\n%s", out)
	}
	if !strings.Contains(out, "static rech_any_t rech_reflect_invoke_tochka_koordinata(void *self, rech_any_t *args, int argc) {") {
		t.Fatalf("ожидался инвокер метода координата, получено:\n%s", out)
	}
	if !strings.Contains(out, "static rech_any_t rech_reflect_ctor_tochka(rech_any_t *args, int argc) {") {
		t.Fatalf("ожидалась обёртка конструктора, получено:\n%s", out)
	}
	if !strings.Contains(out, `static const rech_type_meta_t rech_type_meta_tochka = {"Точка",`) {
		t.Fatalf("ожидался дескриптор типа Точка, получено:\n%s", out)
	}
}

// TestGenerateFromIRReflectTypedefsBeforeGenerics - rech_any_t обязан быть
// объявлен раньше genericsCode (Список<Значение> ссылается на него как на
// свой C-тип элемента) - иначе "unknown type name".
func TestGenerateFromIRReflectTypedefsBeforeGenerics(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "список", Value: &ir.NewExpr{Entity: "Список<Значение>"}},
	}}
	out, err := GenerateFromIR(&ir.Program{
		Functions: []*ir.Function{main}, UsesReflection: true,
		Generics: []ir.GenericInstantiation{{Name: "Список<Значение>", Template: "Список", ElemType: ir.TypeAny}},
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	typedefPos := strings.Index(out, "typedef struct {\n    const rech_type_meta_t *type;")
	listPos := strings.Index(out, "rech_any_t *data;")
	if typedefPos == -1 || listPos == -1 || typedefPos >= listPos {
		t.Fatalf("ожидался rech_any_t typedef раньше Список<Значение>, получено:\n%s", out)
	}
}
