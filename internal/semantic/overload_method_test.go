package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestMethodOverloadResolvesByParamCountAndType(t *testing.T) {
	irProg := parseProgram(t, `
сущность Калькулятор
{
    сложить(а: Целое, б: Целое): Целое
    {
        вернуть а + б
    }
    сложить(а: Строка, б: Строка): Строка
    {
        вернуть а + б
    }
    сложить(а: Целое, б: Целое, ц: Целое): Целое
    {
        вернуть а + б + ц
    }
}
функция главная
{
    переменная калькулятор = новый Калькулятор()
    вывести калькулятор.сложить(2, 3)
    вывести калькулятор.сложить("а", "б")
    вывести калькулятор.сложить(1, 2, 3)
}
`)
	ent := findEntity(irProg, "Калькулятор")
	wantMethodNames := map[string]bool{
		"сложить(Целое,Целое)":       true,
		"сложить(Строка,Строка)":     true,
		"сложить(Целое,Целое,Целое)": true,
	}
	if len(ent.Methods) != 3 {
		t.Fatalf("ожидались 3 метода в IR, получено %d", len(ent.Methods))
	}
	for _, m := range ent.Methods {
		if !wantMethodNames[m.Name] {
			t.Fatalf("неожиданное мангл-имя метода в IR: %q", m.Name)
		}
	}

	main := findFunction(irProg, "главная")
	wantCallees := []string{"сложить(Целое,Целое)", "сложить(Строка,Строка)", "сложить(Целое,Целое,Целое)"}
	for i, want := range wantCallees {
		print, ok := main.Body[i+1].(*ir.PrintStmt)
		if !ok {
			t.Fatalf("инструкция %d: ожидался PrintStmt, получено %T", i+1, main.Body[i+1])
		}
		call, ok := print.Value.(*ir.MethodCallExpr)
		if !ok {
			t.Fatalf("инструкция %d: ожидался ir.MethodCallExpr, получено %T", i+1, print.Value)
		}
		if call.Method != want {
			t.Fatalf("инструкция %d: ожидался Method %q, получено %q", i+1, want, call.Method)
		}
	}
}

func TestMethodOverloadDuplicateParamTypeListFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Калькулятор
{
    сложить(а: Целое, б: Целое): Целое
    {
        вернуть а + б
    }
    сложить(а: Целое, б: Целое): Строка
    {
        вернуть "х"
    }
}
функция главная
{
    вывести "ок"
}
`)
	if err == nil || !strings.Contains(err.Error(), "одинаковым списком типов параметров") {
		t.Fatalf("ожидалась ошибка о дублирующемся списке типов параметров, получено: %v", err)
	}
}

func TestMethodOverloadRejectsAbstractInGroup(t *testing.T) {
	_, err := AnalyzeSource(t, `
абстрактная сущность Калькулятор
{
    сложить(а: Целое, б: Целое): Целое

    сложить(а: Строка, б: Строка): Строка
    {
        вернуть а + б
    }
}
функция главная
{
    вывести "ок"
}
`)
	if err == nil || !strings.Contains(err.Error(), "абстрактных/статических") {
		t.Fatalf("ожидалась ошибка о запрете перегрузки абстрактных/статических методов, получено: %v", err)
	}
}

func TestMethodOverloadRejectsStaticInGroup(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Калькулятор
{
    статический сложить(а: Целое, б: Целое): Целое
    {
        вернуть а + б
    }
    статический сложить(а: Строка, б: Строка): Строка
    {
        вернуть а + б
    }
}
функция главная
{
    вывести "ок"
}
`)
	if err == nil || !strings.Contains(err.Error(), "абстрактных/статических") {
		t.Fatalf("ожидалась ошибка о запрете перегрузки абстрактных/статических методов, получено: %v", err)
	}
}

func TestMethodOverloadRejectsOverrideOfOverloadedGroup(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность База
{
    сложить(а: Целое, б: Целое): Целое
    {
        вернуть а + б
    }
    сложить(а: Строка, б: Строка): Строка
    {
        вернуть а + б
    }
}
сущность Потомок наследует Базу
{
    сложить(а: Целое, б: Целое): Целое
    {
        вернуть 0
    }
}
функция главная
{
    вывести "ок"
}
`)
	if err == nil || !strings.Contains(err.Error(), "нельзя переопределить перегруженный метод") {
		t.Fatalf("ожидалась ошибка о запрете переопределения перегруженного метода, получено: %v", err)
	}
}

func TestMethodOverloadRejectsOverloadOnTopOfInheritedMethod(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность База
{
    сложить(а: Целое, б: Целое): Целое
    {
        вернуть а + б
    }
}
сущность Потомок наследует Базу
{
    сложить(а: Целое, б: Целое): Целое
    {
        вернуть 0
    }
    сложить(а: Строка, б: Строка): Строка
    {
        вернуть а + б
    }
}
функция главная
{
    вывести "ок"
}
`)
	if err == nil || !strings.Contains(err.Error(), "нельзя перегружать метод") {
		t.Fatalf("ожидалась ошибка о запрете перегрузки поверх унаследованного метода, получено: %v", err)
	}
}

// TestMethodOverloadInheritedGroupResolvesInDerived - перегрузка методов
// БАЗОВОЙ сущности продолжает резолвиться корректно через переменную типа
// ПОТОМКА (methodOverloads должен транзитивно наследоваться, см. package
// doc Analyzer.methodOverloads).
func TestMethodOverloadInheritedGroupResolvesInDerived(t *testing.T) {
	irProg := parseProgram(t, `
сущность База
{
    сложить(а: Целое, б: Целое): Целое
    {
        вернуть а + б
    }
    сложить(а: Строка, б: Строка): Строка
    {
        вернуть а + б
    }
}
сущность Потомок наследует Базу
{
    другой_метод(): Целое
    {
        вернуть 1
    }
}
функция главная
{
    переменная п = новый Потомок()
    вывести п.сложить(2, 3)
    вывести п.сложить("а", "б")
}
`)
	main := findFunction(irProg, "главная")
	print1 := main.Body[1].(*ir.PrintStmt)
	call1 := print1.Value.(*ir.MethodCallExpr)
	if call1.Method != "сложить(Целое,Целое)" {
		t.Fatalf("ожидался Method сложить(Целое,Целое), получено %q", call1.Method)
	}
	print2 := main.Body[2].(*ir.PrintStmt)
	call2 := print2.Value.(*ir.MethodCallExpr)
	if call2.Method != "сложить(Строка,Строка)" {
		t.Fatalf("ожидался Method сложить(Строка,Строка), получено %q", call2.Method)
	}
}

// TestMethodOverloadOpposesTagOnOverloadedTargetFails - "войти" перегружен
// (два объявления), "выйти" объявлен его противоположностью - target тега
// найдётся в списке методов сущности произвольно (первое совпадение по
// имени, см. package doc resolveOpposesTag) - явная ошибка вместо тихого
// выбора не той перегрузки.
func TestMethodOverloadOpposesTagOnOverloadedTargetFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    состояние: вошедший

    войти(): Пользователь
    {
        этот становится вошедшим
        вернуть этот
    }
    войти(токен: Строка): Пользователь
    {
        этот становится вошедшим
        вернуть этот
    }
    выйти() `+"`противоположность:войти`"+`
    {
    }
}
функция главная
{
    вывести "ок"
}
`)
	if err == nil || !strings.Contains(err.Error(), "перегружен") {
		t.Fatalf("ожидалась ошибка о теге \"противоположность\" на перегруженный метод, получено: %v", err)
	}
}
