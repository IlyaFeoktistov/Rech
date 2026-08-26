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
	if !strings.Contains(out, `int main(int argc, char **argv) {`) {
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

// TestGenerateFromIRFloatDivisionDoesNotUseCheckedHelper - "/" между Число
// (в отличие от Целое) - обычный оператор C: деление на 0.0 даёт IEEE 754
// +-Inf/NaN, не панику, поэтому rech_int_div здесь не нужен и не должен
// генерироваться.
func TestGenerateFromIRFloatDivisionDoesNotUseCheckedHelper(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.BinaryExpr{Op: "/", Left: &ir.FloatLiteral{Value: 7}, Right: &ir.FloatLiteral{Value: 2}, Type: ir.TypeFloat}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(7.0 / 2.0)") {
		t.Fatalf("ожидался обычный оператор \"/\", получено:\n%s", out)
	}
	if strings.Contains(out, "rech_int_div") {
		t.Fatalf("rech_int_div не должен генерироваться для деления Число: %s", out)
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

// TestGenerateFromIRBuiltinMathCall - паритет с AST-путём (шаг 5): builtin
// "корень" даёт inline C-выражение (sqrt), не вызов по имени, как обычная
// функция программы (mangleName) - см. builtinCallCode, общий для обоих
// путей через builtinFlags.
func TestGenerateFromIRBuiltinMathCall(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CallExpr{Callee: "корень", Args: []ir.Expr{&ir.FloatLiteral{Value: 16}}, Type: ir.TypeFloat, HasReturn: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "sqrt(16") {
		t.Fatalf("ожидался вызов sqrt, получено:\n%s", out)
	}
	if !strings.Contains(out, "#include <math.h>") {
		t.Fatalf("ожидался #include <math.h>, получено:\n%s", out)
	}
}

// TestGenerateFromIRExternFuncCall - ir.Program.ExternFuncs даёт настоящий
// extern-прототип и вызов по CName, не по mangleName(Alias).
func TestGenerateFromIRExternFuncCall(t *testing.T) {
	prog := &ir.Program{
		ExternFuncs: []*ir.ExternFuncDecl{
			{CName: "sqrt", Alias: "корень2", Params: []ir.Type{ir.TypeFloat}, ReturnType: ir.TypeFloat, HasReturn: true},
		},
		Functions: []*ir.Function{
			{Name: "главная", Body: []ir.Stmt{
				&ir.PrintStmt{Value: &ir.CallExpr{Callee: "корень2", Args: []ir.Expr{&ir.FloatLiteral{Value: 16}}, Type: ir.TypeFloat, HasReturn: true}},
			}},
		},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "extern double sqrt(double);") {
		t.Fatalf("ожидался extern-прототип sqrt, получено:\n%s", out)
	}
	if !strings.Contains(out, "sqrt(16") {
		t.Fatalf("ожидался вызов по настоящему C-имени sqrt, получено:\n%s", out)
	}
	if strings.Contains(out, "корень2") {
		t.Fatalf("Rech-facing имя не должно попасть в C (только CName), получено:\n%s", out)
	}
}

// TestGenerateFromIRFileForEach - "для <строка> в <Файл>" - отдельная,
// потоковая форма от индексированного Список<T>.
func TestGenerateFromIRFileForEach(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "ф", Value: &ir.CallExpr{Callee: "открыть", Args: []ir.Expr{&ir.StringLiteral{Value: "путь"}}, Type: ir.TypeFile, HasReturn: true}},
		&ir.ForEachStmt{
			Var:        "строка",
			Collection: &ir.Identifier{Name: "ф", Type: ir.TypeFile},
			ElemType:   ir.TypeString,
			Body: []ir.Stmt{
				&ir.PrintStmt{Value: &ir.Identifier{Name: "строка", Type: ir.TypeString}},
			},
		},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_open(\"путь\")",
		"for (;;) {",
		"const char *v_stroka = rech_read_line_from_file(v_f, &rech_fe_ok_v_stroka);",
		"if (!rech_fe_ok_v_stroka) { break; }",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRSocketBuiltinCalls(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "слушатель", Value: &ir.CallExpr{Callee: "слушать_tcp", Args: []ir.Expr{&ir.IntLiteral{Value: 8080}}, Type: ir.TypeSocket, HasReturn: true}},
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "отправить", Args: []ir.Expr{&ir.Identifier{Name: "слушатель", Type: ir.TypeSocket}, &ir.StringLiteral{Value: "привет"}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_listen_tcp(8080LL)",
		`rech_send(v_slushatel, "привет")`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRHTTPRequestBuiltinCall - "запрос" (шаг 9) поверх
// встроенных сущностей "Запрос"/"Ответ" - в отличие от остальных builtin'ов
// шага 8, они не резолвятся в AST-пути (cbackend.resolveType не знает про
// них, см. package doc internal/semantic.registerHTTPEntity) - только
// через internal/semantic.Analyze, поэтому здесь строится ir.Program
// вручную, не через AST/Generate.
func TestGenerateFromIRHTTPRequestBuiltinCall(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Запрос", Fields: []ir.Field{
				{Name: "метод", Type: ir.TypeString}, {Name: "адрес", Type: ir.TypeString},
				{Name: "порт", Type: ir.TypeInt}, {Name: "путь", Type: ir.TypeString},
				{Name: "заголовки", Type: ir.TypeString}, {Name: "тело", Type: ir.TypeString},
			}},
			{Name: "Ответ", Fields: []ir.Field{
				{Name: "код", Type: ir.TypeInt}, {Name: "заголовки", Type: ir.TypeString}, {Name: "тело", Type: ir.TypeString},
			}},
		},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "р", Value: &ir.NewExpr{Entity: "Запрос", Args: []ir.Expr{
				&ir.StringLiteral{Value: "GET"}, &ir.StringLiteral{Value: "example.com"},
				&ir.IntLiteral{Value: 80}, &ir.StringLiteral{Value: "/"},
				&ir.StringLiteral{Value: ""}, &ir.StringLiteral{Value: ""},
			}}},
			&ir.VarDecl{Name: "о", Value: &ir.CallExpr{
				Callee: "запрос", Args: []ir.Expr{&ir.Identifier{Name: "р", Type: ir.EntityType("Запрос")}},
				Type: ir.EntityType("Ответ"), HasReturn: true,
			}},
			&ir.PrintStmt{Value: &ir.FieldAccess{
				Receiver: &ir.Identifier{Name: "о", Type: ir.EntityType("Ответ")}, Field: "код", Type: ir.TypeInt,
			}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_http_request(v_r)",
		"static rech_T_otvet rech_http_request(rech_T_zapros req)",
		"} rech_T_zapros;", "} rech_T_otvet;",
		"static char *rech_recv_until_close(rech_socket_t s)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRWholeFileBuiltinCalls(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CallExpr{Callee: "прочитать_файл", Args: []ir.Expr{&ir.StringLiteral{Value: "путь"}}, Type: ir.TypeString, HasReturn: true}},
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "записать_в_файл", Args: []ir.Expr{&ir.StringLiteral{Value: "путь2"}, &ir.StringLiteral{Value: "текст"}}}},
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "дописать_в_файл", Args: []ir.Expr{&ir.StringLiteral{Value: "путь3"}, &ir.StringLiteral{Value: "хвост"}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		`rech_read_whole_file("путь")`,
		`rech_write_file("путь2", "текст");`,
		`rech_append_file("путь3", "хвост");`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRConsoleInputBuiltinCall(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CallExpr{Callee: "прочитать_строку", Type: ir.TypeString, HasReturn: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_read_line()",
		"static char *rech_read_line(void)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRStdStreamsBuiltinCalls(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "записать", Args: []ir.Expr{
			&ir.CallExpr{Callee: "стандартный_вывод", Type: ir.TypeInt, HasReturn: true},
			&ir.StringLiteral{Value: "ок"},
		}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `rech_write(stdout, "ок");`) {
		t.Fatalf("ожидался rech_write(stdout, ...), получено:\n%s", out)
	}
}

func TestGenerateFromIRContainsBuiltinCall(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CallExpr{Callee: "содержит", Args: []ir.Expr{
			&ir.StringLiteral{Value: "привет мир"}, &ir.StringLiteral{Value: "мир"},
		}, Type: ir.TypeBool, HasReturn: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `(strstr("привет мир", "мир") != NULL)`) {
		t.Fatalf("ожидался strstr, получено:\n%s", out)
	}
}

// TestGenerateFromIRArgsBuiltinCalls - "количество_аргументов"/"аргумент" -
// главная всегда получает настоящие argc/argv (не "void"), независимо от
// того, что вызывающая сторона видит только фиксированную сигнатуру без
// параметров на уровне самого Rech (docs/SPEC.md).
func TestGenerateFromIRArgsBuiltinCalls(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CallExpr{Callee: "количество_аргументов", Type: ir.TypeInt, HasReturn: true}},
		&ir.PrintStmt{Value: &ir.CallExpr{Callee: "аргумент", Args: []ir.Expr{&ir.IntLiteral{Value: 0}}, Type: ir.TypeString, HasReturn: true}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"int main(int argc, char **argv) {",
		"rech_init_args(argc, argv);",
		"(rech_argc - 1)",
		"rech_arg(0LL)",
		"static const char *rech_arg(long long i)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRArgsGlobalsAlwaysPresent - rech_argc/rech_argv
// объявлены, даже когда программа вообще не использует "аргумент"/
// "количество_аргументов" - main() всё равно безусловно присваивает их и
// должен видеть объявление раньше присваивания.
func TestGenerateFromIRArgsGlobalsAlwaysPresent(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "static int rech_argc = 0;") {
		t.Fatalf("ожидалось безусловное объявление rech_argc, получено:\n%s", out)
	}
	if strings.Contains(out, "rech_arg(long long i)") {
		t.Fatalf("rech_arg не должен генерироваться, когда \"аргумент\" не используется:\n%s", out)
	}
}

func TestGenerateFromIRShowStmtPrimitive(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.ShowStmt{Value: &ir.IntLiteral{Value: 5}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `printf("%lld (Целое)", 5LL);`) {
		t.Fatalf("ожидался показ Целое, получено:\n%s", out)
	}
}

func TestGenerateFromIRShowStmtEntityGeneratesShowFunc(t *testing.T) {
	ent := &ir.Entity{Name: "Точка", Fields: []ir.Field{
		{Name: "x", Type: ir.TypeInt},
		{Name: "y", Type: ir.TypeInt},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "p", Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.IntLiteral{Value: 1}, &ir.IntLiteral{Value: 2}}}},
		&ir.ShowStmt{Value: &ir.Identifier{Name: "p", Type: ir.EntityType("Точка")}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{ent}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"static void rech_show_tochka(rech_T_tochka v) {",
		`printf("Точка { ");`,
		`printf("x: ");`,
		`printf("%lld (Целое)", v.f_x);`,
		"rech_show_tochka(v_p);",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

func TestGenerateFromIRShowStmtOmittedWhenUnused(t *testing.T) {
	ent := &ir.Entity{Name: "Точка", Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "p", Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.IntLiteral{Value: 1}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{ent}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_show_") {
		t.Fatalf("функции показа не должны генерироваться, если \"показать\" не используется:\n%s", out)
	}
}

func TestGenerateFromIRShowStmtList(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.ShowStmt{Value: &ir.Identifier{Name: "список", Type: ir.EntityType("Список<Целое>")}},
	}}
	prog := &ir.Program{
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Целое>", Template: "Список", ElemType: ir.TypeInt}},
	}
	main.Body = append([]ir.Stmt{&ir.VarDecl{Name: "список", Value: &ir.NewExpr{Entity: "Список<Целое>"}}}, main.Body...)
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"static void rech_show_spisok_of_celoe(rech_T_spisok_of_celoe v) {",
		`printf("Список<Целое> [");`,
		"for (long long i = 0; i < v.len; i++) {",
		`printf("%lld (Целое)", v.data[i]);`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRPrintOnEntityShowsFriendlyPlaceholder - "вывести" на
// сущности не ошибка компиляции и не printf("%s", ...) на C-структуре
// (неопределённое поведение) - печатает маркер "[сущность]<Тип>".
func TestGenerateFromIRPrintOnEntityShowsFriendlyPlaceholder(t *testing.T) {
	ent := &ir.Entity{Name: "Точка", Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "p", Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.IntLiteral{Value: 1}}}},
		&ir.PrintStmt{Value: &ir.Identifier{Name: "p", Type: ir.EntityType("Точка")}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{ent}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `printf("[сущность]<Точка>\n");`) {
		t.Fatalf("ожидался дружелюбный плейсхолдер, получено:\n%s", out)
	}
}
