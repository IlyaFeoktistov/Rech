package parser

import (
	"strings"
	"testing"
)

func TestParseConstructorAndParentCall(t *testing.T) {
	prog, err := parseSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
сущность Сотрудник наследует Пользователь
{
    имеет:
        зарплата: Целое

    конструктор(имя: Строка, зарплата: Целое)
    {
        родитель(имя)
        этот.зарплата = зарплата
    }
}
функция главная { вывести "ок" }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Entities[0].Ctor == nil || len(prog.Entities[0].Ctor.Params) != 1 {
		t.Fatalf("ожидался конструктор с 1 параметром у Пользователь, получено %+v", prog.Entities[0].Ctor)
	}
	if prog.Entities[1].Ctor == nil || len(prog.Entities[1].Ctor.Body) != 2 {
		t.Fatalf("ожидалось тело конструктора из 2 инструкций у Сотрудник, получено %+v", prog.Entities[1].Ctor)
	}
}

func TestParseStructCannotHaveConstructor(t *testing.T) {
	_, err := parseSource(t, `
структура Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "\"конструктор\" допустим только у") {
		t.Fatalf("ожидалась ошибка про недопустимость конструктора у структуры, получено: %v", err)
	}
}

func TestParseEntityConstructorDeclaredTwiceRejected(t *testing.T) {
	_, err := parseSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }

    конструктор(y: Целое)
    {
        этот.x = y
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "конструктор объявлен более одного раза") {
		t.Fatalf("ожидалась ошибка про повторное объявление конструктора, получено: %v", err)
	}
}

func TestParseParentCallOutsideConstructorRejected(t *testing.T) {
	_, err := parseSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    задать(x: Целое)
    {
        родитель(x)
    }
}
функция главная { вывести "ок" }
`)
	if err == nil || !strings.Contains(err.Error(), "родитель(...)") {
		t.Fatalf("ожидалась ошибка про \"родитель(...)\" вне тела конструктора, получено: %v", err)
	}
}

func TestParseParentCallInFreeFunctionRejected(t *testing.T) {
	_, err := parseSource(t, `
функция главная
{
    родитель()
}
`)
	if err == nil || !strings.Contains(err.Error(), "родитель(...)") {
		t.Fatalf("ожидалась ошибка про \"родитель(...)\" вне тела конструктора, получено: %v", err)
	}
}
