package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestAnalyzeVerbFamilyRevokesStateOnCall - "выйти", объявленный
// противоположностью "войти", снимает ДОКАЗАННОЕ состояние "вошедший" -
// established больше не содержит его после "выйти()", поэтому повторное
// использование производного типа состояния компилируется (established -
// консервативное над-приближение, не "доказанное отсутствие", см. package
// doc checkCallArgs), но со вставленной рантайм-проверкой (ir.
// StateCheckExpr), а не как обычный аргумент напрямую.
func TestAnalyzeVerbFamilyRevokesStateOnCall(t *testing.T) {
	prog := parseProgram(t, `
сущность Пользователь
{
    состояние: вошедший

    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }

    войти(): Пользователь
    {
        этот становится вошедшим
        вернуть этот
    }

    выйти() `+"`противоположность:войти`"+`
    {
    }

    представиться(п: ВошедшийПользователь): Строка
    {
        вернуть п.имя
    }
}
функция главная
{
    переменная пользователь = новый Пользователь("Аня")
    пользователь = пользователь.войти()
    пользователь.выйти()
    вывести пользователь.представиться(пользователь)
}
`)
	fn := findFunc(prog, "главная")
	printStmt := fn.Body[len(fn.Body)-1].(*ir.PrintStmt)
	call := printStmt.Value.(*ir.MethodCallExpr)
	check, ok := call.Args[0].(*ir.StateCheckExpr)
	if !ok {
		t.Fatalf("ожидался StateCheckExpr для аргумента после \"выйти()\", получено %T", call.Args[0])
	}
	if check.State != "вошедший" {
		t.Fatalf("ожидалась проверка состояния \"вошедший\", получено %q", check.State)
	}

	// "пользователь.выйти()" сам несёт RevokesStates - backend'у нужно
	// СНЯТЬ бит в рантайме, не только established на этапе компиляции (см.
	// package doc ir.MethodCallExpr.RevokesStates).
	exitStmt := fn.Body[2].(*ir.ExprStmt)
	if len(exitStmt.MethodCall.RevokesStates) != 1 || exitStmt.MethodCall.RevokesStates[0] != "вошедший" {
		t.Fatalf("ожидался RevokesStates=[\"вошедший\"] у вызова \"выйти()\", получено %+v", exitStmt.MethodCall.RevokesStates)
	}
}

// TestAnalyzeVerbFamilyAllowsUseBeforeOppositeCalled - до вызова "выйти()"
// доказанное состояние ещё доступно (RevokesStates не влияет на established
// раньше самого вызова).
func TestAnalyzeVerbFamilyAllowsUseBeforeOppositeCalled(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    состояние: вошедший

    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }

    войти(): Пользователь
    {
        этот становится вошедшим
        вернуть этот
    }

    выйти() `+"`противоположность:войти`"+`
    {
    }

    представиться(п: ВошедшийПользователь): Строка
    {
        вернуть п.имя
    }
}
функция главная
{
    переменная пользователь = новый Пользователь("Аня")
    пользователь = пользователь.войти()
    вывести пользователь.представиться(пользователь)
    пользователь.выйти()
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeVerbFamilyOpposesUnknownMethodFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    состояние: вошедший
    имеет:
        имя: Строка
    выйти() `+"`противоположность:нетТакого`"+`
    {
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "нет метода с таким именем") {
		t.Fatalf("ожидалась ошибка про отсутствующий метод, получено: %v", err)
	}
}

func TestAnalyzeVerbFamilyOpposesSelfFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    состояние: вошедший
    имеет:
        имя: Строка
    выйти() `+"`противоположность:выйти`"+`
    {
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "самому себе") {
		t.Fatalf("ожидалась ошибка про противоположность самому себе, получено: %v", err)
	}
}

func TestAnalyzeVerbFamilyOpposesMethodWithoutBecomeFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    состояние: вошедший
    имеет:
        имя: Строка
    войти()
    {
    }
    выйти() `+"`противоположность:войти`"+`
    {
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "нечего отменять") {
		t.Fatalf("ожидалась ошибка \"нечего отменять\", получено: %v", err)
	}
}
