package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzePointerRoundTrip(t *testing.T) {
	prog, err := AnalyzeSource(t, `
сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}

функция изменить(п: Указатель<Целое>)
{
    *п = 99
}

функция главная
{
    переменная а = 5
    переменная указ = &а
    вывести *указ
    изменить(&а)
    вывести а

    переменная т = новая Точка(1)
    переменная указНаПоле = &т.x
    вывести *указНаПоле
    *указНаПоле = 42

    переменная сп = новый Список<Целое>()
    сп.добавить(10)
    переменная указНаЭлемент = &сп[0]
    вывести *указНаЭлемент
    *указНаЭлемент = 77

    переменная пп = &указ
    вывести **пп
    **пп = 123
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	wantPointers := map[string]string{
		"Указатель<Целое>":            "Целое",
		"Указатель<Указатель<Целое>>": "Указатель<Целое>",
	}
	if len(prog.Pointers) != len(wantPointers) {
		t.Fatalf("ожидалось %d инстанциаций Указатель, получено %v", len(wantPointers), prog.Pointers)
	}
	for _, p := range prog.Pointers {
		if want, ok := wantPointers[p.Name]; !ok || p.Inner.String() != want {
			t.Fatalf("неожиданная инстанциация %q -> %s", p.Name, p.Inner)
		}
	}
	for _, g := range prog.Generics {
		if strings.HasPrefix(g.Name, "Указатель<") {
			t.Fatalf("Указатель<T> не должен попадать в Program.Generics: %q", g.Name)
		}
	}
}

func TestAnalyzeFuncRefStillWorksAlongsidePointers(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция удвоить(х: Целое): Целое { вернуть х * 2 }
функция главная
{
    переменная ф = &удвоить
    вывести ф(21)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzePointerNullLiteralOK - "Ничто" как значение "Указатель<T>" -
// НЕ через "?Тип" (см. package doc coerceExpr, nullable.go): var-decl с
// аннотацией типа, обычное присваивание, аргумент вызова и сравнение
// "== Ничто"/"!= Ничто" - все точки, идущие через coerceExpr/
// analyzeNullCheck.
func TestAnalyzePointerNullLiteralOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция взять_указатель(п: Указатель<Целое>) { }
функция главная
{
    переменная а: Указатель<Целое> = Ничто
    утверждать а == Ничто
    переменная б = 5
    а = &б
    утверждать а != Ничто
    взять_указатель(Ничто)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeNullLiteralStillRejectedForNonPointerNonNullable - "Ничто" по-
// прежнему недопустимо для обычного (не "?Тип", не "Указатель<T>") типа.
func TestAnalyzeNullLiteralStillRejectedForNonPointerNonNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а: Целое = Ничто
}
`)
	if err == nil || !strings.Contains(err.Error(), "\"Ничто\" допустимо только") {
		t.Fatalf("ожидалась ошибка про недопустимость \"Ничто\" здесь, получено: %v", err)
	}
}

func TestAnalyzeDerefRejectsNonPointer(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная а = 5
    вывести *а
}
`)
	if err == nil || !strings.Contains(err.Error(), "Указатель<T>") {
		t.Fatalf("ожидалась ошибка про \"Указатель<T>\", получено: %v", err)
	}
}

func TestAnalyzeAddressOfRejectsUndeclaredName(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    вывести *&несуществующее.поле
}
`)
	if err == nil || !strings.Contains(err.Error(), "не объявлено") {
		t.Fatalf("ожидалась ошибка про необъявленное имя, получено: %v", err)
	}
}
