package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateFromIRExplicitConstructorSplitsInitFromNew - у сущности с
// явным конструктором (ent.Ctor != nil) "rech_new_X" - тонкая обёртка
// (GC_MALLOC + вызов rech_init_X) вместо прежнего designated-по-полю
// заполнения - см. package doc generateIREntityCtorInit.
func TestGenerateFromIRExplicitConstructorSplitsInitFromNew(t *testing.T) {
	ent := &ir.Entity{
		Name:   "Точка",
		Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}, {Name: "y", Type: ir.TypeInt}},
		Ctor: &ir.Method{
			Params: []ir.Param{{Name: "x", Type: ir.TypeInt}, {Name: "y", Type: ir.TypeInt}},
			Body: []ir.Stmt{
				&ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: "Точка"}, Field: "x", Value: &ir.Identifier{Name: "x", Type: ir.TypeInt}},
				&ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: "Точка"}, Field: "y", Value: &ir.Identifier{Name: "y", Type: ir.TypeInt}},
			},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "p", Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.IntLiteral{Value: 1}, &ir.IntLiteral{Value: 2}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{ent}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static void rech_init_tochka(rech_T_tochka *this_, long long v_x, long long v_y) {") {
		t.Fatalf("ожидалась функция инициализации конструктора, получено:\n%s", out)
	}
	if !strings.Contains(out, "(this_)->f_x = v_x;") || !strings.Contains(out, "(this_)->f_y = v_y;") {
		t.Fatalf("ожидалось присваивание полей внутри инициализации, получено:\n%s", out)
	}
	if !strings.Contains(out, "static rech_T_tochka *rech_new_tochka(long long v_x, long long v_y) {") {
		t.Fatalf("ожидалась обёртка rech_new_tochka, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_T_tochka *p = GC_MALLOC(sizeof(rech_T_tochka));") ||
		!strings.Contains(out, "rech_init_tochka(p, v_x, v_y);") {
		t.Fatalf("ожидался GC_MALLOC + вызов инициализации в обёртке, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_T_tochka * v_p_INIT = rech_new_tochka(1LL, 2LL);") {
		t.Fatalf("ожидался вызов rech_new_tochka с аргументами конструктора, получено:\n%s", out)
	}
}

// TestGenerateFromIREntityWithoutCtorUnchangedBehavior - сущность без
// ent.Ctor продолжает старую позиционную форму (один rech_new_X,
// заполняющий поля напрямую) - явный конструктор не должен менять
// поведение для сущностей, которые его не объявили.
func TestGenerateFromIREntityWithoutCtorUnchangedBehavior(t *testing.T) {
	ent := &ir.Entity{Name: "Точка", Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "p", Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.IntLiteral{Value: 1}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{ent}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_init_tochka") {
		t.Fatalf("не ожидалась функция инициализации у сущности без явного конструктора, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_T_tochka *p = GC_MALLOC(sizeof(rech_T_tochka));\n    p->f_x = v_x;") {
		t.Fatalf("ожидалось прежнее прямое присваивание поля, получено:\n%s", out)
	}
}

// TestGenerateFromIRParentCallStmtCallsBaseInit - "родитель(...)"
// компилируется в прямой вызов rech_init_БАЗА(this_, ...) - на том же
// указателе, без нового GC_MALLOC.
func TestGenerateFromIRParentCallStmtCallsBaseInit(t *testing.T) {
	base := &ir.Entity{
		Name:   "Пользователь",
		Fields: []ir.Field{{Name: "имя", Type: ir.TypeString}},
		Ctor: &ir.Method{
			Params: []ir.Param{{Name: "имя", Type: ir.TypeString}},
			Body: []ir.Stmt{
				&ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: "Пользователь"}, Field: "имя", Value: &ir.Identifier{Name: "имя", Type: ir.TypeString}},
			},
		},
	}
	derived := &ir.Entity{
		Name: "Сотрудник", BaseName: "Пользователь",
		Fields: []ir.Field{{Name: "имя", Type: ir.TypeString}, {Name: "зарплата", Type: ir.TypeInt}},
		Ctor: &ir.Method{
			Params: []ir.Param{{Name: "имя", Type: ir.TypeString}, {Name: "зарплата", Type: ir.TypeInt}},
			Body: []ir.Stmt{
				&ir.ParentCallStmt{BaseEntity: "Пользователь", Args: []ir.Expr{&ir.Identifier{Name: "имя", Type: ir.TypeString}}},
				&ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: "Сотрудник"}, Field: "зарплата", Value: &ir.Identifier{Name: "зарплата", Type: ir.TypeInt}},
			},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "с", Value: &ir.NewExpr{Entity: "Сотрудник", Args: []ir.Expr{&ir.StringLiteral{Value: "Аня"}, &ir.IntLiteral{Value: 1000}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{base, derived}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_init_polzovatel(this_, v_imya);") {
		t.Fatalf("ожидался вызов rech_init_polzovatel(this_, ...) внутри rech_init_sotrudnik, получено:\n%s", out)
	}
	initSotrudnik := out[strings.Index(out, "static void rech_init_sotrudnik"):strings.Index(out, "static rech_T_sotrudnik *rech_new_sotrudnik")]
	if strings.Contains(initSotrudnik, "rech_new_polzovatel(") {
		t.Fatalf("\"родитель(...)\" не должен создавать новый объект (без rech_new_polzovatel) - вызов должен идти телу rech_init_sotrudnik:\n%s", initSotrudnik)
	}
}
