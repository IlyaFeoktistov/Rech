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
        Целое x
        Целое y

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

// TestAnalyzeExplicitConstructorNamedFormByParamName - именованная форма у
// явного конструктора сопоставляет по ИМЕНАМ ЕГО ПАРАМЕТРОВ (не полей -
// они не обязаны совпадать, см. package doc entitySig.HasCtor), любой
// порядок, но обязательны все параметры.
func TestAnalyzeExplicitConstructorNamedFormByParamName(t *testing.T) {
	irProg := parseProgram(t, `
сущность Точка
{
    имеет:
        Целое x
        Целое y

    конструктор(x: Целое, y: Целое)
    {
        этот.x = x
        этот.y = y
    }
}
функция главная
{
    переменная точка = новая Точка(y: 4, x: 3)
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
}

// TestAnalyzeExplicitConstructorNamedFormUnknownParamFails - именованная
// форма ссылается на имя, которого нет среди параметров конструктора.
func TestAnalyzeExplicitConstructorNamedFormUnknownParamFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        Целое x

    конструктор(x: Целое)
    {
        этот.x = x
    }
}
функция главная
{
    переменная точка = новая Точка(y: 3)
}
`)
	if err == nil || !strings.Contains(err.Error(), "нет параметра") {
		t.Fatalf("ожидалась ошибка про отсутствующий параметр конструктора, получено: %v", err)
	}
}

// TestAnalyzeExplicitConstructorNamedFormMissingParamFails - именованная
// форма обязана указать ВСЕ параметры конструктора (в отличие от полей у
// "новый" без конструктора, у параметра нет своего значения по умолчанию).
func TestAnalyzeExplicitConstructorNamedFormMissingParamFails(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        Целое x
        Целое y

    конструктор(x: Целое, y: Целое)
    {
        этот.x = x
        этот.y = y
    }
}
функция главная
{
    переменная точка = новая Точка(x: 3)
}
`)
	if err == nil || !strings.Contains(err.Error(), "не указан параметр") {
		t.Fatalf("ожидалась ошибка о непереданном параметре без значения по умолчанию, получено: %v", err)
	}
}

func TestAnalyzeExplicitConstructorWrongArgCount(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        Целое x

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
        Строка имя

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
сущность Сотрудник наследует Пользователя
{
    имеет:
        Целое зарплата

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
        Строка имя

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
сущность Сотрудник наследует Пользователя
{
    имеет:
        Целое зарплата

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
        Строка имя
}
сущность Сотрудник наследует Пользователя
{
    имеет:
        Целое зарплата

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
        Целое x

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
// синтетический ПУСТОЙ (0 параметров, см. package doc analyzeEntity в
// analyzer.go), не конструктор базы, даже если у неё он есть (см. package
// doc entitySig.HasCtor) - "новый Гость(...)" с аргументами поэтому
// недоступен, только "новый Гость()".
func TestAnalyzeEntityWithoutOwnCtorDoesNotInheritBaseCtor(t *testing.T) {
	irProg := parseProgram(t, `
сущность Пользователь
{
    имеет:
        Строка имя

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
сущность Гость наследует Пользователя
{
    имеет:
        Целое код
}
функция главная
{
    переменная гость = новый Гость()
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
	if гость.Ctor == nil || len(гость.Ctor.Params) != 0 {
		t.Fatalf("ожидался синтетический пустой Ctor у наследника без собственного \"конструктор\", получено %+v", гость.Ctor)
	}
}

// TestAnalyzeEntityWithoutOwnCtorRejectsArgs - "новый Гость(...)" с
// аргументами недоступен без собственного "конструктор", даже если у базы
// он есть (см. TestAnalyzeEntityWithoutOwnCtorDoesNotInheritBaseCtor выше).
func TestAnalyzeEntityWithoutOwnCtorRejectsArgs(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Пользователь
{
    имеет:
        Строка имя

    конструктор(имя: Строка)
    {
        этот.имя = имя
    }
}
сущность Гость наследует Пользователя
{
    имеет:
        Целое код
}
функция главная
{
    переменная гость = новый Гость("аноним", 7)
}
`)
	if err == nil || !strings.Contains(err.Error(), "нет явного конструктора") {
		t.Fatalf("ожидалась ошибка про отсутствие явного конструктора, получено: %v", err)
	}
}
