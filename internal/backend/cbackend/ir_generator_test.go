package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateFromIRPrintString(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.StringLiteral{Value: "Привет, мир!"}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `int main(void) {`) {
		t.Fatalf("ожидалась сигнатура main, получено:\n%s", out)
	}
	if !strings.Contains(out, `printf("%s\n", "Привет, мир!");`) {
		t.Fatalf("ожидался вызов printf, получено:\n%s", out)
	}
}

func TestGenerateFromIRVarDeclAssignAndArithmetic(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "а", Value: &ir.IntLiteral{Value: 10}},
			&ir.Assign{Name: "а", Value: &ir.BinaryExpr{
				Op: "+", Left: &ir.Identifier{Name: "а", Type: ir.TypeInt}, Right: &ir.IntLiteral{Value: 1}, Type: ir.TypeInt,
			}},
			&ir.PrintStmt{Value: &ir.Identifier{Name: "а", Type: ir.TypeInt}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "long long v_a = 10LL;") {
		t.Fatalf("ожидалось объявление переменной, получено:\n%s", out)
	}
	if !strings.Contains(out, "v_a = (v_a + 1LL);") {
		t.Fatalf("ожидалось присваивание с арифметикой, получено:\n%s", out)
	}
}

func TestGenerateFromIRStringConcatUsesHelper(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.BinaryExpr{
				Op: "+", Left: &ir.StringLiteral{Value: "Привет, "}, Right: &ir.StringLiteral{Value: "мир!"}, Type: ir.TypeString,
			}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_str_concat(") || !strings.Contains(out, "static char *rech_str_concat") {
		t.Fatalf("ожидался вызов и определение rech_str_concat, получено:\n%s", out)
	}
}

func TestGenerateFromIRIntFloatPromotion(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.BinaryExpr{
				Op: "+", Left: &ir.IntLiteral{Value: 1}, Right: &ir.FloatLiteral{Value: 2.5}, Type: ir.TypeFloat,
			}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "((double)1LL) + 2.5") {
		t.Fatalf("ожидалось приведение Целое к double при смешении с Число, получено:\n%s", out)
	}
}

func TestGenerateFromIRIfWhileForReturn(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.IfStmt{
				Cond: &ir.BoolLiteral{Value: true},
				Then: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "да"}}},
				Else: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "нет"}}},
			},
			&ir.WhileStmt{
				Cond: &ir.BoolLiteral{Value: false},
				Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "x"}}},
			},
			&ir.ForStmt{
				Var:  "i",
				From: &ir.IntLiteral{Value: 1},
				To:   &ir.IntLiteral{Value: 5},
				Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.Identifier{Name: "i", Type: ir.TypeInt}}},
			},
			&ir.ReturnStmt{},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"if (true) {", "} else {",
		"while (false) {",
		"for (long long v_i = 1LL; v_i <= 5LL; v_i++) {",
		"return 0;",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRFunctionWithParamsCallAndReturn(t *testing.T) {
	sum := &ir.Function{
		Name:       "сложить",
		Params:     []ir.Param{{Name: "что", Type: ir.TypeInt}, {Name: "кем", Type: ir.TypeInt}},
		ReturnType: ir.TypeInt,
		HasReturn:  true,
		Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{
			Op: "+", Left: &ir.Identifier{Name: "что", Type: ir.TypeInt}, Right: &ir.Identifier{Name: "кем", Type: ir.TypeInt}, Type: ir.TypeInt,
		}}},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CallExpr{
			Callee: "сложить", Args: []ir.Expr{&ir.IntLiteral{Value: 2}, &ir.IntLiteral{Value: 3}}, Type: ir.TypeInt, HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{sum, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "long long rech_slozhit(long long v_chto, long long v_kem);") {
		t.Fatalf("ожидался прототип функции, получено:\n%s", out)
	}
	if !strings.Contains(out, "long long rech_slozhit(long long v_chto, long long v_kem) {") {
		t.Fatalf("ожидалось определение функции, получено:\n%s", out)
	}
	if !strings.Contains(out, "return (v_chto + v_kem);") {
		t.Fatalf("ожидался возврат значения, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_slozhit(2LL, 3LL)") {
		t.Fatalf("ожидался вызов функции, получено:\n%s", out)
	}
}

func TestGenerateFromIREntityFieldsConstructorMethodsAndThis(t *testing.T) {
	entity := &ir.Entity{
		Name:   "Пользователь",
		Fields: []ir.Field{{Name: "возраст", Type: ir.TypeInt}},
		Methods: []*ir.Method{
			{
				Name: "состариться",
				Body: []ir.Stmt{&ir.FieldAssign{
					Receiver: &ir.ThisExpr{Entity: "Пользователь"},
					Field:    "возраст",
					Value: &ir.BinaryExpr{
						Op: "+", Left: &ir.FieldAccess{Receiver: &ir.ThisExpr{Entity: "Пользователь"}, Field: "возраст", Type: ir.TypeInt},
						Right: &ir.IntLiteral{Value: 1}, Type: ir.TypeInt,
					},
				}},
			},
			{
				Name: "представиться", ReturnType: ir.TypeInt, HasReturn: true,
				Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.FieldAccess{
					Receiver: &ir.ThisExpr{Entity: "Пользователь"}, Field: "возраст", Type: ir.TypeInt,
				}}},
			},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "пользователь", Value: &ir.NewExpr{Entity: "Пользователь", Args: []ir.Expr{&ir.IntLiteral{Value: 30}}}},
		&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "пользователь", Type: ir.EntityType("Пользователь")}, Entity: "Пользователь", Method: "состариться",
		}},
		&ir.PrintStmt{Value: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "пользователь", Type: ir.EntityType("Пользователь")}, Entity: "Пользователь", Method: "представиться",
			Type: ir.TypeInt, HasReturn: true,
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "typedef struct {") || !strings.Contains(out, "long long f_vozrast;\n} rech_T_polzovatel;") {
		t.Fatalf("ожидался typedef struct для сущности, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_T_polzovatel v_polzovatel = (rech_T_polzovatel){.f_vozrast = 30LL};") {
		t.Fatalf("ожидался конструктор (compound literal), получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_polzovatel_sostaritsya(&(v_polzovatel));") {
		t.Fatalf("ожидался вызов метода с адресом получателя, получено:\n%s", out)
	}
	if !strings.Contains(out, "((*this_)).f_vozrast = (((*this_)).f_vozrast + 1LL);") {
		t.Fatalf("ожидалась мутация поля через \"этот\", получено:\n%s", out)
	}
	if !strings.Contains(out, "return ((*this_)).f_vozrast;") {
		t.Fatalf("ожидался возврат поля через \"этот\", получено:\n%s", out)
	}
}

func TestGenerateFromIRBecomeStmtEmitsNoCode(t *testing.T) {
	entity := &ir.Entity{
		Name:   "Пользователь",
		Fields: []ir.Field{{Name: "имя", Type: ir.TypeString}},
		Methods: []*ir.Method{
			{Name: "авторизовать", Body: []ir.Stmt{
				&ir.BecomeStmt{Receiver: &ir.ThisExpr{Entity: "Пользователь"}, State: "авторизован"},
			}},
		},
	}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{entity}, Functions: []*ir.Function{mainFuncIR()}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "int rech_M_polzovatel_avtorizovat(rech_T_polzovatel *this_) {\n    return 0;\n}") {
		t.Fatalf("ожидалось пустое тело метода (\"становится\" не эмитит код), получено:\n%s", out)
	}
}

func TestGenerateFromIRIntDivisionUsesCheckedHelper(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.BinaryExpr{Op: "/", Left: &ir.IntLiteral{Value: 7}, Right: &ir.IntLiteral{Value: 2}, Type: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_int_div(7LL, 2LL)") {
		t.Fatalf("ожидался вызов rech_int_div, получено:\n%s", out)
	}
	if !strings.Contains(out, "static long long rech_int_div(") {
		t.Fatalf("ожидалось определение rech_int_div, получено:\n%s", out)
	}
}

func TestGenerateFromIRAssertStmt(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.AssertStmt{Cond: &ir.BoolLiteral{Value: true}, Line: 4},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `if (!(true)) { rech_panic("утверждение не выполнено (строка 4)"); }`) {
		t.Fatalf("ожидалась проверка условия с rech_panic и номером строки, получено:\n%s", out)
	}
	if !strings.Contains(out, "static void rech_panic(") {
		t.Fatalf("ожидалось определение rech_panic, получено:\n%s", out)
	}
}

func mainFuncIR() *ir.Function {
	return &ir.Function{Name: "главная", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "ок"}}}}
}

func TestGenerateFromIRListGenericAndForEach(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "числа", Value: &ir.NewExpr{Entity: "Список<Целое>"}},
		&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "числа", Type: ir.EntityType("Список<Целое>")},
			Entity:   "Список<Целое>", Method: "добавить",
			Args: []ir.Expr{&ir.IntLiteral{Value: 5}},
		}},
		&ir.ForEachStmt{
			Var:        "x",
			Collection: &ir.Identifier{Name: "числа", Type: ir.EntityType("Список<Целое>")},
			ElemType:   ir.TypeInt,
			Body:       []ir.Stmt{&ir.PrintStmt{Value: &ir.Identifier{Name: "x", Type: ir.TypeInt}}},
		},
	}}
	prog := &ir.Program{
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "#include <stdlib.h>") || !strings.Contains(out, "static void rech_panic(") {
		t.Fatalf("ожидались stdlib.h и rech_panic (нужны generic-коллекциям), получено:\n%s", out)
	}
	if !strings.Contains(out, "} rech_T_spisok_of_celoe;") {
		t.Fatalf("ожидался typedef struct для Список<Целое>, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_spisok_of_celoe_dobavit(&(v_chisla), 5LL);") {
		t.Fatalf("ожидался вызов добавить, получено:\n%s", out)
	}
	if !strings.Contains(out, "for (long long rech_fe_i_v_x = 0; rech_fe_i_v_x < v_chisla.len; rech_fe_i_v_x++) {") {
		t.Fatalf("ожидался цикл по данным Списка, получено:\n%s", out)
	}
	if !strings.Contains(out, "long long v_x = v_chisla.data[rech_fe_i_v_x];") {
		t.Fatalf("ожидалось присваивание элемента переменной цикла, получено:\n%s", out)
	}
}

func TestGenerateFromIRStackAndQueueGenerics(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "ок"}}}}},
		Generics: []ir.GenericInstantiation{
			{Name: "Стопка<Строка>", Template: "Стопка", ElemType: ir.TypeString},
			{Name: "Очередь<Логическое>", Template: "Очередь", ElemType: ir.TypeBool},
		},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_T_stopka_of_stroka") {
		t.Fatalf("ожидалась инстанциация Стопки, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_T_ochered_of_logicheskoe") {
		t.Fatalf("ожидалась инстанциация Очереди, получено:\n%s", out)
	}
}

func TestGenerateFromIRUnknownGenericTemplateFails(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: nil}},
		Generics:  []ir.GenericInstantiation{{Name: "Мешок<Целое>", Template: "Мешок", ElemType: ir.TypeInt}},
	}
	_, err := GenerateFromIR(prog)
	if err == nil || !strings.Contains(err.Error(), "неизвестный generic-шаблон") {
		t.Fatalf("ожидалась ошибка про неизвестный шаблон, получено: %v", err)
	}
}

func TestGenerateFromIRMissingEntryPointFails(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "чтототам", Body: nil},
	}}
	_, err := GenerateFromIR(prog)
	if err == nil || !strings.Contains(err.Error(), "не найдена функция") {
		t.Fatalf("ожидалась ошибка про отсутствие точки входа, получено: %v", err)
	}
}

func TestGenerateFromIRBitwiseOperators(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.BinaryExpr{Op: "|", Left: &ir.IntLiteral{Value: 6}, Right: &ir.IntLiteral{Value: 3}, Type: ir.TypeInt}},
		&ir.PrintStmt{Value: &ir.UnaryExpr{Op: "~", Operand: &ir.IntLiteral{Value: 0}, Type: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(6LL | 3LL)") || !strings.Contains(out, "(~0LL)") {
		t.Fatalf("ожидались побитовые C-операторы, получено:\n%s", out)
	}
}

func TestGenerateFromIRIncDecExpr(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.IntLiteral{Value: 5}},
		&ir.PrintStmt{Value: &ir.IncDecExpr{Name: "а", Op: "++", Prefix: false, Type: ir.TypeInt}},
		&ir.ExprStmt{IncDec: &ir.IncDecExpr{Name: "а", Op: "--", Prefix: true, Type: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(v_a++)") || !strings.Contains(out, "(--v_a);") {
		t.Fatalf("ожидались C-постфикс/префикс ++/--, получено:\n%s", out)
	}
}
