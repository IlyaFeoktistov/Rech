package interfacestub

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateInterfaceStubMethodAndCtor - экспортированная сущность с
// экспортированным конструктором и методом - .rechi несёт непрозрачный
// дескриптор, "внешняя функция" на настоящие C-символы и автоген
// обёрточную сущность с ТЕМ ЖЕ именем/методом, вызывающую их (docs/
// SPEC.md, "Закрытые (бинарные) пакеты").
func TestGenerateInterfaceStubMethodAndCtor(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{
				Name:     "Точка",
				Exported: true,
				Fields:   []ir.Field{{Name: "x", Type: ir.TypeInt}},
				Ctor: &ir.Method{
					Exported: true,
					Params:   []ir.Param{{Name: "x", Type: ir.TypeInt}},
					Body:     []ir.Stmt{&ir.FieldAssign{Receiver: &ir.ThisExpr{Entity: "Точка"}, Field: "x", Value: &ir.Identifier{Name: "x", Type: ir.TypeInt}}},
				},
				Methods: []*ir.Method{
					{Name: "координата", Exported: true, ReturnType: ir.TypeInt, HasReturn: true,
						Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.FieldAccess{Receiver: &ir.ThisExpr{Entity: "Точка"}, Field: "x", Type: ir.TypeInt}}}},
				},
			},
		},
	}
	stub, err := Generate(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"внешняя сущность Точка как ТочкаДескриптор",
		"внешняя функция Точка_новый(x: Целое): ТочкаДескриптор",
		"внешняя функция Точка_координата(получатель: ТочкаДескриптор): Целое",
		"сущность Точка",
		"приватный дескриптор: ТочкаДескриптор",
		"конструктор(x: Целое)",
		"этот.дескриптор = Точка_новый(x)",
		"публичный координата(): Целое",
		"вернуть Точка_координата(этот.дескриптор)",
	} {
		if !strings.Contains(stub, want) {
			t.Fatalf("ожидалась подстрока %q в .rechi, получено:\n%s", want, stub)
		}
	}
}

// TestGenerateInterfaceStubSkipsNonExported - неэкспортированные функция/
// сущность/метод не попадают в .rechi вовсе.
func TestGenerateInterfaceStubSkipsNonExported(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "приватная", Exported: false}},
		Entities: []*ir.Entity{
			{Name: "Внутренняя", Exported: false, Ctor: &ir.Method{}},
		},
	}
	stub, err := Generate(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(stub, "приватная") || strings.Contains(stub, "Внутренняя") {
		t.Fatalf("неэкспортированные объявления не должны попадать в .rechi, получено:\n%s", stub)
	}
}

// TestGenerateInterfaceStubEntityWithoutExportedMembersOnlyDescriptor -
// экспортированная сущность без единого экспортированного метода/
// конструктора - только непрозрачный дескриптор, без пустой обёртки
// (docs/SPEC.md, границы среза).
func TestGenerateInterfaceStubEntityWithoutExportedMembersOnlyDescriptor(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Точка", Exported: true, Ctor: &ir.Method{}},
		},
	}
	stub, err := Generate(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(stub, "внешняя сущность Точка как ТочкаДескриптор") {
		t.Fatalf("ожидался непрозрачный дескриптор, получено:\n%s", stub)
	}
	if strings.Contains(stub, "сущность Точка\n{") {
		t.Fatalf("не ожидалась пустая автоген обёртка без членов, получено:\n%s", stub)
	}
}
