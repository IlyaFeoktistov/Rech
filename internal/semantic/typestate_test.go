package semantic

import (
	"fmt"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

const typestateEntities = `
сущность Пользователь
{
    состояние: авторизован

    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }

    авторизовать()
    {
        этот становится авторизованным
    }

    представиться(п: АвторизованныйПользователь): Строка
    {
        вернуть п.имя
    }
}
`

// TestCheckCallArgsEstablishedArgIsPlain - переход, доказанный established в
// пределах того же тела, компилируется как обычный аргумент - без
// ir.StateCheckExpr.
func TestCheckCallArgsEstablishedArgIsPlain(t *testing.T) {
	prog := parseProgram(t, typestateEntities+`
функция главная
{
    переменная п = новый Пользователь("Аня")
    п.авторизовать()
    вывести п.представиться(п)
}
`)
	fn := findFunc(prog, "главная")
	printStmt := fn.Body[2].(*ir.PrintStmt)
	call := printStmt.Value.(*ir.MethodCallExpr)
	if _, ok := call.Args[0].(*ir.StateCheckExpr); ok {
		t.Fatalf("established доказал переход - ожидался обычный аргумент без StateCheckExpr, получено %T", call.Args[0])
	}
	if _, ok := call.Args[0].(*ir.Identifier); !ok {
		t.Fatalf("ожидался Identifier, получено %T", call.Args[0])
	}
}

// TestCheckCallArgsUnprovenArgWrapsInStateCheck - недоказанный на этапе
// компиляции переход (эффект чужой функции established не отслеживает) -
// НЕ ошибка компиляции, а ir.StateCheckExpr вокруг того же аргумента.
func TestCheckCallArgsUnprovenArgWrapsInStateCheck(t *testing.T) {
	prog := parseProgram(t, typestateEntities+`
функция авторизовать_кого_то(п: Пользователь)
{
    п.авторизовать()
}
функция главная
{
    переменная п = новый Пользователь("Аня")
    авторизовать_кого_то(п)
    вывести п.представиться(п)
}
`)
	fn := findFunc(prog, "главная")
	printStmt := fn.Body[2].(*ir.PrintStmt)
	call := printStmt.Value.(*ir.MethodCallExpr)
	check, ok := call.Args[0].(*ir.StateCheckExpr)
	if !ok {
		t.Fatalf("ожидался ir.StateCheckExpr, получено %T", call.Args[0])
	}
	if check.Entity != "Пользователь" || check.State != "авторизован" {
		t.Fatalf("ожидалась проверка Пользователь/авторизован, получено %s/%s", check.Entity, check.State)
	}
	if _, ok := check.Value.(*ir.Identifier); !ok {
		t.Fatalf("ожидался Identifier под проверкой, получено %T", check.Value)
	}
}

// TestCheckCallArgsInvalidatesEstablishedAfterOpaqueCall - сущность,
// переданная ПРОСТЫМ именем в обычный параметр (по ссылке), теряет
// established целиком после вызова - опаque-функция могла снять
// established состояние своим собственным вызовом внутри тела (здесь -
// метод-противоположность), established не может этого узнать, поэтому
// консервативно инвалидируется целиком (см. package doc checkCallArgs,
// invalidateEstablishedForOpaqueArg). Без этого повторное использование
// того же значения после опаque-вызова ошибочно осталось бы "доказанным".
func TestCheckCallArgsInvalidatesEstablishedAfterOpaqueCall(t *testing.T) {
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
функция обернуть_выход(п: Пользователь)
{
    п.выйти()
}
функция главная
{
    переменная аня = новый Пользователь("Аня")
    аня = аня.войти()
    обернуть_выход(аня)
    вывести аня.представиться(аня)
}
`)
	fn := findFunc(prog, "главная")
	printStmt := fn.Body[3].(*ir.PrintStmt)
	call := printStmt.Value.(*ir.MethodCallExpr)
	check, ok := call.Args[0].(*ir.StateCheckExpr)
	if !ok {
		t.Fatalf("established должен был инвалидироваться вызовом \"обернуть_выход(аня)\" - ожидался ir.StateCheckExpr, получено %T", call.Args[0])
	}
	if check.State != "вошедший" {
		t.Fatalf("ожидалась проверка состояния \"вошедший\", получено %q", check.State)
	}
}

// TestAnalyzeIsExprStateAliasBuildsHasStateExpr - "относится к
// <ПроизводныйТипСостояния>" - ir.HasStateExpr (запрос текущего состояния),
// не ir.IsExpr (проверка типа) - Target зарегистрирован как alias
// состояния (sc.a.aliasState), не просто резолвится как синоним типа.
func TestAnalyzeIsExprStateAliasBuildsHasStateExpr(t *testing.T) {
	prog := parseProgram(t, typestateEntities+`
функция главная
{
    переменная п = новый Пользователь("Аня")
    если п относится к АвторизованныйПользователь
    {
        вывести "да"
    }
}
`)
	fn := findFunc(prog, "главная")
	ifStmt := fn.Body[1].(*ir.IfStmt)
	check, ok := ifStmt.Cond.(*ir.HasStateExpr)
	if !ok {
		t.Fatalf("ожидался ir.HasStateExpr, получено %T", ifStmt.Cond)
	}
	if check.Entity != "Пользователь" || check.State != "авторизован" {
		t.Fatalf("ожидалась проверка Пользователь/авторизован, получено %s/%s", check.Entity, check.State)
	}
}

// TestAnalyzeIsExprStateAliasRejectsUnrelatedType - "относится к
// <ПроизводныйТипСостояния>" применим только к значению той сущности,
// которая объявляет это состояние (или её наследнику) - иначе ошибка
// компиляции, а не "всегда ложь".
func TestAnalyzeIsExprStateAliasRejectsUnrelatedType(t *testing.T) {
	_, err := AnalyzeSource(t, typestateEntities+`
сущность Точка
{
    имеет:
        x: Целое = 0
}
функция главная
{
    переменная т = новая Точка()
    если т относится к АвторизованныйПользователь
    {
        вывести "да"
    }
}
`)
	if err == nil || !strings.Contains(err.Error(), "не является") {
		t.Fatalf("ожидалась ошибка о несвязанном типе, получено: %v", err)
	}
}

// TestApplyInheritanceTooManyStatesFails - суммарно больше 64 состояний по
// цепочке наследования (одно хранилище "state_flags" на всю цепочку,
// internal/ir.Entity.HasStateFlags) - ошибка компиляции, а не молчаливое
// переполнение битовой маски. Проверяет applyInheritance напрямую
// (entitySig/ast.EntityDecl, минуя парсер) - настоящие 66 различных слов,
// прошедших курируемую морфологическую проверку "состояние:", неоткуда
// взять для одного теста.
func TestApplyInheritanceTooManyStatesFails(t *testing.T) {
	baseStates := make([]string, 40)
	for i := range baseStates {
		baseStates[i] = fmt.Sprintf("состояниеБ%d", i)
	}
	ownStates := make([]string, 30)
	for i := range ownStates {
		ownStates[i] = fmt.Sprintf("состояниеП%d", i)
	}
	a := &Analyzer{}
	entities := map[string]entitySig{"База": {StateNames: baseStates, HasStateFlags: true}}
	methods := map[string]map[string]funcSig{"База": {}}
	ent := &ast.EntityDecl{Name: "Потомок", BaseName: "База", Line: 1}
	ownSig := entitySig{StateNames: ownStates, HasStateFlags: true}
	_, _, _, err := a.applyInheritance(entities, methods, ent, ownSig, map[string]funcSig{}, map[string][]funcSig{})
	if err == nil || !strings.Contains(err.Error(), "вмещает не больше 64") {
		t.Fatalf("ожидалась ошибка о переполнении хранилища состояний, получено: %v", err)
	}
}
