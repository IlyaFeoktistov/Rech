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

func entityUser() *ast.EntityDecl {
	return &ast.EntityDecl{Name: "Пользователь", Fields: []ast.Field{
		{Name: "имя", Type: "Строка"},
		{Name: "возраст", Type: "Целое"},
	}}
}

func TestGenerateNewExprAndFieldAccess(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "пользователь", Value: &ast.NewExpr{
			Type: "Пользователь",
			Args: []ast.Expr{&ast.StringLiteral{Value: "Илья"}, &ast.IntLiteral{Value: 30}},
		}},
		&ast.PrintStmt{Value: &ast.FieldAccess{Receiver: &ast.Identifier{Name: "пользователь"}, Field: "имя"}},
	}}
	out, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `(rech_T_polzovatel){.f_imya = "Илья", .f_vozrast = 30LL}`) {
		t.Fatalf("ожидался compound literal конструктора, получено:\n%s", out)
	}
	if !strings.Contains(out, "(v_polzovatel).f_imya") {
		t.Fatalf("ожидалось чтение поля через .f_imya, получено:\n%s", out)
	}
}

func TestGenerateNewExprWrongArgCount(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}}}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "ожидает значений полей") {
		t.Fatalf("ожидалась ошибка про число аргументов конструктора, получено: %v", err)
	}
}

func TestGenerateNewExprWrongArgType(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.IntLiteral{Value: 1}, &ast.IntLiteral{Value: 2}}}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "должно быть типа") {
		t.Fatalf("ожидалась ошибка про тип аргумента конструктора, получено: %v", err)
	}
}

func TestGenerateNewExprUndeclaredEntity(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Неизвестная"}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "неизвестный тип") {
		t.Fatalf("ожидалась ошибка про необъявленную сущность, получено: %v", err)
	}
}

func TestGenerateNewExprLabeledArgs(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{
			Type:       "Пользователь",
			Args:       []ast.Expr{&ast.IntLiteral{Value: 30}, &ast.StringLiteral{Value: "Илья"}},
			FieldNames: []string{"возраст", "имя"},
		}},
	}}
	out, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `(rech_T_polzovatel){.f_vozrast = 30LL, .f_imya = "Илья"}`) {
		t.Fatalf("ожидался compound literal с метками в порядке аргументов, получено:\n%s", out)
	}
}

func TestGenerateNewExprLabeledArgsUnknownField(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{
			Type:       "Пользователь",
			Args:       []ast.Expr{&ast.StringLiteral{Value: "Илья"}, &ast.IntLiteral{Value: 30}},
			FieldNames: []string{"фамилия", "возраст"},
		}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "нет поля") {
		t.Fatalf("ожидалась ошибка про несуществующее поле, получено: %v", err)
	}
}

func TestGenerateNewExprLabeledArgsDuplicateField(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{
			Type:       "Пользователь",
			Args:       []ast.Expr{&ast.StringLiteral{Value: "Илья"}, &ast.StringLiteral{Value: "Пётр"}},
			FieldNames: []string{"имя", "имя"},
		}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "указано более одного раза") {
		t.Fatalf("ожидалась ошибка про дублирующееся поле, получено: %v", err)
	}
}

func TestGenerateNewExprLabeledArgsWrongCount(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{
			Type:       "Пользователь",
			Args:       []ast.Expr{&ast.StringLiteral{Value: "Илья"}},
			FieldNames: []string{"имя"},
		}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "ожидает значений полей") {
		t.Fatalf("ожидалась ошибка про число полей конструктора, получено: %v", err)
	}
}

func TestGenerateNewExprLabeledArgsWrongType(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{
			Type:       "Пользователь",
			Args:       []ast.Expr{&ast.IntLiteral{Value: 1}, &ast.IntLiteral{Value: 30}},
			FieldNames: []string{"имя", "возраст"},
		}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "должно быть типа") {
		t.Fatalf("ожидалась ошибка про тип значения поля, получено: %v", err)
	}
}

func TestGenerateFieldAccessUnknownField(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}, &ast.IntLiteral{Value: 1}}}},
		&ast.PrintStmt{Value: &ast.FieldAccess{Receiver: &ast.Identifier{Name: "p"}, Field: "несуществующее"}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "нет поля") {
		t.Fatalf("ожидалась ошибка про несуществующее поле, получено: %v", err)
	}
}

func TestGenerateFieldAccessOnNonEntity(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "x", Value: &ast.IntLiteral{Value: 1}},
		&ast.PrintStmt{Value: &ast.FieldAccess{Receiver: &ast.Identifier{Name: "x"}, Field: "поле"}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "не сущность") {
		t.Fatalf("ожидалась ошибка про доступ к полю не-сущности, получено: %v", err)
	}
}

func TestGenerateFieldAssign(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}, &ast.IntLiteral{Value: 30}}}},
		&ast.FieldAssign{Receiver: &ast.Identifier{Name: "p"}, Field: "возраст", Value: &ast.IntLiteral{Value: 31}},
	}}
	out, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(v_p).f_vozrast = 31LL;") {
		t.Fatalf("ожидалось присваивание полю, получено:\n%s", out)
	}
}

func TestGenerateFieldAssignTypeMismatch(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}, &ast.IntLiteral{Value: 30}}}},
		&ast.FieldAssign{Receiver: &ast.Identifier{Name: "p"}, Field: "возраст", Value: &ast.StringLiteral{Value: "текст"}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "нельзя присвоить") {
		t.Fatalf("ожидалась ошибка несовпадения типа, получено: %v", err)
	}
}

func TestGenerateFieldAssignUnknownField(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}, &ast.IntLiteral{Value: 30}}}},
		&ast.FieldAssign{Receiver: &ast.Identifier{Name: "p"}, Field: "несуществующее", Value: &ast.IntLiteral{Value: 1}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "нет поля") {
		t.Fatalf("ожидалась ошибка про несуществующее поле, получено: %v", err)
	}
}

func TestGenerateFieldAssignOnNonEntity(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "x", Value: &ast.IntLiteral{Value: 1}},
		&ast.FieldAssign{Receiver: &ast.Identifier{Name: "x"}, Field: "поле", Value: &ast.IntLiteral{Value: 1}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "не сущность") {
		t.Fatalf("ожидалась ошибка про присваивание полю не-сущности, получено: %v", err)
	}
}

func entityUserWithMethods() *ast.EntityDecl {
	ent := entityUser()
	ent.Methods = []*ast.MethodDecl{
		{
			Name: "состариться",
			Body: []ast.Stmt{&ast.FieldAssign{
				Receiver: &ast.ThisExpr{},
				Field:    "возраст",
				Value: &ast.BinaryExpr{
					Op:    "+",
					Left:  &ast.FieldAccess{Receiver: &ast.ThisExpr{}, Field: "возраст"},
					Right: &ast.IntLiteral{Value: 1},
				},
			}},
		},
		{
			Name:       "представиться",
			ReturnType: "Строка",
			Body:       []ast.Stmt{&ast.ReturnStmt{Value: &ast.FieldAccess{Receiver: &ast.ThisExpr{}, Field: "имя"}}},
		},
	}
	return ent
}

func TestGenerateMethodMutatesViaThis(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}, &ast.IntLiteral{Value: 30}}}},
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "p"}, Method: "состариться"}},
		&ast.PrintStmt{Value: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "p"}, Method: "представиться"}},
	}}
	out, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUserWithMethods()}, Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "int rech_M_polzovatel_sostaritsya(rech_T_polzovatel *this_) {") {
		t.Fatalf("ожидалась сигнатура метода с this_, получено:\n%s", out)
	}
	if !strings.Contains(out, "((*this_)).f_vozrast = ((((*this_)).f_vozrast) + 1LL);") &&
		!strings.Contains(out, "((*this_)).f_vozrast = (((*this_)).f_vozrast + 1LL);") {
		t.Fatalf("ожидалась мутация поля через this_, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_polzovatel_sostaritsya(&(v_p))") {
		t.Fatalf("ожидался вызов метода с адресом получателя, получено:\n%s", out)
	}
}

func TestGenerateMethodCallUnknownMethod(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}, &ast.IntLiteral{Value: 1}}}},
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "p"}, Method: "несуществующий"}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "нет метода") {
		t.Fatalf("ожидалась ошибка про несуществующий метод, получено: %v", err)
	}
}

func TestGenerateMethodCallWrongArgCount(t *testing.T) {
	ent := entityUserWithMethods()
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}, &ast.IntLiteral{Value: 1}}}},
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "p"}, Method: "состариться", Args: []ast.Expr{&ast.IntLiteral{Value: 1}}}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{ent}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "ожидает аргументов") {
		t.Fatalf("ожидалась ошибка про число аргументов метода, получено: %v", err)
	}
}

func TestGenerateMethodCallOnNonEntity(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "x", Value: &ast.IntLiteral{Value: 1}},
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "x"}, Method: "метод"}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "не сущность") {
		t.Fatalf("ожидалась ошибка про вызов метода не-сущности, получено: %v", err)
	}
}

func TestGenerateMethodCallReceiverMustBeAddressable(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{
			Receiver: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}, &ast.IntLiteral{Value: 1}}},
			Method:   "состариться",
		}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUserWithMethods()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "получателем вызова метода") {
		t.Fatalf("ожидалась ошибка про недопустимого получателя вызова метода, получено: %v", err)
	}
}

func TestGenerateDuplicateMethodName(t *testing.T) {
	ent := entityUser()
	ent.Methods = []*ast.MethodDecl{
		{Name: "метод", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "1"}}}},
		{Name: "метод", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "2"}}}},
	}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{ent}, Functions: []*ast.FunctionDecl{mainFunc()}})
	if err == nil || !strings.Contains(err.Error(), "объявлен более одного раза") {
		t.Fatalf("ожидалась ошибка про повторное объявление метода, получено: %v", err)
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

func TestGenerateListInstantiationAndMethods(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "числа", Value: &ast.NewExpr{Type: "Список<Целое>"}},
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{
			Receiver: &ast.Identifier{Name: "числа"}, Method: "добавить", Args: []ast.Expr{&ast.IntLiteral{Value: 5}},
		}},
		&ast.PrintStmt{Value: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "числа"}, Method: "получить", Args: []ast.Expr{&ast.IntLiteral{Value: 0}}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "typedef struct {") || !strings.Contains(out, "} rech_T_spisok_of_celoe;") {
		t.Fatalf("ожидался typedef struct для Список<Целое>, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_spisok_of_celoe_dobavit(&(v_chisla), 5LL)") {
		t.Fatalf("ожидался вызов добавить с адресом получателя, получено:\n%s", out)
	}
	if !strings.Contains(out, "static void rech_panic(") {
		t.Fatalf("ожидался хелпер rech_panic (нужен для получить/установить), получено:\n%s", out)
	}
}

func TestGenerateListWrongElementTypeArgument(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "числа", Value: &ast.NewExpr{Type: "Список<Целое>"}},
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{
			Receiver: &ast.Identifier{Name: "числа"}, Method: "добавить", Args: []ast.Expr{&ast.StringLiteral{Value: "текст"}},
		}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "должен быть типа") {
		t.Fatalf("ожидалась ошибка несовпадения типа элемента, получено: %v", err)
	}
}

func TestGenerateListElementEntityUnsupported(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "p", Value: &ast.NewExpr{Type: "Пользователь", Args: []ast.Expr{&ast.StringLiteral{Value: "x"}, &ast.IntLiteral{Value: 1}}}},
		&ast.VarDecl{Name: "список", Value: &ast.NewExpr{Type: "Список<Пользователь>"}},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUser()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "только для Строка/Целое/Число/Логическое") {
		t.Fatalf("ожидалась ошибка про неподдерживаемый элемент-сущность, получено: %v", err)
	}
}

func TestGenerateUnknownGenericTemplate(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "x", Value: &ast.NewExpr{Type: "Мешок<Целое>"}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "неизвестный generic-тип") {
		t.Fatalf("ожидалась ошибка про неизвестный generic-шаблон, получено: %v", err)
	}
}

func TestGenerateTwoListInstantiationsCoexist(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "числа", Value: &ast.NewExpr{Type: "Список<Целое>"}},
		&ast.VarDecl{Name: "слова", Value: &ast.NewExpr{Type: "Список<Строка>"}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_T_spisok_of_celoe") || !strings.Contains(out, "rech_T_spisok_of_stroka") {
		t.Fatalf("ожидались обе инстанциации Списка, получено:\n%s", out)
	}
}

func TestGenerateForEachStmtOverList(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "числа", Value: &ast.NewExpr{Type: "Список<Целое>"}},
		&ast.ForEachStmt{
			Var:        "x",
			Collection: &ast.Identifier{Name: "числа"},
			Body:       []ast.Stmt{&ast.PrintStmt{Value: &ast.Identifier{Name: "x"}}},
		},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "for (long long rech_fe_i_v_x = 0; rech_fe_i_v_x < v_chisla.len; rech_fe_i_v_x++) {") {
		t.Fatalf("ожидался цикл по данным Списка, получено:\n%s", out)
	}
	if !strings.Contains(out, "long long v_x = v_chisla.data[rech_fe_i_v_x];") {
		t.Fatalf("ожидалось присваивание элемента переменной цикла, получено:\n%s", out)
	}
}

func TestGenerateStackInstantiationAndMethods(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "стопка", Value: &ast.NewExpr{Type: "Стопка<Целое>"}},
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{
			Receiver: &ast.Identifier{Name: "стопка"}, Method: "положить", Args: []ast.Expr{&ast.IntLiteral{Value: 5}},
		}},
		&ast.PrintStmt{Value: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "стопка"}, Method: "верх"}},
		&ast.PrintStmt{Value: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "стопка"}, Method: "снять"}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "typedef struct {") || !strings.Contains(out, "} rech_T_stopka_of_celoe;") {
		t.Fatalf("ожидался typedef struct для Стопка<Целое>, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_stopka_of_celoe_polozhit(&(v_stopka), 5LL)") {
		t.Fatalf("ожидался вызов положить с адресом получателя, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_stopka_of_celoe_verh(&(v_stopka))") || !strings.Contains(out, "rech_M_stopka_of_celoe_snyat(&(v_stopka))") {
		t.Fatalf("ожидались вызовы верх/снять, получено:\n%s", out)
	}
}

func TestGenerateStackNotIterableInForEach(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "стопка", Value: &ast.NewExpr{Type: "Стопка<Целое>"}},
		&ast.ForEachStmt{
			Var:        "x",
			Collection: &ast.Identifier{Name: "стопка"},
			Body:       []ast.Stmt{&ast.PrintStmt{Value: &ast.Identifier{Name: "x"}}},
		},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "ожидает переменную-Список<T>") {
		t.Fatalf("ожидалась ошибка про несоответствие типа коллекции (Стопка<T> не перебирается), получено: %v", err)
	}
}

func TestGenerateQueueInstantiationAndMethods(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "очередь", Value: &ast.NewExpr{Type: "Очередь<Целое>"}},
		&ast.ExprStmt{MethodCall: &ast.MethodCallExpr{
			Receiver: &ast.Identifier{Name: "очередь"}, Method: "добавить", Args: []ast.Expr{&ast.IntLiteral{Value: 5}},
		}},
		&ast.PrintStmt{Value: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "очередь"}, Method: "первый"}},
		&ast.PrintStmt{Value: &ast.MethodCallExpr{Receiver: &ast.Identifier{Name: "очередь"}, Method: "извлечь"}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "typedef struct {") || !strings.Contains(out, "} rech_T_ochered_of_celoe;") {
		t.Fatalf("ожидался typedef struct для Очередь<Целое>, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_ochered_of_celoe_dobavit(&(v_ochered), 5LL)") {
		t.Fatalf("ожидался вызов добавить с адресом получателя, получено:\n%s", out)
	}
	if !strings.Contains(out, "rech_M_ochered_of_celoe_pervyj(&(v_ochered))") || !strings.Contains(out, "rech_M_ochered_of_celoe_izvlech(&(v_ochered))") {
		t.Fatalf("ожидались вызовы первый/извлечь, получено:\n%s", out)
	}
}

func TestGenerateQueueNotIterableInForEach(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "очередь", Value: &ast.NewExpr{Type: "Очередь<Целое>"}},
		&ast.ForEachStmt{
			Var:        "x",
			Collection: &ast.Identifier{Name: "очередь"},
			Body:       []ast.Stmt{&ast.PrintStmt{Value: &ast.Identifier{Name: "x"}}},
		},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "ожидает переменную-Список<T>") {
		t.Fatalf("ожидалась ошибка про несоответствие типа коллекции (Очередь<T> не перебирается), получено: %v", err)
	}
}

func TestGenerateForEachStmtNonListError(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "число", Value: &ast.IntLiteral{Value: 5}},
		&ast.ForEachStmt{
			Var:        "x",
			Collection: &ast.Identifier{Name: "число"},
			Body:       []ast.Stmt{&ast.PrintStmt{Value: &ast.Identifier{Name: "x"}}},
		},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "ожидает переменную-Список<T>") {
		t.Fatalf("ожидалась ошибка про несоответствие типа коллекции, получено: %v", err)
	}
}

// entityUserWithStates - как entityUser, но с "состояние:" (шаг 4.1) -
// States здесь заполнен так, как это сделал бы парсер (StateDecl.TypeName
// уже вычислен), потому что генератор сам морфологией не занимается.
func entityUserWithStates() *ast.EntityDecl {
	ent := entityUser()
	ent.States = []ast.StateDecl{
		{Name: "неавторизован", TypeName: "НеавторизованныйПользователь"},
		{Name: "авторизован", TypeName: "АвторизованныйПользователь"},
	}
	return ent
}

func TestGenerateBecomeStmtValidState(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "пользователь", Value: &ast.NewExpr{
			Type: "Пользователь",
			Args: []ast.Expr{&ast.StringLiteral{Value: "Илья"}, &ast.IntLiteral{Value: 30}},
		}},
		&ast.BecomeStmt{Receiver: &ast.Identifier{Name: "пользователь"}, State: "авторизован"},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUserWithStates()}, Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestGenerateBecomeStmtUnknownStateFails(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "пользователь", Value: &ast.NewExpr{
			Type: "Пользователь",
			Args: []ast.Expr{&ast.StringLiteral{Value: "Илья"}, &ast.IntLiteral{Value: 30}},
		}},
		&ast.BecomeStmt{Receiver: &ast.Identifier{Name: "пользователь"}, State: "удалён"},
	}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUserWithStates()}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "не объявлено состояние") {
		t.Fatalf("ожидалась ошибка про необъявленное состояние, получено: %v", err)
	}
}

func TestGenerateIntDivisionUsesCheckedHelper(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.BinaryExpr{Op: "/", Left: &ast.IntLiteral{Value: 7}, Right: &ast.IntLiteral{Value: 2}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
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

func TestGenerateFloatDivisionDoesNotUseCheckedHelper(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.BinaryExpr{Op: "/", Left: &ast.FloatLiteral{Value: 7}, Right: &ast.FloatLiteral{Value: 2}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.Contains(out, "rech_int_div") {
		t.Fatalf("деление Число/Число не должно использовать целочисленный helper, получено:\n%s", out)
	}
}

func TestGenerateMathBuiltinCall(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.CallExpr{Callee: "корень", Args: []ast.Expr{&ast.FloatLiteral{Value: 16}}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
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

func TestGenerateConsoleInputBuiltinCall(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.CallExpr{Callee: "прочитать_строку"}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_read_line()") {
		t.Fatalf("ожидался вызов rech_read_line, получено:\n%s", out)
	}
	if !strings.Contains(out, "static char *rech_read_line(void)") {
		t.Fatalf("ожидалось определение rech_read_line, получено:\n%s", out)
	}
}

func TestGenerateFileBuiltinCalls(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "ф", Value: &ast.CallExpr{Callee: "открыть", Args: []ast.Expr{&ast.StringLiteral{Value: "путь"}}}},
		&ast.PrintStmt{Value: &ast.CallExpr{Callee: "прочитать", Args: []ast.Expr{&ast.Identifier{Name: "ф"}, &ast.IntLiteral{Value: 10}}}},
		&ast.VarDecl{Name: "в", Value: &ast.CallExpr{Callee: "открыть_для_записи", Args: []ast.Expr{&ast.StringLiteral{Value: "путь2"}}}},
		&ast.ExprStmt{Call: &ast.CallExpr{Callee: "записать", Args: []ast.Expr{&ast.Identifier{Name: "в"}, &ast.StringLiteral{Value: "текст"}}}},
		&ast.ExprStmt{Call: &ast.CallExpr{Callee: "закрыть", Args: []ast.Expr{&ast.Identifier{Name: "ф"}}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"rech_open(\"путь\")", "rech_read(v_f, 10LL)", "rech_close(v_f);",
		"rech_open_write(\"путь2\")", "rech_write(v_v, \"текст\");",
		"typedef FILE *rech_file_t;",
		"#ifdef _WIN32\n#include <windows.h>\n#include <shellapi.h>\n#endif",
		"_wfopen(", "fopen(path, mode)", "fread(buf,", "fwrite(s, 1, len, f)", "fclose(f);",
		"static char *rech_read_line_from_file(rech_file_t f, bool *ok)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

func TestGenerateWholeFileBuiltinCalls(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.CallExpr{Callee: "прочитать_файл", Args: []ast.Expr{&ast.StringLiteral{Value: "путь"}}}},
		&ast.ExprStmt{Call: &ast.CallExpr{Callee: "записать_в_файл", Args: []ast.Expr{&ast.StringLiteral{Value: "путь2"}, &ast.StringLiteral{Value: "текст"}}}},
		&ast.ExprStmt{Call: &ast.CallExpr{Callee: "дописать_в_файл", Args: []ast.Expr{&ast.StringLiteral{Value: "путь3"}, &ast.StringLiteral{Value: "хвост"}}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		`rech_read_whole_file("путь")`,
		`rech_write_file("путь2", "текст");`,
		`rech_append_file("путь3", "хвост");`,
		"static char *rech_read_whole_file(const char *path)",
		"static void rech_write_file(const char *path, const char *content)",
		"static void rech_append_file(const char *path, const char *content)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

func TestGenerateForEachFileLine(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "ф", Value: &ast.CallExpr{Callee: "открыть", Args: []ast.Expr{&ast.StringLiteral{Value: "путь"}}}},
		&ast.ForEachStmt{Var: "строка", Collection: &ast.Identifier{Name: "ф"}, Body: []ast.Stmt{
			&ast.PrintStmt{Value: &ast.Identifier{Name: "строка"}},
		}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, want := range []string{
		"for (;;) {",
		"bool rech_fe_ok_v_stroka = false;",
		"const char *v_stroka = rech_read_line_from_file(v_f, &rech_fe_ok_v_stroka);",
		"if (!rech_fe_ok_v_stroka) { break; }",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q в выводе, получено:\n%s", want, out)
		}
	}
}

func TestGenerateStdStreamsBuiltinCalls(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.ExprStmt{Call: &ast.CallExpr{Callee: "записать", Args: []ast.Expr{
			&ast.CallExpr{Callee: "стандартный_вывод"}, &ast.StringLiteral{Value: "ок"},
		}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_write(stdout, \"ок\");") {
		t.Fatalf("ожидался rech_write(stdout, ...), получено:\n%s", out)
	}
}

func TestGenerateContainsBuiltinCall(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.CallExpr{Callee: "содержит", Args: []ast.Expr{
			&ast.StringLiteral{Value: "привет мир"}, &ast.StringLiteral{Value: "мир"},
		}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, `(strstr("привет мир", "мир") != NULL)`) {
		t.Fatalf("ожидался strstr, получено:\n%s", out)
	}
}

// TestGenerateArgsBuiltinCalls - "количество_аргументов"/"аргумент" -
// главная всегда получает настоящие argc/argv (не "void"), независимо от
// того, что вызывающая сторона видит только фиксированную сигнатуру без
// параметров на уровне самого Rech (docs/SPEC.md).
func TestGenerateArgsBuiltinCalls(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.CallExpr{Callee: "количество_аргументов"}},
		&ast.PrintStmt{Value: &ast.CallExpr{Callee: "аргумент", Args: []ast.Expr{&ast.IntLiteral{Value: 0}}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
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

// TestGenerateArgsGlobalsAlwaysPresent - rech_argc/rech_argv объявлены,
// даже когда программа вообще не использует "аргумент"/
// "количество_аргументов" - main() всё равно безусловно присваивает их
// (см. generateCallable) и должен видеть объявление раньше присваивания.
func TestGenerateArgsGlobalsAlwaysPresent(t *testing.T) {
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{mainFunc()}})
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

func TestGenerateBuiltinNameCollisionFails(t *testing.T) {
	corень := &ast.FunctionDecl{
		Name:       "корень",
		Params:     []ast.Param{{Name: "x", Type: "Целое"}},
		ReturnType: "Целое",
		Body:       []ast.Stmt{&ast.ReturnStmt{Value: &ast.Identifier{Name: "x"}}},
	}
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.IntLiteral{Value: 1}}}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{corень, main}})
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя встроенной функции") {
		t.Fatalf("ожидалась ошибка про зарезервированное имя, получено: %v", err)
	}
}

func TestGenerateExternFuncCallUsesRealCName(t *testing.T) {
	ext := &ast.ExternFuncDecl{
		CName:      "sqrt",
		Alias:      "корень2",
		Params:     []ast.Param{{Name: "x", Type: "Число"}},
		ReturnType: "Число",
	}
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.CallExpr{Callee: "корень2", Args: []ast.Expr{&ast.FloatLiteral{Value: 16}}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}, ExternFuncs: []*ast.ExternFuncDecl{ext}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "extern double sqrt(double v_x);") {
		t.Fatalf("ожидался extern-прототип по настоящему C-имени, получено:\n%s", out)
	}
	if !strings.Contains(out, "sqrt(16") {
		t.Fatalf("ожидался вызов по настоящему C-имени sqrt, а не по мангл-имени Alias, получено:\n%s", out)
	}
	if strings.Contains(out, "korenq") || strings.Contains(out, "koren2") {
		t.Fatalf("вызов не должен использовать транслитерацию Alias, получено:\n%s", out)
	}
}

func TestGenerateExternFuncWithoutReturnTypeUsesVoid(t *testing.T) {
	ext := &ast.ExternFuncDecl{CName: "srand", Alias: "srand", Params: []ast.Param{{Name: "семя", Type: "Целое"}}}
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.ExprStmt{Call: &ast.CallExpr{Callee: "srand", Args: []ast.Expr{&ast.IntLiteral{Value: 1}}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}, ExternFuncs: []*ast.ExternFuncDecl{ext}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "extern void srand(long long v_semya);") {
		t.Fatalf("ожидался void-прототип (нет объявленного типа результата), получено:\n%s", out)
	}
}

func TestGenerateExternFuncRejectsNonScalarType(t *testing.T) {
	ent := &ast.EntityDecl{Name: "Точка", Fields: []ast.Field{{Name: "x", Type: "Целое"}}}
	ext := &ast.ExternFuncDecl{CName: "foo", Alias: "foo", Params: []ast.Param{{Name: "p", Type: "Точка"}}}
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.IntLiteral{Value: 1}}}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}, Entities: []*ast.EntityDecl{ent}, ExternFuncs: []*ast.ExternFuncDecl{ext}})
	if err == nil || !strings.Contains(err.Error(), "только Целое/Число/Строка/Логическое") {
		t.Fatalf("ожидалась ошибка про допустимые типы FFI, получено: %v", err)
	}
}

func TestGenerateExternFuncCollidesWithBuiltinFails(t *testing.T) {
	ext := &ast.ExternFuncDecl{CName: "sqrt", Alias: "корень", Params: []ast.Param{{Name: "x", Type: "Число"}}, ReturnType: "Число"}
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.IntLiteral{Value: 1}}}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}, ExternFuncs: []*ast.ExternFuncDecl{ext}})
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя встроенной функции") {
		t.Fatalf("ожидалась ошибка про зарезервированное имя, получено: %v", err)
	}
}

func TestGenerateExternFuncCollidesWithUserFunctionFails(t *testing.T) {
	ext := &ast.ExternFuncDecl{CName: "abs", Alias: "сложить", Params: []ast.Param{{Name: "n", Type: "Целое"}}, ReturnType: "Целое"}
	slozhit := &ast.FunctionDecl{Name: "сложить", Params: []ast.Param{{Name: "a", Type: "Целое"}, {Name: "b", Type: "Целое"}}, ReturnType: "Целое",
		Body: []ast.Stmt{&ast.ReturnStmt{Value: &ast.BinaryExpr{Op: "+", Left: &ast.Identifier{Name: "a"}, Right: &ast.Identifier{Name: "b"}}}}}
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.IntLiteral{Value: 1}}}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{slozhit, main}, ExternFuncs: []*ast.ExternFuncDecl{ext}})
	if err == nil || !strings.Contains(err.Error(), "уже объявлено") {
		t.Fatalf("ожидалась ошибка про уже занятое имя, получено: %v", err)
	}
}

func TestGenerateAssertStmtValidCondition(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.AssertStmt{Cond: &ast.BoolLiteral{Value: true}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "if (!(true)) { rech_panic(") {
		t.Fatalf("ожидалась проверка условия с rech_panic, получено:\n%s", out)
	}
	if !strings.Contains(out, "static void rech_panic(") {
		t.Fatalf("ожидалось определение rech_panic (usesPanic), получено:\n%s", out)
	}
}

func TestGenerateAssertStmtNonBoolFails(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.AssertStmt{Cond: &ast.IntLiteral{Value: 1}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "должно быть логическим") {
		t.Fatalf("ожидалась ошибка про тип условия, получено: %v", err)
	}
}

func TestGenerateDerivedStateTypeUsableAsParam(t *testing.T) {
	fn := &ast.FunctionDecl{
		Name:   "показать",
		Params: []ast.Param{{Name: "пользователь", Type: "АвторизованныйПользователь"}},
		Body:   []ast.Stmt{&ast.PrintStmt{Value: &ast.FieldAccess{Receiver: &ast.Identifier{Name: "пользователь"}, Field: "имя"}}},
	}
	out, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUserWithStates()}, Functions: []*ast.FunctionDecl{fn, mainFunc()}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "int rech_pokazat(rech_T_polzovatel v_polzovatel)") {
		t.Fatalf("ожидалась сигнатура с C-типом базовой сущности (производный тип - псевдоним без своего C-представления), получено:\n%s", out)
	}
}

func TestGenerateDerivedStateTypeNameCollisionFails(t *testing.T) {
	other := &ast.EntityDecl{Name: "АвторизованныйПользователь"}
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{&ast.PrintStmt{Value: &ast.StringLiteral{Value: "ок"}}}}
	_, err := Generate(&ast.Program{Entities: []*ast.EntityDecl{entityUserWithStates(), other}, Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "производным типом состояния") {
		t.Fatalf("ожидалась ошибка про коллизию производного типа состояния с именем сущности, получено: %v", err)
	}
}

func TestGenerateBitwiseOperators(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.BinaryExpr{Op: "|", Left: &ast.IntLiteral{Value: 6}, Right: &ast.IntLiteral{Value: 3}}},
		&ast.PrintStmt{Value: &ast.UnaryExpr{Op: "~", Operand: &ast.IntLiteral{Value: 0}}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(6LL | 3LL)") || !strings.Contains(out, "(~0LL)") {
		t.Fatalf("ожидались побитовые C-операторы, получено:\n%s", out)
	}
}

func TestGenerateBitwiseOnFloatFails(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.PrintStmt{Value: &ast.BinaryExpr{Op: "|", Left: &ast.FloatLiteral{Value: 1}, Right: &ast.IntLiteral{Value: 2}}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "применим только к Целое") {
		t.Fatalf("ожидалась ошибка про тип операнда, получено: %v", err)
	}
}

func TestGenerateIncDecExprPostfixAndPrefix(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "а", Value: &ast.IntLiteral{Value: 5}},
		&ast.PrintStmt{Value: &ast.IncDecExpr{Name: "а", Op: "++", Prefix: false}},
		&ast.PrintStmt{Value: &ast.IncDecExpr{Name: "а", Op: "--", Prefix: true}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(v_a++)") || !strings.Contains(out, "(--v_a)") {
		t.Fatalf("ожидались C-постфикс/префикс ++/--, получено:\n%s", out)
	}
}

func TestGenerateIncDecAsStatement(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.VarDecl{Name: "а", Value: &ast.IntLiteral{Value: 5}},
		&ast.ExprStmt{IncDec: &ast.IncDecExpr{Name: "а", Op: "++", Prefix: false}},
	}}
	out, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "(v_a++);") {
		t.Fatalf("ожидался а++ как самостоятельная инструкция, получено:\n%s", out)
	}
}

func TestGenerateIncDecOnUndeclaredFails(t *testing.T) {
	main := &ast.FunctionDecl{Name: "главная", Body: []ast.Stmt{
		&ast.ExprStmt{IncDec: &ast.IncDecExpr{Name: "а", Op: "++", Prefix: false}},
	}}
	_, err := Generate(&ast.Program{Functions: []*ast.FunctionDecl{main}})
	if err == nil || !strings.Contains(err.Error(), "не объявлена") {
		t.Fatalf("ожидалась ошибка про необъявленную переменную, получено: %v", err)
	}
}
