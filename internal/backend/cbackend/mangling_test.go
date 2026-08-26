package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestCheckManglingCollisionsRejectsDuplicateFunctionNames(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "ёлка", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.IntLiteral{Value: 1}}}},
		{Name: "елка", Body: []ir.Stmt{&ir.PrintStmt{Value: &ir.IntLiteral{Value: 2}}}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён функций, получено: %v", err)
	}
}

func TestCheckManglingCollisionsRejectsDuplicateEntityNames(t *testing.T) {
	prog := &ir.Program{Entities: []*ir.Entity{
		{Name: "ёлка", Fields: []ir.Field{{Name: "х", Type: ir.Type{Kind: ir.KindInt}}}},
		{Name: "елка", Fields: []ir.Field{{Name: "y", Type: ir.Type{Kind: ir.KindInt}}}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён сущностей, получено: %v", err)
	}
}

func TestCheckManglingCollisionsRejectsDuplicateFieldNames(t *testing.T) {
	prog := &ir.Program{Entities: []*ir.Entity{
		{Name: "П", Fields: []ir.Field{
			{Name: "ёлка", Type: ir.Type{Kind: ir.KindInt}},
			{Name: "елка", Type: ir.Type{Kind: ir.KindInt}},
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
			{Name: "ёлка", Body: []ir.Stmt{}},
			{Name: "елка", Body: []ir.Stmt{}},
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
			&ir.VarDecl{Name: "ёлка", Value: &ir.IntLiteral{Value: 1}},
			&ir.VarDecl{Name: "елка", Value: &ir.IntLiteral{Value: 2}},
		}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "переменные") || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён переменных, получено: %v", err)
	}
}

func TestCheckManglingCollisionsAllowsShadowingAcrossNestedScope(t *testing.T) {
	// Затенение (как в C) - переменная во вложенном блоке с мангл-именем,
	// совпадающим с внешней, но с другим (нижним) блоком, не запрещается
	// этой проверкой (тот же принцип, что и раньше был у AST-пути - каждый
	// блок начинает свою область видимости с нуля, см. checkBlockMangling).
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{
			&ir.VarDecl{Name: "ёлка", Value: &ir.IntLiteral{Value: 1}},
			&ir.IfStmt{
				Cond: &ir.BoolLiteral{Value: true},
				Then: []ir.Stmt{&ir.VarDecl{Name: "елка", Value: &ir.IntLiteral{Value: 2}}},
			},
		}},
	}}
	if err := checkManglingCollisions(prog); err != nil {
		t.Fatalf("затенение в отдельном блоке не должно быть ошибкой: %v", err)
	}
}

func TestCheckManglingCollisionsRejectsDuplicateParamAndLocal(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Params: []ir.Param{{Name: "ёлка", Type: ir.Type{Kind: ir.KindInt}}}, Body: []ir.Stmt{
			&ir.VarDecl{Name: "елка", Value: &ir.IntLiteral{Value: 1}},
		}},
	}}
	err := checkManglingCollisions(prog)
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию параметра и локальной переменной, получено: %v", err)
	}
}

func TestGenerateFromIRRejectsManglingCollisionBeforeGeneratingCode(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "ёлка", Body: []ir.Stmt{}},
		{Name: "елка", Body: []ir.Stmt{}},
	}}
	_, err := GenerateFromIR(prog)
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён от GenerateFromIR, получено: %v", err)
	}
}
