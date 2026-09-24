package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestCheckManglingCollisionsRejectsDuplicateFunctionNames(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "уголь", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}}}},
		{Name: "угол", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.IntLiteral{Value: 2}}}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён функций, получено: %v", err)
	}
}

func TestCheckManglingCollisionsRejectsDuplicateEntityNames(t *testing.T) {
	prog := &ir.Program{Entities: []*ir.Entity{
		{Name: "уголь", Fields: []ir.Field{{Name: "х", Type: ir.Type{Kind: ir.KindInt}}}},
		{Name: "угол", Fields: []ir.Field{{Name: "y", Type: ir.Type{Kind: ir.KindInt}}}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён сущностей, получено: %v", err)
	}
}

func TestCheckManglingCollisionsRejectsDuplicateFieldNames(t *testing.T) {
	prog := &ir.Program{Entities: []*ir.Entity{
		{Name: "П", Fields: []ir.Field{
			{Name: "уголь", Type: ir.Type{Kind: ir.KindInt}},
			{Name: "угол", Type: ir.Type{Kind: ir.KindInt}},
		}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "поля") || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён полей, получено: %v", err)
	}
}

func TestCheckManglingCollisionsRejectsDuplicateMethodNames(t *testing.T) {
	prog := &ir.Program{Entities: []*ir.Entity{
		{Name: "П", Methods: []*ir.Method{
			{Name: "уголь", Body: []ir.Stmt{}},
			{Name: "угол", Body: []ir.Stmt{}},
		}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "методы") || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён методов, получено: %v", err)
	}
}

func TestCheckManglingCollisionsRejectsDuplicateLocalVariablesInSameScope(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "уголь", Value: &ir.IntLiteral{Value: 1}},
			&ir.VarDecl{Name: "угол", Value: &ir.IntLiteral{Value: 2}},
		}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "переменные") || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён переменных, получено: %v", err)
	}
}

// TestCheckManglingCollisionsRejectsDifferentNameAcrossNestedScope - во
// вложенном блоке ДРУГОЕ Rech-имя с тем же C-именем затенило бы внешнее
// прямо в C (внутренний блок видит внешнее имя).
func TestCheckManglingCollisionsRejectsDifferentNameAcrossNestedScope(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "уголь", Value: &ir.IntLiteral{Value: 1}},
			&ir.IfStmt{
				Cond: &ir.BoolLiteral{Value: true},
				Then: []ir.Stmt{&ir.VarDecl{Name: "угол", Value: &ir.IntLiteral{Value: 2}}},
			},
		}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка коллизии через вложенный блок, получено: %v", err)
	}
}

// TestCheckManglingCollisionsAllowsSameNameShadowing - то же Rech-имя во
// вложенном блоке - обычное затенение, как в C.
func TestCheckManglingCollisionsAllowsSameNameShadowing(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "уголь", Value: &ir.IntLiteral{Value: 1}},
			&ir.IfStmt{
				Cond: &ir.BoolLiteral{Value: true},
				Then: []ir.Stmt{&ir.VarDecl{Name: "уголь", Value: &ir.IntLiteral{Value: 2}}},
			},
		}},
	}}
	if err := checkManglingCollisions(prog); err != nil {
		t.Fatalf("затенение тем же именем не должно быть ошибкой: %v", err)
	}
}

// TestTransliterateSeparatesLatinFromCyrillic - сегмент без кириллицы
// получает маркер и сохраняет регистр: "f" и "ф", "x" и "X" различны.
func TestTransliterateSeparatesLatinFromCyrillic(t *testing.T) {
	for name, want := range map[string]string{
		"ф":              "f",
		"f":              "EN_f",
		"X":              "EN_X",
		"ен_ф":           "en_f",
		"поле_x":         "pole_x",
		"Коробка<Point>": "korobka_of_EN_Point",
		"?Point":         "n_EN_Point",
		"сложить(Point)": "slozhit__EN_Point",
		"шаг2":           "shag2",
		"ёлка":           "yolka",
		"2":              "2",
	} {
		if got := transliterate(name); got != want {
			t.Errorf("transliterate(%q) = %q, ожидалось %q", name, got, want)
		}
	}
}

func TestCheckManglingCollisionsRejectsDuplicateParamAndLocal(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Params: []ir.Param{{Name: "уголь", Type: ir.Type{Kind: ir.KindInt}}}, Body: []ir.Stmt{
			&ir.VarDecl{Name: "угол", Value: &ir.IntLiteral{Value: 1}},
		}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию параметра и локальной переменной, получено: %v", err)
	}
}

func TestGenerateFromIRRejectsManglingCollisionBeforeGeneratingCode(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "уголь", Body: []ir.Stmt{}},
		{Name: "угол", Body: []ir.Stmt{}},
	}}
	_, err := GenerateFromIR(prog)
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён от GenerateFromIR, получено: %v", err)
	}
}
