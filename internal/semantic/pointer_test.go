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
