package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// TestOperatorOverloadResolvesToStaticMethodCall - "a + b" резолвится в
// ir.StaticMethodCallExpr на синтетическое имя "оператор+" (см. package
// doc tryOperatorOverload) - тот же узел IR, что и у обычного "Тип::метод(...)",
// поэтому backend'ам не потребовалось ничего нового.
func TestOperatorOverloadResolvesToStaticMethodCall(t *testing.T) {
	irProg := parseProgram(t, `
структура Вектор
{
    имеет:
        Целое x

    статический публичный оператор +(a: Вектор, b: Вектор): Вектор
    {
        вернуть новый Вектор(a.x + b.x)
    }
}
функция главная
{
    переменная a = новый Вектор(1)
    переменная b = новый Вектор(2)
    переменная c = a + b
}
`)
	var main *ir.Function
	for _, fn := range irProg.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	decl, ok := main.Body[2].(*ir.VarDecl)
	if !ok {
		t.Fatalf("ожидался VarDecl, получено %T", main.Body[2])
	}
	call, ok := decl.Value.(*ir.StaticMethodCallExpr)
	if !ok {
		t.Fatalf("ожидался ir.StaticMethodCallExpr, получено %T", decl.Value)
	}
	if call.Entity != "Вектор" || call.Method != "оператор+" {
		t.Fatalf("ожидался вызов Вектор.оператор+, получено %s.%s", call.Entity, call.Method)
	}
}

// TestOperatorOverloadOverridesStructEquality - объявленный "оператор =="
// перекрывает встроенное поле-в-поле сравнение структур.
func TestOperatorOverloadOverridesStructEquality(t *testing.T) {
	irProg := parseProgram(t, `
структура Вектор
{
    имеет:
        Целое x

    статический публичный оператор ==(a: Вектор, b: Вектор): Логическое
    {
        вернуть истина
    }
}
функция главная
{
    переменная a = новый Вектор(1)
    переменная b = новый Вектор(2)
    переменная c = a == b
}
`)
	var main *ir.Function
	for _, fn := range irProg.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	decl := main.Body[2].(*ir.VarDecl)
	call, ok := decl.Value.(*ir.StaticMethodCallExpr)
	if !ok {
		t.Fatalf("ожидался ir.StaticMethodCallExpr, получено %T", decl.Value)
	}
	if call.Method != "оператор==" {
		t.Fatalf("ожидался вызов оператор==, получено %q", call.Method)
	}
}

// TestOperatorOverloadOnlyLeftOperandDeclares - "число + Вектор" не
// подхватывает перегрузку, объявленную на Вектор (только левый операнд
// ищется как объявляющая сторона) - выражение падает на обычной резолюции
// "+" для Целое/Вектор.
func TestOperatorOverloadOnlyLeftOperandDeclares(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Вектор
{
    имеет:
        Целое x

    статический публичный оператор +(a: Вектор, b: Вектор): Вектор
    {
        вернуть новый Вектор(a.x + b.x)
    }
}
функция главная
{
    переменная a = новый Вектор(1)
    переменная c = 1 + a
}
`)
	if err == nil || !strings.Contains(err.Error(), "не поддерживается") {
		t.Fatalf("ожидалась ошибка \"оператор не поддерживается\", получено: %v", err)
	}
}

// TestOperatorOverloadWrongOperandTypeFails - оператор объявлен, но с
// другим списком параметров, чем переданные операнды - явная ошибка про
// несовпадение, а не тихий провал в обычную резолюцию.
func TestOperatorOverloadWrongOperandTypeFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Вектор
{
    имеет:
        Целое x

    статический публичный оператор +(a: Вектор, b: Вектор): Вектор
    {
        вернуть новый Вектор(a.x + b.x)
    }
}
функция главная
{
    переменная a = новый Вектор(1)
    переменная c = a + 5
}
`)
	if err == nil || !strings.Contains(err.Error(), "не подходит для операндов") {
		t.Fatalf("ожидалась ошибка о несовпадении операндов, получено: %v", err)
	}
}

// TestOperatorOverloadMixedTypeParams - второй параметр не обязан быть тем
// же типом, что и первый ("Вектор * Действительное" - масштабирование).
func TestOperatorOverloadMixedTypeParams(t *testing.T) {
	irProg := parseProgram(t, `
структура Вектор
{
    имеет:
        Действительное x

    статический публичный оператор *(a: Вектор, k: Действительное): Вектор
    {
        вернуть новый Вектор(a.x * k)
    }
}
функция главная
{
    переменная a = новый Вектор(2.0)
    переменная c = a * 3.0
}
`)
	var main *ir.Function
	for _, fn := range irProg.Functions {
		if fn.Name == "главная" {
			main = fn
		}
	}
	decl := main.Body[1].(*ir.VarDecl)
	if _, ok := decl.Value.(*ir.StaticMethodCallExpr); !ok {
		t.Fatalf("ожидался ir.StaticMethodCallExpr, получено %T", decl.Value)
	}
}
