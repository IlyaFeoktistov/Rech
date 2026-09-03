package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeCookieValueAndStringTypeCheck(t *testing.T) {
	irProg := parseProgram(t, `
функция главная
{
    переменная запрос = новый Запрос("GET", "", 0, "/", "Cookie: сессия=abc", "")
    переменная имя = значение_куки(запрос, "сессия")
    переменная заголовок = строка_куки("сессия", "abc", новые ОпцииКуки(путь: "/"))
    вывести имя ?? "?"
    вывести заголовок
}
`)
	var opts, req bool
	for _, ent := range irProg.Entities {
		if ent.Name == "ОпцииКуки" {
			opts = true
		}
		if ent.Name == "Запрос" {
			req = true
		}
	}
	if !opts || !req {
		t.Fatalf("ожидались сущности \"Запрос\"/\"ОпцииКуки\" в IR, получено %+v", irProg.Entities)
	}
}

func TestAnalyzeCookieOptionsCanOmitFields(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная опции1 = новые ОпцииКуки()
    переменная опции2 = новые ОпцииКуки(путь: "/", максимальный_возраст: 0)
    переменная опции3 = новые ОпцииКуки(только_http: истина)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeCookieValueReturnsNullableString(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная запрос = новый Запрос("GET", "", 0, "/", "", "")
    переменная имя = значение_куки(запрос, "сессия")
    переменная х: Строка = имя
}
`)
	if err == nil || !strings.Contains(err.Error(), "?Строка") {
		t.Fatalf("ожидалась ошибка типов - значение_куки возвращает ?Строка, не Строка напрямую, получено: %v", err)
	}
}

func TestAnalyzeUserEntityCannotBeNamedOpciiKuki(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность ОпцииКуки
{
    имеет:
        х: Целое
}
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя") {
		t.Fatalf("ожидалась ошибка - \"ОпцииКуки\" зарезервированное имя, получено: %v", err)
	}
}
