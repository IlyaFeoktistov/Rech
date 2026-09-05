package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestGenerateHeaderPropagatesDocComment - Doc экспортированной функции
// (уже очищенный normalizeDoc текст, без "/**"/"*/") попадает в .h как
// настоящий C doc-комментарий (cDocComment) - потребитель библиотеки видит
// документацию автора, не только голую сигнатуру.
func TestGenerateHeaderPropagatesDocComment(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "сумма", Exported: true, Doc: "Складывает два числа.\nВторая строка.",
			Params: []ir.Param{{Name: "а", Type: ir.TypeInt}, {Name: "б", Type: ir.TypeInt}},
			ReturnType: ir.TypeInt, HasReturn: true,
			Body: []ir.Stmt{&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "+", Left: &ir.Identifier{Name: "а", Type: ir.TypeInt}, Right: &ir.Identifier{Name: "б", Type: ir.TypeInt}, Type: ir.TypeInt}}},
		},
	}}
	header, err := generateHeader(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := "/**\n * Складывает два числа.\n * Вторая строка.\n */\nlong long сумма(long long v_a, long long v_b);"
	if !strings.Contains(header, want) {
		t.Fatalf("ожидался doc-комментарий перед прототипом, получено:\n%s", header)
	}
}

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
	if !strings.Contains(header, "long long сумма(long long v_a, long long v_b);") {
		t.Fatalf("ожидался прототип сумма (настоящее имя, не транслитерация) в заголовке, получено:\n%s", header)
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
	if !strings.Contains(cSource, `__attribute__((visibility("default"))) int публичная(void) {`) {
		t.Fatalf("экспортированная функция должна получить атрибут видимости и настоящее имя, получено:\n%s", cSource)
	}
	if !strings.Contains(cSource, "\nint rech_privatnaya(void) {") {
		t.Fatalf("неэкспортированная функция не должна получать атрибут видимости (и должна остаться транслитерированной), получено:\n%s", cSource)
	}
	if strings.Contains(header, "rech_privatnaya") || strings.Contains(header, "приватная") {
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
	if !strings.Contains(header, "typedef struct Точка Точка;") {
		t.Fatalf("ожидался непрозрачный typedef сущности с настоящим именем (не rech_T_tochka), получено:\n%s", header)
	}
	if !strings.Contains(header, "Точка * новая_точка(long long v_x);") {
		t.Fatalf("ожидался прототип, возвращающий указатель на сущность по настоящему имени, получено:\n%s", header)
	}
}
