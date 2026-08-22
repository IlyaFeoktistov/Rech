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
