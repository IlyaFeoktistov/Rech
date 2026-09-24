package cbackend

import (
	"fmt"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// exceptionEntity - минимальный "Исключение", как его засеивает
// internal/semantic (см. package doc internal/semantic/exception.go) -
// тесты этого файла строят ir.Program вручную, в обход semantic.Analyze,
// поэтому заводят её сами, тем же принципом, что и остальные тесты пакета
// (vtable_test.go).
func exceptionEntity() *ir.Entity {
	return &ir.Entity{Name: systemExceptionEntity, Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}}
}

// TestGenerateFromIRTryCatchEmitsSetjmpLongjmp - "попытаться"/"выбросить" с
// пользовательским наследником "Исключение" - должны появиться кадр,
// setjmp/rech_throw и настоящий рантайм-хелпер rech_throw.
func TestGenerateFromIRTryCatchEmitsSetjmpLongjmp(t *testing.T) {
	custom := &ir.Entity{Name: "ОшибкаДеления", BaseName: systemExceptionEntity}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.TryStmt{
			Body: []ir.Stmt{
				&ir.ThrowStmt{Value: &ir.NewExpr{Entity: "ОшибкаДеления", Args: []ir.Expr{&ir.StringLiteral{Value: "ноль"}}}},
			},
			CatchVar: "иск", CatchTypes: []string{"ОшибкаДеления"}, CatchVarType: ir.EntityType("ОшибкаДеления"),
			CatchBody: []ir.Stmt{
				&ir.PrintStmt{Value: &ir.FieldAccess{Receiver: &ir.Identifier{Name: "иск", Type: ir.EntityType("ОшибкаДеления")}, Field: "сообщение", Type: ir.TypeString}},
			},
		},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{exceptionEntity(), custom}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{"#include <setjmp.h>", "setjmp(rech_frame_1.buf)", "rech_throw(", "static void rech_throw(void *exc, const char *type_name) {", "rech_isa["} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRIsExprUnrelatedToExceptionsStillGetsIsaTable -
// "относится к" - общая проверка, не только для исключений: программа без
// единой "Исключение" в наследовании (только интерфейс) должна получить
// свою собственную рабочую таблицу rech_isa - регрессия на баг, где
// rech_isa эмитилась только при exceptionsUseDispatch() (наследник
// "Исключение" где-то в программе), а не при реальном участнике вообще.
func TestGenerateFromIRIsExprUnrelatedToExceptionsStillGetsIsaTable(t *testing.T) {
	iface := &ir.Interface{Name: "Звучащий", Methods: []ir.InterfaceMethod{
		{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true},
	}}
	cat := &ir.Entity{Name: "Кошка", Interfaces: []string{"Звучащий"}, Methods: []*ir.Method{
		{Name: "издать_звук", ReturnType: ir.TypeString, HasReturn: true, Body: []ir.Stmt{
			&ir.ReturnStmt{Value: &ir.StringLiteral{Value: "мяу"}},
		}},
	}}
	fn := &ir.Function{
		Name: "проверить", Params: []ir.Param{{Name: "з", Type: ir.InterfaceType("Звучащий")}},
		Body: []ir.Stmt{
			&ir.IfStmt{
				Cond: &ir.IsExpr{Value: &ir.Identifier{Name: "з", Type: ir.InterfaceType("Звучащий")}, Target: "Кошка"},
				Then: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "кошка"}}},
			},
		},
	}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{cat}, Interfaces: []*ir.Interface{iface}, Functions: []*ir.Function{fn, main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_isa[") {
		t.Fatalf("ожидалась таблица rech_isa (относится к без единого исключения в программе), получено:\n%s", out)
	}
}

// TestGenerateFromIRIsExprOnNonParticipantIsCompileTimeConstant - операнд
// статического типа вне какой-либо иерархии/интерфейса не может нести
// vt/type_id - результат уже известен на этапе компиляции (семантический
// анализ уже проверил единственно возможное отношение - совпадение типов),
// без обращения к рантайму вовсе.
func TestGenerateFromIRIsExprOnNonParticipantIsCompileTimeConstant(t *testing.T) {
	point := &ir.Entity{Name: "Точка", Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}}}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "т", Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.IntLiteral{Value: 1}}}},
		&ir.IfStmt{
			Cond: &ir.IsExpr{Value: &ir.Identifier{Name: "т", Type: ir.EntityType("Точка")}, Target: "Точка"},
			Then: []ir.Stmt{&ir.PrintStmt{Value: &ir.StringLiteral{Value: "да"}}},
		},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{point}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "if (true) {") {
		t.Fatalf("ожидалась константа true вместо рантайм-проверки, получено:\n%s", out)
	}
}

// TestGenerateFromIRVtableFieldEmittedForEntityWithoutMethods - регрессия:
// сущность, участвующая в наследовании, но без единого метода (как
// "Исключение" и её маркерные наследники) всё равно должна получить поле
// vt И типы rech_vtable/rech_vtable_header - раньше usesVtable зависел от
// количества СЛОТОВ (len(slots) > 0), а не от количества УЧАСТНИКОВ,
// поэтому такая программа не компилировалась (rech_vtable использовалась в
// структуре, но типа не существовало).
func TestGenerateFromIRVtableFieldEmittedForEntityWithoutMethods(t *testing.T) {
	base := &ir.Entity{Name: systemExceptionEntity, Fields: []ir.Field{{Name: "сообщение", Type: ir.TypeString}}}
	derived := &ir.Entity{Name: "ОшибкаДеления", BaseName: systemExceptionEntity}
	main := &ir.Function{Name: "главная", Body: []ir.Stmt{
		&ir.VarDecl{Name: "и", Value: &ir.NewExpr{Entity: "ОшибкаДеления", Args: []ir.Expr{&ir.StringLiteral{Value: "х"}}}},
	}}
	out, err := GenerateFromIR(&ir.Program{Entities: []*ir.Entity{base, derived}, Functions: []*ir.Function{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "typedef struct { int type_id; rech_vtfunc slots[0]; } rech_vtable;") {
		t.Fatalf("ожидался typedef rech_vtable (даже с пустым slots), получено:\n%s", out)
	}
	if !strings.Contains(out, "const rech_vtable *vt;") {
		t.Fatalf("ожидалось поле vt у участвующей в наследовании сущности без методов, получено:\n%s", out)
	}
}

// TestGeneratePanicIsThrownObject - паника рантайма строит объект
// встроенного типа (rech_panic_as) и бросается тем же rech_throw, что и
// "выбросить"; "поймать(о: ВыходЗаГраницы)" сопоставляет тип по rech_isa;
// прототип rech_panic_as - раньше определения.
func TestGeneratePanicIsThrownObject(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    Список<Целое> сп = []
    попытаться {
        вывести сп.получить(1)
    } поймать(о: ВыходЗаГраницы) {
        вывести о
    }
}
`)
	protoIdx := strings.Index(out, "static void rech_panic_as(int kind, const char *msg);")
	defIdx := strings.Index(out, "static void rech_panic_as(int kind, const char *msg) {")
	if protoIdx < 0 || defIdx < protoIdx {
		t.Fatalf("ожидался прототип rech_panic_as до определения, получено:\n%s", out)
	}
	for _, want := range []string{
		fmt.Sprintf("rech_panic_as(%d, ", rtmsg.PanicKind(rtmsg.PanicOutOfRange)),
		fmt.Sprintf("case %d: rech_throw(", rtmsg.PanicKind(rtmsg.PanicOutOfRange)),
		"rech_isa[((const rech_vtable_header *)(rech_frame_1_exc))->vt->type_id]",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	if strings.Contains(out, "catch_panics") {
		t.Fatalf("отдельного канала паник больше нет, получено:\n%s", out)
	}
}
