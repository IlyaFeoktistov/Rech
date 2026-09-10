package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestMangleNameHandlesOverloadSyntax - мангл-имя перегрузки, построенное
// internal/semantic/overload.go (вида "имя(Тип1,Тип2)"), обязано
// транслитерироваться в валидный C-идентификатор (никаких "(", ")", ","
// внутри) и различаться для разных списков параметров - критичная
// предпосылка всей схемы перегрузки (см. OVERLOAD_PLAN.md в истории
// коммитов, "маленький ручной тест").
func TestMangleNameHandlesOverloadSyntax(t *testing.T) {
	cases := []string{
		"сложить()",
		"сложить(Целое)",
		"сложить(Целое,Целое)",
		"сложить(Строка,Строка)",
		"сложить(Целое,Целое,Целое)",
	}
	seen := map[string]string{}
	for _, name := range cases {
		mangled := mangleName(name)
		for _, r := range mangled {
			if r == '(' || r == ')' || r == ',' {
				t.Fatalf("mangleName(%q) = %q содержит недопустимый для C-идентификатора символ %q", name, mangled, r)
			}
		}
		if other, exists := seen[mangled]; exists {
			t.Fatalf("mangleName(%q) и mangleName(%q) дали одинаковый C-идентификатор %q - перегрузки неразличимы", name, other, mangled)
		}
		seen[mangled] = name
	}
}

// TestMangleMethodHandlesOverloadSyntax - тот же принцип, что
// TestMangleNameHandlesOverloadSyntax выше, но для методов сущности
// (mangleMethod(entity, method) - method сам несёт мангл-имя перегрузки).
func TestMangleMethodHandlesOverloadSyntax(t *testing.T) {
	cases := []string{
		"сложить()",
		"сложить(Целое)",
		"сложить(Целое,Целое)",
		"сложить(Строка,Строка)",
	}
	seen := map[string]string{}
	for _, name := range cases {
		mangled := mangleMethod("Калькулятор", name)
		for _, r := range mangled {
			if r == '(' || r == ')' || r == ',' {
				t.Fatalf("mangleMethod(Калькулятор, %q) = %q содержит недопустимый для C-идентификатора символ %q", name, mangled, r)
			}
		}
		if other, exists := seen[mangled]; exists {
			t.Fatalf("mangleMethod(Калькулятор, %q) и mangleMethod(Калькулятор, %q) дали одинаковый C-идентификатор %q", name, other, mangled)
		}
		seen[mangled] = name
	}
}

// TestGenerateFromIRAcceptsOverloadMangledMethodNames - сквозная проверка
// перегрузки МЕТОДА (ir.Entity.Methods) - тем же принципом, что
// TestGenerateFromIRAcceptsOverloadMangledFunctionNames ниже, но для
// методов.
func TestGenerateFromIRAcceptsOverloadMangledMethodNames(t *testing.T) {
	prog := &ir.Program{
		Functions: []*ir.Function{{Name: "главная", Body: []ir.Stmt{}}},
		Entities: []*ir.Entity{{
			Name: "Калькулятор",
			Methods: []*ir.Method{
				{Name: "сложить(Целое,Целое)", Params: []ir.Param{{Name: "а", Type: ir.TypeInt}, {Name: "б", Type: ir.TypeInt}}, ReturnType: ir.TypeInt, HasReturn: true, Body: []ir.Stmt{
					&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "+", Left: &ir.Identifier{Name: "а", Type: ir.TypeInt}, Right: &ir.Identifier{Name: "б", Type: ir.TypeInt}, Type: ir.TypeInt}},
				}},
				{Name: "сложить(Строка,Строка)", Params: []ir.Param{{Name: "а", Type: ir.TypeString}, {Name: "б", Type: ir.TypeString}}, ReturnType: ir.TypeString, HasReturn: true, Body: []ir.Stmt{
					&ir.ReturnStmt{Value: &ir.Identifier{Name: "а", Type: ir.TypeString}},
				}},
			},
		}},
	}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	wantIntOverload := mangleMethod("Калькулятор", "сложить(Целое,Целое)") + "("
	if !strings.Contains(out, wantIntOverload) {
		t.Fatalf("не найдена C-функция %q для перегрузки метода сложить(Целое,Целое) в:\n%s", wantIntOverload, out)
	}
	wantStringOverload := mangleMethod("Калькулятор", "сложить(Строка,Строка)") + "("
	if !strings.Contains(out, wantStringOverload) {
		t.Fatalf("не найдена C-функция %q для перегрузки метода сложить(Строка,Строка) в:\n%s", wantStringOverload, out)
	}
}

// TestGenerateFromIRAcceptsOverloadMangledFunctionNames - сквозная проверка
// на уровне ir.Program -> C: две "функции" с именами вида результата
// overloadMangledName (как их видит cbackend - для него это просто разные
// строки Name, семантика перегрузки уже решена в internal/semantic) не
// вызывают ошибку коллизии мангл-имён и не встречаются в тексте
// сгенерированного кода как "(", "," - и не встречаются ДРУГ В ДРУГЕ по
// сгенерированному символу.
func TestGenerateFromIRAcceptsOverloadMangledFunctionNames(t *testing.T) {
	prog := &ir.Program{Functions: []*ir.Function{
		{Name: "главная", Body: []ir.Stmt{}},
		{Name: "сложить(Целое,Целое)", Params: []ir.Param{{Name: "а", Type: ir.TypeInt}, {Name: "б", Type: ir.TypeInt}}, ReturnType: ir.TypeInt, HasReturn: true, Body: []ir.Stmt{
			&ir.ReturnStmt{Value: &ir.BinaryExpr{Op: "+", Left: &ir.Identifier{Name: "а", Type: ir.TypeInt}, Right: &ir.Identifier{Name: "б", Type: ir.TypeInt}, Type: ir.TypeInt}},
		}},
		{Name: "сложить(Целое,Целое,Целое)", Params: []ir.Param{{Name: "а", Type: ir.TypeInt}, {Name: "б", Type: ir.TypeInt}, {Name: "ц", Type: ir.TypeInt}}, ReturnType: ir.TypeInt, HasReturn: true, Body: []ir.Stmt{
			&ir.ReturnStmt{Value: &ir.Identifier{Name: "а", Type: ir.TypeInt}},
		}},
	}}
	out, err := GenerateFromIR(prog)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !strings.Contains(out, "rech_slozhit__celoe_celoe(") {
		t.Fatalf("не найдена C-функция для перегрузки сложить(Целое,Целое) в:\n%s", out)
	}
	if !strings.Contains(out, "rech_slozhit__celoe_celoe_celoe(") {
		t.Fatalf("не найдена C-функция для перегрузки сложить(Целое,Целое,Целое) в:\n%s", out)
	}
}
