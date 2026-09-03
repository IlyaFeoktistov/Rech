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
	if !strings.Contains(out, "long long v_a_INIT = 10LL;\n") || !strings.Contains(out, "long long v_a = v_a_INIT;\n") {
		t.Fatalf("ожидалось объявление переменной через временную (см. package doc VarDecl codegen), получено:\n%s", out)
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
	if !strings.Contains(out, "rech_ret_tmp = (v_chto + v_kem);") || !strings.Contains(out, "return rech_ret_tmp;") {
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
		Ctor: &ir.Method{
			Params: []ir.Param{{Name: "возраст", Type: ir.TypeInt}},
			Body: []ir.Stmt{&ir.FieldAssign{
				Receiver: &ir.ThisExpr{Entity: "Пользователь"},
				Field:    "возраст",
				Value:    &ir.Identifier{Name: "возраст", Type: ir.TypeInt},
			}},
		},
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
	if !strings.Contains(out, "rech_T_polzovatel * v_polzovatel_INIT = rech_new_polzovatel(30LL);") {
		t.Fatalf("ожидался вызов GC-конструктора (сущность - ссылочный тип по умолчанию), получено:\n%s", out)
	}
	if !strings.Contains(out, "static rech_T_polzovatel *rech_new_polzovatel(long long v_vozrast) {") ||
		!strings.Contains(out, "rech_T_polzovatel *p = GC_MALLOC(sizeof(rech_T_polzovatel));") {
		t.Fatalf("ожидалось определение GC-конструктора, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_polzovatel_sostaritsya((v_polzovatel));") {
		t.Fatalf("ожидался вызов метода с получателем-указателем как есть (без \"&\" - сущность уже ссылка), получено:\n%s", out)
	}
	if !strings.Contains(out, "(this_)->f_vozrast = ((this_)->f_vozrast + 1LL);") {
		t.Fatalf("ожидалась мутация поля через \"этот\" (this_ - уже указатель), получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_ret_tmp = (this_)->f_vozrast;") || !strings.Contains(out, "return rech_ret_tmp;") {
		t.Fatalf("ожидался возврат поля через \"этот\", получено:\n%s", out)
	}
}

// TestGenerateFromIRStructFieldsConstructorMethodsAndThis - как
// TestGenerateFromIREntityFieldsConstructorMethodsAndThis выше, но
// IsStruct: true ("структура", не "сущность") - продолжает старую модель
// "по значению" один-в-один: compound literal вместо GC-конструктора,
// "&" при передаче получателя метода вместо готового указателя. Получатель
// самого метода ("этот") - указатель в обоих случаях (общее исключение из
// копирования, не меняется этой фичей вообще), поэтому мутация поля и
// возврат через "этот" совпадают с ref-версией теста выше.
func TestGenerateFromIRStructFieldsConstructorMethodsAndThis(t *testing.T) {
	entity := &ir.Entity{
		Name:     "Пользователь",
		IsStruct: true,
		Fields:   []ir.Field{{Name: "возраст", Type: ir.TypeInt}},
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
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "пользователь", Value: &ir.NewExpr{Entity: "Пользователь", Args: []ir.Expr{&ir.IntLiteral{Value: 30}}}},
		&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{
			Receiver: &ir.Identifier{Name: "пользователь", Type: ir.EntityType("Пользователь")}, Entity: "Пользователь", Method: "состариться",
		}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_T_polzovatel v_polzovatel_INIT = (rech_T_polzovatel){.f_vozrast = 30LL};") {
		t.Fatalf("ожидался конструктор-значение (compound literal), получено:\n%s", out)
	}
	if strings.Contains(out, "rech_new_polzovatel") {
		t.Fatalf("структура не должна получать GC-конструктор, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_polzovatel_sostaritsya(&(v_polzovatel));") {
		t.Fatalf("ожидался вызов метода с адресом получателя (структура - значение, не указатель), получено:\n%s", out)
	}
	if !strings.Contains(out, "((*this_)).f_vozrast = (((*this_)).f_vozrast + 1LL);") {
		t.Fatalf("ожидалась мутация поля через \"этот\" (получатель метода - всегда указатель), получено:\n%s", out)
	}
}

// TestGenerateFromIRBecomeStmtSetsStateFlagsBit - "становится" устанавливает
// бит СОБСТВЕННОГО состояния сущности в скрытом рантайм-поле "state_flags"
// (StateBit - индекс в StateNames + StateBitOffset, здесь оба 0) - читает
// его ir.StateCheckExpr там, где established не смог доказать переход на
// этапе компиляции (internal/backend/cbackend/typestate.go).
func TestGenerateFromIRBecomeStmtSetsStateFlagsBit(t *testing.T) {
	entity := &ir.Entity{
		Name:          "Пользователь",
		Fields:        []ir.Field{{Name: "имя", Type: ir.TypeString}},
		StateNames:    []string{"авторизован"},
		HasStateFlags: true,
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
	if !strings.Contains(out, "this_->state_flags |= 1ULL << 0;") {
		t.Fatalf("ожидалась установка бита состояния, получено:\n%s", out)
	}
	if !strings.Contains(out, "uint64_t state_flags;") {
		t.Fatalf("ожидалось скрытое поле \"state_flags\" в C-структуре, получено:\n%s", out)
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

// TestGenerateFromIROmitsGCWithoutEntities - libgc не входит в стандартный
// C-тулчейн ни на одной платформе (в отличие от -lm) - программа без
// единой ссылочной сущности не должна требовать её установки, поэтому
// "#include <gc.h>"/"GC_INIT()" эмитятся только при ctx.usesGC (см.
// internal/toolchain.CC - решает про "-lgc" по этому же признаку в файле).
func TestGenerateFromIROmitsGCWithoutEntities(t *testing.T) {
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{mainFuncIR()}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "gc.h") || strings.Contains(out, "GC_INIT") {
		t.Fatalf("не ожидался Boehm GC в программе без сущностей, получено:\n%s", out)
	}
}

func TestGenerateFromIRIncludesGCForRefEntity(t *testing.T) {
	entity := &ir.Entity{Name: "Точка", Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "p", Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.IntLiteral{Value: 1}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "#include <gc.h>") || !strings.Contains(out, "GC_INIT();") {
		t.Fatalf("ожидался Boehm GC для программы со ссылочной сущностью, получено:\n%s", out)
	}
}

func TestGenerateFromIROmitsGCForStructOnlyProgram(t *testing.T) {
	entity := &ir.Entity{Name: "Точка", IsStruct: true, Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "p", Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.IntLiteral{Value: 1}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "gc.h") || strings.Contains(out, "GC_INIT") {
		t.Fatalf("не ожидался Boehm GC для программы только со \"структура\", получено:\n%s", out)
	}
}

// TestGenerateFromIRCharFieldWithoutShowDoesNotPullInCastHelpers -
// регрессия: сущность с полем типа "Символ" не должна включать
// #include <string.h>/<stdlib.h> и рантайм-хелперы кастов, если "показать"
// нигде в программе реально не встречается - раньше showFuncs строился
// для КАЖДОЙ сущности заранее, "на всякий случай" (до того как
// ctx.usesShow становился известен), и это взводило ctx.usesCast как
// побочный эффект даже для полностью отброшенного, никогда не
// эмитируемого текста - на практике эти лишние заголовки конфликтовали с
// настоящими объявлениями функций стандартной библиотеки, объявленных
// через "внешняя функция" (см. docs/SPEC.md, раздел 12).
func TestGenerateFromIRCharFieldWithoutShowDoesNotPullInCastHelpers(t *testing.T) {
	entity := &ir.Entity{Name: "Т", Fields: []ir.Field{{Name: "с", Type: ir.TypeChar}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "p", Value: &ir.NewExpr{Entity: "Т", Args: []ir.Expr{&ir.CharLiteral{Value: 'я'}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "#include <string.h>") || strings.Contains(out, "#include <stdlib.h>") {
		t.Fatalf("не ожидались string.h/stdlib.h без единого \"показать\" в программе, получено:\n%s", out)
	}
	if strings.Contains(out, "rech_cast_") {
		t.Fatalf("не ожидались рантайм-хелперы кастов без единого \"показать\" в программе, получено:\n%s", out)
	}
}

// TestGenerateFromIRShowCharFieldDefinesPanicHelper - регрессия: "показать"
// сущности с полем типа "Символ" использует castHelpers (rech_cast_
// char_to_str), а несколько других функций в том же общем текстовом блоке
// (rech_cast_str_to_int и т. п.) вызывают rech_panic - если panicHelper не
// эмитирован в файл вместе с castHelpers, сборка линкуется с ошибкой
// "undefined reference to rech_panic", даже когда сам "показать" ни разу
// не парсит строку.
func TestGenerateFromIRShowCharFieldDefinesPanicHelper(t *testing.T) {
	entity := &ir.Entity{Name: "Т", Fields: []ir.Field{{Name: "с", Type: ir.TypeChar}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.NewExpr{Entity: "Т", Args: []ir.Expr{&ir.CharLiteral{Value: 'я'}}}},
		&ir.ShowStmt{Value: &ir.Identifier{Name: "т", Type: ir.EntityType("Т")}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{entity}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_cast_char_to_str") {
		t.Fatalf("ожидался rech_cast_char_to_str для показа поля \"Символ\", получено:\n%s", out)
	}
	if !strings.Contains(out, "void rech_panic(") && !strings.Contains(out, "static void rech_panic(") {
		t.Fatalf("ожидалось определение rech_panic (panicHelper) вместе с castHelpers, получено:\n%s", out)
	}
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
	if !strings.Contains(out, "rech_M_spisok_of_celoe_dobavit((v_chisla), 5LL);") {
		t.Fatalf("ожидался вызов добавить, получено:\n%s", out)
	}
	if !strings.Contains(out, "for (long long rech_fe_i_v_x = 0; rech_fe_i_v_x < v_chisla->len; rech_fe_i_v_x++) {") {
		t.Fatalf("ожидался цикл по данным Списка, получено:\n%s", out)
	}
	if !strings.Contains(out, "long long v_x = v_chisla->data[rech_fe_i_v_x];") {
		t.Fatalf("ожидалось присваивание элемента переменной цикла, получено:\n%s", out)
	}
}

// TestGenerateFromIRGenericCollectionIsReferenceTypeNotValueType - главная
// находка Шага 28 (docs/ROADMAP.md): Список/Словарь/Стопка/Очередь раньше
// были представлены В C ПО ЗНАЧЕНИЮ (копия структуры-обёртки при передаче
// параметром/присваивании, хотя внутренний буфер-указатель оставался общим
// - после resize видна только та копия, где он произошёл) - расходилось с
// interp (Go-карта/срез - тот же объект при копировании). Функция,
// принимающая "Словарь<Строка, Строка>" параметром, должна получать
// ПОЛНОЦЕННЫЙ УКАЗАТЕЛЬ (entityIsRef), не структуру по значению - иначе
// мутация внутри функции невидима вызывающей стороне после resize.
func TestGenerateFromIRGenericCollectionIsReferenceTypeNotValueType(t *testing.T) {
	dictType := "Словарь<Строка, Строка>"
	fn := &ir.Function{
		Name:   "вставить_в",
		Params: []ir.Param{{Name: "д", Type: ir.EntityType(dictType)}},
		Body: []ir.Stmt{
			&ir.ExprStmt{MethodCall: &ir.MethodCallExpr{
				Receiver: &ir.Identifier{Name: "д", Type: ir.EntityType(dictType)},
				Entity:   dictType, Method: "установить",
				Args: []ir.Expr{&ir.StringLiteral{Value: "а"}, &ir.StringLiteral{Value: "1"}},
			}},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "д", Value: &ir.NewExpr{Entity: dictType}},
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "вставить_в", Args: []ir.Expr{&ir.Identifier{Name: "д", Type: ir.EntityType(dictType)}}}},
	}}
	prog := &ir.Program{
		Functions: []*ir.Function{fn, main},
		Generics: []ir.GenericInstantiation{
			{Name: dictType, Template: "Словарь", KeyType: ir.TypeString, ElemType: ir.TypeString},
		},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	structName := mangleType(dictType)
	// Параметр функции - указатель, не значение структуры.
	if !strings.Contains(out, "rech_vstavit_v("+structName+" * v_d)") {
		t.Fatalf("ожидался указательный параметр %s *, получено:\n%s", structName, out)
	}
	// Конструктор возвращает указатель, переменная в "главная" - тоже указатель.
	if !strings.Contains(out, structName+" * v_d_INIT = "+mangleCtor(dictType)+"();") {
		t.Fatalf("ожидался указательный конструктор, получено:\n%s", out)
	}
	// Аргумент вызова - сама переменная (указатель), без "&" - её уже
	// незачем адресовать, она и так указатель.
	if !strings.Contains(out, "rech_vstavit_v(v_d);") {
		t.Fatalf("ожидался вызов без \"&\" (значение уже указатель), получено:\n%s", out)
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
		&ir.VarDecl{Name: "слушатель", Value: &ir.CallExpr{Callee: "сетевой_узел", Args: []ir.Expr{&ir.StringLiteral{Value: "tcp"}, &ir.StringLiteral{Value: ""}, &ir.IntLiteral{Value: 8080}}, Type: ir.TypeSocket, HasReturn: true}},
		&ir.ExprStmt{Call: &ir.CallExpr{Callee: "отправить", Args: []ir.Expr{&ir.Identifier{Name: "слушатель", Type: ir.TypeSocket}, &ir.StringLiteral{Value: "привет"}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		`rech_network_node("tcp", "", 8080LL)`,
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
		"static void rech_show_tochka(rech_T_tochka * v) {",
		`printf("Точка { ");`,
		`printf("x: ");`,
		`printf("%lld (Целое)", v->f_x);`,
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
		"static void rech_show_spisok_of_celoe(rech_T_spisok_of_celoe *v) {",
		`printf("Список<Целое> [");`,
		"for (long long i = 0; i < v->len; i++) {",
		`printf("%lld (Целое)", v->data[i]);`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRShowEntityWithGenericFieldOrdersGenericShowFuncFirst -
// сущность с полем-generic-коллекцией ("участники: Список<Строка>") -
// её функция показа зовёт rech_show_<коллекция> - раньше показ-функции
// сущностей эмитировались ВСЕ перед показ-функциями generic-коллекций
// безусловно (порядок в исходном тексте не совпадал с порядком реальной
// зависимости), из-за чего C не компилировался ("implicit declaration"
// -> "conflicting types", т. к. сама реализация уже static) - найдено при
// проверке Шага 28 (Список/Словарь как ссылка) на сущности с полем-
// коллекцией.
func TestGenerateFromIRShowEntityWithGenericFieldOrdersGenericShowFuncFirst(t *testing.T) {
	ent := &ir.Entity{Name: "Группа", Fields: []ir.Field{
		{Name: "участники", Type: ir.EntityType("Список<Строка>")},
	}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "г", Value: &ir.NewExpr{Entity: "Группа"}},
		&ir.ShowStmt{Value: &ir.Identifier{Name: "г", Type: ir.EntityType("Группа")}},
	}}
	prog := &ir.Program{
		Entities:  []*ir.Entity{ent},
		Functions: []*ir.Function{main},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Строка>", Template: "Список", ElemType: ir.TypeString}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	genericShowIdx := strings.Index(out, "static void rech_show_spisok_of_stroka(")
	entityShowIdx := strings.Index(out, "static void rech_show_gruppa(")
	if genericShowIdx == -1 || entityShowIdx == -1 || genericShowIdx > entityShowIdx {
		t.Fatalf("функция показа generic-коллекции должна идти раньше функции показа сущности, несущей её полем, получено:\n%s", out)
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

// TestGenerateFromIRDirectNumericCastIsPlainCCast - приведения без участия
// "Строка" (числа/Символ/Байт между собой) - обычный C-каст "((тип)(...))",
// без рантайм-хелперов (см. internal/backend/cbackend/cast.go).
func TestGenerateFromIRDirectNumericCastIsPlainCCast(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "а", Value: &ir.CastExpr{Value: &ir.IntLiteral{Value: 97}, To: ir.TypeChar}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "((int32_t)(97LL))") {
		t.Fatalf("ожидался прямой C-каст в int32_t, получено:\n%s", out)
	}
	if strings.Contains(out, "rech_cast_") {
		t.Fatalf("прямой числовой каст не должен использовать рантайм-хелперы: %s", out)
	}
}

// TestGenerateFromIRStringCastUsesRuntimeHelper - каст с участием "Строка"
// (в обе стороны) - через рантайм-хелпер (castHelpers), не обычный C-каст,
// потому что нужен разбор/форматирование, а не переинтерпретация битов.
func TestGenerateFromIRStringCastUsesRuntimeHelper(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CastExpr{Value: &ir.StringLiteral{Value: "42"}, To: ir.TypeInt}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `rech_cast_str_to_int("42", "Целое")`) {
		t.Fatalf("ожидался вызов rech_cast_str_to_int, получено:\n%s", out)
	}
	if !strings.Contains(out, "static long long rech_cast_str_to_int(") {
		t.Fatalf("ожидалось определение rech_cast_str_to_int (castHelpers), получено:\n%s", out)
	}
}

func TestGenerateFromIRCharLiteral(t *testing.T) {
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.PrintStmt{Value: &ir.CharLiteral{Value: 'я'}},
	}}
	out, err := GenerateFromIR(&ir.Program{Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "1103") {
		t.Fatalf("ожидался код точки 'я' (1103) в выводе, получено:\n%s", out)
	}
}

// TestGenerateFromIREntityWithGenericFieldOrdering - найденный вручную баг
// (не через тест): сущность с полем встроенной generic-коллекции по
// значению (например, "товары: Список<Строка>") требует, чтобы C-typedef
// коллекции (genericsCode) был объявлен РАНЬШЕ typedef самой сущности
// (typedefs) - иначе "unknown type name" в сгенерированном C (компилятор
// видит тип поля раньше его typedef). Элементы встроенных коллекций -
// только примитивы, значит обратная зависимость (коллекция несёт сущность
// как элемент) невозможна и порядок genericsCode-до-typedefs всегда
// безопасен.
func TestGenerateFromIREntityWithGenericFieldOrdering(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Корзина", Fields: []ir.Field{{Name: "товары", Type: ir.EntityType("Список<Строка>")}}},
		},
		Generics: []ir.GenericInstantiation{{Name: "Список<Строка>", Template: "Список", ElemType: ir.TypeString}},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "т", Value: &ir.NewExpr{Entity: "Список<Строка>"}},
			&ir.VarDecl{Name: "к", Value: &ir.NewExpr{Entity: "Корзина", Args: []ir.Expr{&ir.Identifier{Name: "т", Type: ir.EntityType("Список<Строка>")}}}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	listDefPos := strings.Index(out, "} "+mangleType("Список<Строка>")+";")
	entityDefPos := strings.Index(out, "} "+mangleType("Корзина")+";")
	if listDefPos < 0 || entityDefPos < 0 {
		t.Fatalf("не найдены оба typedef в выводе:\n%s", out)
	}
	if listDefPos > entityDefPos {
		t.Fatalf("typedef Список<Строка> должен идти раньше typedef Корзина, получено:\n%s", out)
	}
}

// TestGenerateFromIRCollectionHoldingCallableOrdersCallableTypedefFirst -
// коллекция, несущая "Вызываемый" элементом ПО ЗНАЧЕНИЮ ("Список<Вызываемый
// (Целое): Целое>" - например, таблица маршрутов HTTP-сервера), должна
// получить C-typedef самой сигнатуры (rech_C_...) РАНЬШЕ typedef коллекции -
// иначе C-структура коллекции ссылается на ещё не объявленный тип
// (см. package doc ir_generator.go, genericsCodeAfterCallables).
// TestGenerateFromIRStringLiteralEscapesCarriageReturn - строковый литерал
// со значением "\r" (уже разобранным лексером в настоящий байт CR, см.
// internal/lexer) должен попасть в C-исходник как escape-последовательность
// "\r", а не как сырой байт 0x0D - сырой CR внутри C-строкового литерала
// обрывает строку и ломает компиляцию сгенерированного .c ("missing
// terminating \" character") - найдено при добавлении "\r" как
// escape-последовательности в сам лексер Rech (escapeCString не знала о
// '\r', хотя знала про '\n'/'\t').
func TestGenerateFromIRStringLiteralEscapesCarriageReturn(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.PrintStmt{Value: &ir.StringLiteral{Value: "a\r\nb"}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.ContainsRune(out, '\r') {
		t.Fatalf("сырой байт CR попал в C-исходник, должен быть экранирован как \\r:\n%s", out)
	}
	if !strings.Contains(out, `"a\r\nb"`) {
		t.Fatalf("ожидалась экранированная строка \"a\\r\\nb\", получено:\n%s", out)
	}
}

// TestGenerateFromIRFunctionWithDBParamOrdersDBStructFirst - функция,
// принимающая параметр типа "БД" (ir.TypeDB, "rech_db_t *" в C), должна
// получить свой ПРОТОТИП только после typedef самой "rech_db_t" -
// dbRuntimeCode() (единственное место, где этот typedef определяется)
// раньше эмитировалась ПОСЛЕ блока прототипов функций, из-за чего прототип
// любой функции с параметром/возвратом типа "БД" не компилировался
// ("unknown type name 'rech_db_t'") - тот же класс typedef-ordering бага,
// что и у Callable-в-generic-коллекции выше, найдено при написании
// examples/проекты/http_сервер (middleware/обработчики, принимающие "БД"
// аргументом).
func TestGenerateFromIRFunctionWithDBParamOrdersDBStructFirst(t *testing.T) {
	prog := &ir.Program{
		UsesDB: true,
		Functions: []*ir.Function{
			{Name: "закрыть", Params: []ir.Param{{Name: "бд", Type: ir.TypeDB}}},
			{Name: "главная", Body: []ir.Stmt{
				&ir.VarDecl{Name: "бд", Value: &ir.CallExpr{Callee: "подключиться_к_бд", Args: []ir.Expr{&ir.StringLiteral{Value: "sqlite::memory:"}}, Type: ir.TypeDB, HasReturn: true}},
			}},
		},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	structDefPos := strings.Index(out, "} rech_db_t;")
	protoPos := strings.Index(out, "rech_zakryt(rech_db_t")
	if structDefPos < 0 || protoPos < 0 {
		t.Fatalf("не найдены и typedef, и прототип функции с параметром \"БД\" в выводе:\n%s", out)
	}
	if structDefPos > protoPos {
		t.Fatalf("typedef \"rech_db_t\" должен идти раньше прототипа функции, использующей его, получено:\n%s", out)
	}
}

func TestGenerateFromIRCollectionHoldingCallableOrdersCallableTypedefFirst(t *testing.T) {
	result := ir.TypeInt
	sig := ir.CallableSig{Name: ir.CallableType([]ir.Type{ir.TypeInt}, &result).Entity, Params: []ir.Type{ir.TypeInt}, Result: &result}
	elemType := ir.CallableType([]ir.Type{ir.TypeInt}, &result)
	prog := &ir.Program{
		Callables: []ir.CallableSig{sig},
		Generics:  []ir.GenericInstantiation{{Name: "Список<Вызываемый(Целое)Целое>", Template: "Список", ElemType: elemType}},
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "м", Value: &ir.NewExpr{Entity: "Список<Вызываемый(Целое)Целое>"}},
		}}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	callableDefPos := strings.Index(out, "} "+mangleCallableType(sig.Name)+";")
	listDefPos := strings.Index(out, "} "+mangleType("Список<Вызываемый(Целое)Целое>")+";")
	if callableDefPos < 0 || listDefPos < 0 {
		t.Fatalf("не найдены оба typedef в выводе:\n%s", out)
	}
	if callableDefPos > listDefPos {
		t.Fatalf("typedef \"Вызываемый\" должен идти раньше typedef коллекции, получено:\n%s", out)
	}
}
