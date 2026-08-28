package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestGenerateLibFromIRDoesNotRequireEntryPoint(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "сумма", Exported: true, Params: []ir.Param{{Name: "а", Type: ir.TypeInt}, {Name: "б", Type: ir.TypeInt}},
			ReturnType: ir.TypeInt, HasReturn: true,
			Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "+", Left: &ir.Identifier{Name: "а", Type: ir.TypeInt}, Right: &ir.Identifier{Name: "б", Type: ir.TypeInt}, Type: ir.TypeInt}}},
		},
	}}
	cSource, header, err := GenerateLibFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(cSource, "int main(") {
		t.Fatalf("библиотека без \"главная\" не должна получать main(), получено:\n%s", cSource)
	}
	if !strings.Contains(header, "long long rech_summa(long long v_a, long long v_b);") {
		t.Fatalf("ожидался прототип rech_summa в заголовке, получено:\n%s", header)
	}
}

func TestGenerateLibFromIRHidesNonExportedFunction(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "публичная", Exported: true, Body: nil},
		{Name: "приватная", Exported: false, Body: nil},
	}}
	cSource, header, err := GenerateLibFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(cSource, `__attribute__((visibility("default"))) int rech_publichnaya(void) {`) {
		t.Fatalf("экспортированная функция должна получить атрибут видимости, получено:\n%s", cSource)
	}
	if !strings.Contains(cSource, "\nint rech_privatnaya(void) {") {
		t.Fatalf("неэкспортированная функция не должна получать атрибут видимости, получено:\n%s", cSource)
	}
	if strings.Contains(header, "rech_privatnaya") {
		t.Fatalf("неэкспортированная функция не должна попадать в заголовок, получено:\n%s", header)
	}
}

func TestGenerateFromIRProgramModeOmitsVisibilityAttr(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Exported: false, Body: nil},
	}}
	cSource, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(cSource, "__attribute__") {
		t.Fatalf("обычная программа (не библиотека) не должна получать атрибуты видимости, получено:\n%s", cSource)
	}
}

func TestGenerateHeaderOpaqueEntityTypedef(t *testing.T) {
	prog := &ir.Program{
		Entities: []*ir.Entity{
			{Name: "Точка", Exported: true, Fields: []ir.Field{{Name: "x", Type: ir.TypeInt}}},
		},
		Functions: []*ir.Function{
			{Name: "новая_точка", Exported: true, Params: []ir.Param{{Name: "x", Type: ir.TypeInt}},
				ReturnType: ir.EntityType("Точка"), HasReturn: true,
				Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.NewExpr{Entity: "Точка", Args: []ir.Expr{&ir.Identifier{Name: "x", Type: ir.TypeInt}}}}},
			},
		},
	}
	_, header, err := GenerateLibFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(header, "typedef struct rech_T_tochka rech_T_tochka;") {
		t.Fatalf("ожидался непрозрачный typedef сущности, получено:\n%s", header)
	}
	if !strings.Contains(header, "rech_T_tochka * rech_novaya_tochka(long long v_x);") {
		t.Fatalf("ожидался прототип, возвращающий указатель на сущность, получено:\n%s", header)
	}
}
