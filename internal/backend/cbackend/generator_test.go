package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func mainFunc() *ast.FunctionDecl {
	return &ast.FunctionDecl{
		Name: "главная",
		Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "ок"}}},
	}
}

func TestGenerateFunctionSignatureWithParamsAndReturnType(t *testing.T) {
	fn := &ast.FunctionDecl{
		Name:       "сложить",
		Params:     []ast.Param{{Name: "кого", Type: "Целое"}, {Name: "кому", Type: "Целое"}},
		ReturnType: "Целое",
		Body: []ast.Stmt{
			&ast.ReturnStmt{Value: &ast.BinaryExpr{
				Op:    "+",
				Left:  &ast.Identifier{Name: "кого"},
				Right: &ast.Identifier{Name: "кому"},
			}},
		},
	}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{fn, mainFunc()}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	const wantSig = "long long rech_slozhit(long long v_kogo, long long v_komu) {"
	if !strings.Contains(out, wantSig) {
		t.Fatalf("ожидалась сигнатура %q, получено:\n%s", wantSig, out)
	}
	if !strings.Contains(out, "return (v_kogo + v_komu);") {
		t.Fatalf("ожидался return сложения, получено:\n%s", out)
	}
}

func TestGenerateFunctionWithReturnTypeMustEndWithReturnValue(t *testing.T) {
	fn := &ast.FunctionDecl{
		Name:       "удвоить",
		Params:     []ast.Param{{Name: "кого", Type: "Целое"}},
		ReturnType: "Целое",
		Body:       []ast.Stmt{&ast.PrintStmt{Value: &ast.Identifier{Name: "кого"}}},
	}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{fn, mainFunc()}})
	if err == nil || !strings.Contains(err.Error(), "должна заканчиваться") {
		t.Fatalf("ожидалась ошибка про отсутствие \"вернуть <значение>\" в конце, получено: %v", err)
	}
}

func TestGenerateReturnValueTypeMismatch(t *testing.T) {
	fn := &ast.FunctionDecl{
		Name:       "тип",
		ReturnType: "Целое",
		Body:       []ast.Stmt{&ast.ReturnStmt{Value: &ast.StringLiteral{Value: "не число"}}},
	}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{fn, mainFunc()}})
	if err == nil || !strings.Contains(err.Error(), "но возвращается значение типа") {
		t.Fatalf("ожидалась ошибка несоответствия типа результата, получено: %v", err)
	}
}

func TestGenerateUnknownParamType(t *testing.T) {
	fn := &ast.FunctionDecl{
		Name:   "х",
		Params: []ast.Param{{Name: "кого", Type: "НеизвестныйТип"}},
		Body:   []ast.Stmt{&ast.PrintStmt{Value: &ast.Identifier{Name: "кого"}}},
	}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{fn, mainFunc()}})
	if err == nil || !strings.Contains(err.Error(), "неизвестный тип") {
		t.Fatalf("ожидалась ошибка неизвестного типа параметра, получено: %v", err)
	}
}

func TestGenerateCallExprAndForwardDeclaration(t *testing.T) {
	callee := &ast.FunctionDecl{
		Name:       "сложить",
		Params:     []ast.Param{{Name: "кого", Type: "Целое"}, {Name: "кому", Type: "Целое"}},
		ReturnType: "Целое",
		Body: []ast.Stmt{&ast.ReturnStmt{Value: &ast.BinaryExpr{
			Op:    "+",
			Left:  &ast.Identifier{Name: "кого"},
			Right: &ast.Identifier{Name: "кому"},
		}}},
	}
	main := &ast.FunctionDecl{
		Name: "главная",
		Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.CallExpr{
			Callee: "сложить",
			Args:   []ast.Expr{&ast.IntLiteral{Value: 2}, &ast.IntLiteral{Value: 3}},
		}}},
	}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main, callee}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "long long rech_slozhit(long long v_kogo, long long v_komu);") {
		t.Fatalf("ожидался прототип функции, объявленной ниже точки входа, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_slozhit(2LL, 3LL)") {
		t.Fatalf("ожидался вызов rech_slozhit в теле главной, получено:\n%s", out)
	}
}

func TestGenerateCallWrongArgCount(t *testing.T) {
	callee := &ast.FunctionDecl{Name: "х", Params: []ast.Param{{Name: "кого", Type: "Целое"}}, ReturnType: "Целое", Body: []ast.Stmt{&ast.ReturnStmt{Value: &ast.Identifier{Name: "кого"}}}}
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{&ast.ExprStmt{Call: &ast.CallExpr{Callee: "х"}}}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{callee, main}})
	if err == nil || !strings.Contains(err.Error(), "ожидает аргументов") {
		t.Fatalf("ожидалась ошибка про число аргументов, получено: %v", err)
	}
}

func TestGenerateCallUndeclaredFunction(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{&ast.ExprStmt{Call: &ast.CallExpr{Callee: "неизвестная"}}}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "не объявлена") {
		t.Fatalf("ожидалась ошибка про необъявленную функцию, получено: %v", err)
	}
}

func TestGenerateCallWithoutReturnTypeAsExpressionFails(t *testing.T) {
	callee := &ast.FunctionDecl{Name: "побочный", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "x"}}}}
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.CallExpr{Callee: "побочный"}}}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{callee, main}})
	if err == nil || !strings.Contains(err.Error(), "нельзя использовать как выражение") {
		t.Fatalf("ожидалась ошибка про использование функции без результата как выражения, получено: %v", err)
	}
}

func TestGenerateDuplicateFunctionName(t *testing.T) {
	a := &ast.FunctionDecl{Name: "дубль", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "1"}}}}
	b := &ast.FunctionDecl{Name: "дубль", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "2"}}}}
	main := mainFunc()
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{a, b, main}})
	if err == nil || !strings.Contains(err.Error(), "объявлена более одного раза") {
		t.Fatalf("ожидалась ошибка про повторное объявление функции, получено: %v", err)
	}
}

func TestGenerateDuplicateFunctionMangling(t *testing.T) {
	a := &ast.FunctionDecl{Name: "ёлка", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "1"}}}}
	b := &ast.FunctionDecl{Name: "елка", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "2"}}}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{a, b, mainFunc()}})
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён функций, получено: %v", err)
	}
}

func TestGenerateDuplicateVariableMangling(t *testing.T) {
	fn := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "ёлка", Value: &ast.IntLiteral{Value: 1}},
		&ast.VarDecl{Name: "елка", Value: &ast.IntLiteral{Value: 2}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{fn}})
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён переменных, получено: %v", err)
	}
}

func TestGenerateShadowingSameNameDifferentScopesStillWorks(t *testing.T) {
	fn := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "а", Value: &ast.IntLiteral{Value: 1}},
		&ast.IfStmt{
			Cond: &ast.BoolLiteral{Value: true},
			Then: []ast.Stmt{&ast.VarDecl{Name: "а", Value: &ast.IntLiteral{Value: 2}}},
		},
	}}
	if _, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{fn}}); err != nil {
		t.Fatalf("затенение того же имени во вложенной области не должно быть ошибкой: %v", err)
	}
}

func TestGenerateEntityStruct(t *testing.T) {
	ent := &ast.EntityDecl{Name: "Пользователь", Fields: []ast.Field{
		{Name: "имя", Type: "Строка"},
		{Name: "возраст", Type: "Целое"},
	}}
	out, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{ent}, Functions: []*ast.FunctionDecl{mainFunc()}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "typedef struct {") || !strings.Contains(out, "} rech_T_polzovatel;") {
		t.Fatalf("ожидался typedef struct для сущности, получено:\n%s", out)
	}
	if !strings.Contains(out, "const char * f_imya;") || !strings.Contains(out, "long long f_vozrast;") {
		t.Fatalf("ожидались поля структуры, получено:\n%s", out)
	}
}

func TestGenerateEmptyEntityGetsPlaceholderField(t *testing.T) {
	ent := &ast.EntityDecl{Name: "Пустая"}
	out, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{ent}, Functions: []*ast.FunctionDecl{mainFunc()}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "} rech_T_pustaya;") {
		t.Fatalf("ожидался typedef для пустой сущности, получено:\n%s", out)
	}
}

func TestGenerateEntityUnknownFieldType(t *testing.T) {
	ent := &ast.EntityDecl{Name: "П", Fields: []ast.Field{{Name: "х", Type: "НеизвестныйТип"}}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{ent}, Functions: []*ast.FunctionDecl{mainFunc()}})
	if err == nil || !strings.Contains(err.Error(), "неизвестный тип") {
		t.Fatalf("ожидалась ошибка про неизвестный тип поля, получено: %v", err)
	}
}

func TestGenerateEntityFieldManglingCollision(t *testing.T) {
	ent := &ast.EntityDecl{Name: "П", Fields: []ast.Field{{Name: "ёлка", Type: "Целое"}, {Name: "елка", Type: "Целое"}}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{ent}, Functions: []*ast.FunctionDecl{mainFunc()}})
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён полей, получено: %v", err)
	}
}

func TestGenerateEntityNameManglingCollision(t *testing.T) {
	a := &ast.EntityDecl{Name: "ёлка", Fields: []ast.Field{{Name: "х", Type: "Целое"}}}
	b := &ast.EntityDecl{Name: "елка", Fields: []ast.Field{{Name: "y", Type: "Целое"}}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{a, b}, Functions: []*ast.FunctionDecl{mainFunc()}})
	if err == nil || !strings.Contains(err.Error(), "транслитерируются в одно и то же имя") {
		t.Fatalf("ожидалась ошибка про коллизию мангл-имён сущностей, получено: %v", err)
	}
}

func TestGenerateDuplicateEntityName(t *testing.T) {
	a := &ast.EntityDecl{Name: "П", Fields: []ast.Field{{Name: "х", Type: "Целое"}}}
	b := &ast.EntityDecl{Name: "П", Fields: []ast.Field{{Name: "y", Type: "Целое"}}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{a, b}, Functions: []*ast.FunctionDecl{mainFunc()}})
	if err == nil || !strings.Contains(err.Error(), "объявлена более одного раза") {
		t.Fatalf("ожидалась ошибка про повторное объявление сущности, получено: %v", err)
	}
}

func TestGenerateFunctionWithoutParamsUsesVoid(t *testing.T) {
	fn := &ast.FunctionDecl{Name: "ничего", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "x"}}}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{fn, mainFunc()}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "int rech_nichego(void) {") {
		t.Fatalf("ожидалась сигнатура с (void), получено:\n%s", out)
	}
}
