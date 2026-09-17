package semantic

import "testing"

func TestAnalyzeRequestCanOmitFieldsWithDefaults(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новый Запрос(адрес: "example.com")
    переменная б = новый Запрос(адрес: "example.com", метод: "POST", путь: "/api")
    переменная г = новый Запрос("GET", "example.com")
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeRequestRequiresAddress(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новый Запрос(метод: "POST")
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка - у поля \"адрес\" нет значения по умолчанию")
	}
}

func TestAnalyzeResponseCanOmitHeadersAndBody(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новый Ответ(код: 200)
    переменная б = новый Ответ(200)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeResponseRequiresCode(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новый Ответ(заголовки: "")
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка - у поля \"код\" нет значения по умолчанию")
	}
}

func TestAnalyzeDateTimeCanOmitTimeOfDay(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новая ДатаВремя(год: 2026, месяц: 9, день: 17)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeDateTimeRequiresDate(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новая ДатаВремя(час: 12)
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка - у поля \"год\" нет значения по умолчанию")
	}
}

func TestAnalyzeRequestBytesAndResponseBytesCanOmitFields(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = новый ЗапросБайты(адрес: "example.com")
    переменная б = новый ОтветБайты(код: 200)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
