package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

func TestParseOperatorOverload(t *testing.T) {
	prog, err := parseSource(t, `
структура Вектор
{
    имеет:
        Целое x

    статический публичный оператор +(a: Вектор, b: Вектор): Вектор
    {
        вернуть новый Вектор(a.x + b.x)
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if len(ent.Methods) != 1 {
		t.Fatalf("ожидался один метод, получено: %+v", ent.Methods)
	}
	m := ent.Methods[0]
	if m.Name != ast.OperatorMethodName("+") {
		t.Fatalf("ожидалось синтетическое имя %q, получено %q", ast.OperatorMethodName("+"), m.Name)
	}
	if !m.IsStatic {
		t.Fatalf("оператор обязан быть статическим методом")
	}
	if len(m.Params) != 2 {
		t.Fatalf("ожидалось 2 параметра, получено %d", len(m.Params))
	}
}

func TestParseOperatorOverloadRequiresStatic(t *testing.T) {
	_, err := parseSource(t, `
структура Вектор
{
    имеет:
        Целое x

    публичный оператор +(a: Вектор, b: Вектор): Вектор
    {
        вернуть новый Вектор(a.x + b.x)
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "обязана быть статической") {
		t.Fatalf("ожидалась ошибка про обязательность \"статический\", получено: %v", err)
	}
}

func TestParseOperatorOverloadRequiresTwoParams(t *testing.T) {
	_, err := parseSource(t, `
структура Вектор
{
    имеет:
        Целое x

    статический оператор +(a: Вектор): Вектор
    {
        вернуть a
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "ровно 2 параметра") {
		t.Fatalf("ожидалась ошибка про арность 2, получено: %v", err)
	}
}

func TestParseOperatorOverloadRequiresReturnType(t *testing.T) {
	_, err := parseSource(t, `
структура Вектор
{
    имеет:
        Целое x

    статический оператор +(a: Вектор, b: Вектор)
    {
        вернуть a
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "обязана объявлять тип результата") {
		t.Fatalf("ожидалась ошибка про обязательный тип результата, получено: %v", err)
	}
}

func TestParseOperatorOverloadUnknownSymbolFails(t *testing.T) {
	_, err := parseSource(t, `
структура Вектор
{
    имеет:
        Целое x

    статический оператор &&(a: Вектор, b: Вектор): Вектор
    {
        вернуть a
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "после \"оператор\" ожидался один из") {
		t.Fatalf("ожидалась ошибка про неизвестный символ оператора, получено: %v", err)
	}
}
