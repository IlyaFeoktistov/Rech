package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzeExplicitConstructorPositionalCall(t *testing.T) {
	irProg := parseProgram(t, `
сущность Точка
{
    имеет:
        x: Целое
        y: Целое

    конструктор(x: Целое, y: Целое)
    {
        этот.x = x
        этот.y = y
    }
}
функция главная
{
    переменная точка = новая Точка(3, 4)
    вывести точка.x
}
`)
	var точка *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Точка" {
			точка = ent
		}
	}
	if точка == nil || точка.Ctor == nil {
		t.Fatalf("ожидался ir.Entity.Ctor у сущности с явным конструктором")
	}
	if len(точка.Ctor.Params) != 2 {
		t.Fatalf("ожидалось 2 параметра конструктора, получено %d", len(точка.Ctor.Params))
	}
}

func TestAnalyzeExplicitConstructorRejectsNamedFieldForm(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
функция главная
{
    переменная точка = новая Точка(x: 3)
}
`)
	if err == nil || !strings.Contains(err.Error(), "именованная форма") {
		t.Fatalf("ожидалась ошибка про недоступность именованной формы, получено: %v", err)
	}
}

func TestAnalyzeExplicitConstructorWrongArgCount(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
функция главная
{
    переменная точка = новая Точка(1, 2)
}
`)
	if err == nil || !strings.Contains(err.Error(), "ожидает аргументов") {
		t.Fatalf("ожидалась ошибка о числе аргументов конструктора, получено: %v", err)
	}
}

func TestAnalyzeParentCallRunsBaseConstructor(t *testing.T) {
	irProg := parseProgram(t, `
сущность Пользователь
{
    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
сущность Сотрудник наследует Пользователя
{
    имеет:
        зарплата: Целое

    конструктор(имя: Строка, зарплата: Целое)
    {
        родитель(имя)
        этот.зарплата = зарплата
    }
}
функция главная
{
    переменная сотрудник = новый Сотрудник("Аня", 1000)
}
`)
	var сотрудник *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Сотрудник" {
			сотрудник = ent
		}
	}
	if сотрудник == nil || сотрудник.Ctor == nil || len(сотрудник.Ctor.Body) == 0 {
		t.Fatalf("ожидалось тело конструктора Сотрудник")
	}
	pc, ok := сотрудник.Ctor.Body[0].(*ir.ParentCallStmt)
	if !ok {
		t.Fatalf("ожидался ir.ParentCallStmt первой инструкцией, получено %T", сотрудник.Ctor.Body[0])
	}
	if pc.BaseEntity != "Пользователь" {
		t.Fatalf("ожидался BaseEntity %q, получено %q", "Пользователь", pc.BaseEntity)
	}
}

func TestAnalyzeParentCallMustBeFirstStatement(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
сущность Сотрудник наследует Пользователя
{
    имеет:
        зарплата: Целое

    конструктор(имя: Строка, зарплата: Целое)
    {
        этот.зарплата = зарплата
        родитель(имя)
    }
}
функция главная
{
    переменная сотрудник = новый Сотрудник("Аня", 1000)
}
`)
	if err == nil || !strings.Contains(err.Error(), "первой инструкцией") {
		t.Fatalf("ожидалась ошибка про позицию \"родитель(...)\", получено: %v", err)
	}
}

func TestAnalyzeParentCallWithoutBaseConstructorFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    имеет:
        имя: Строка
}
сущность Сотрудник наследует Пользователя
{
    имеет:
        зарплата: Целое

    конструктор(имя: Строка, зарплата: Целое)
    {
        родитель(имя)
        этот.имя = имя
        этот.зарплата = зарплата
    }
}
функция главная
{
    переменная сотрудник = новый Сотрудник("Аня", 1000)
}
`)
	if err == nil || !strings.Contains(err.Error(), "нет собственного конструктора") {
		t.Fatalf("ожидалась ошибка про отсутствие собственного конструктора у базовой сущности, получено: %v", err)
	}
}

func TestAnalyzeParentCallWithoutBaseNameFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        родитель()
        этот.x = x
    }
}
функция главная
{
    переменная точка = новая Точка(1)
}
`)
	if err == nil || !strings.Contains(err.Error(), "нет базовой сущности") {
		t.Fatalf("ожидалась ошибка про отсутствие базовой сущности, получено: %v", err)
	}
}

// TestAnalyzeEntityWithoutOwnCtorDoesNotInheritBaseCtor - HasCtor/CtorParams
// не наследуются: у наследника без своего "конструктор" ir.Entity.Ctor -
// nil, даже если у базы он есть (см. package doc entitySig.HasCtor).
func TestAnalyzeEntityWithoutOwnCtorDoesNotInheritBaseCtor(t *testing.T) {
	irProg := parseProgram(t, `
сущность Пользователь
{
    имеет:
        имя: Строка

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
сущность Гость наследует Пользователя
{
    имеет:
        код: Целое
}
функция главная
{
    переменная гость = новый Гость("аноним", 7)
}
`)
	var гость *ir.Entity
	for _, ent := range irProg.Entities {
		if ent.Name == "Гость" {
			гость = ent
		}
	}
	if гость == nil {
		t.Fatalf("сущность Гость не найдена")
	}
	if гость.Ctor != nil {
		t.Fatalf("ожидался nil Ctor у наследника без собственного \"конструктор\", получено %+v", гость.Ctor)
	}
}
