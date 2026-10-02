package lsp

import (
	"testing"
)

const occurrencesTestSrc = `
сущность Пользователь
{
    имеет:
        Строка пароль

    авторизовать(кем пароль: Строка): Логическое
    {
        вернуть этот.пароль == пароль
    }
}

функция главная
{
    переменная пароль = "секрет"
    переменная пользователь: Пользователь = новый Пользователь()
    пользователь авторизовать паролем
    вывести пароль
}
`

func TestOccurrencesLocalVarIncludesDeclinedForm(t *testing.T) {
	d := newDoc("test://a.rech", occurrencesTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	declLine := lineOf(t, occurrencesTestSrc, `переменная пароль = "секрет"`)
	declCol := runeCol(t, `    переменная пароль = "секрет"`, "пароль")
	occ, ok := d.occurrences(declLine, declCol)
	if !ok {
		t.Fatalf("ожидались вхождения локальной переменной \"пароль\"")
	}
	// Ожидается 3 вхождения: само объявление, "паролем" (словоформа) в
	// естественном вызове метода и буквальное "пароль" в "вывести пароль".
	// НЕ входит: поле "пароль" сущности, параметр "пароль" метода
	// "авторизовать" (другая, ВНУТРЕННЯЯ область видимости), "этот.пароль"
	// (после точки).
	if len(occ) != 3 {
		t.Fatalf("ожидалось 3 вхождения, получено %d: %+v", len(occ), occ)
	}
	sawDeclined := false
	for _, o := range occ {
		if o.Line == declLine {
			continue
		}
		if o.Line == lineOf(t, occurrencesTestSrc, "пользователь авторизовать паролем") {
			sawDeclined = true
		}
	}
	if !sawDeclined {
		t.Fatalf("словоформа \"паролем\" не найдена среди вхождений: %+v", occ)
	}
}

func TestOccurrencesExcludesFieldAccessAfterDot(t *testing.T) {
	d := newDoc("test://a.rech", occurrencesTestSrc)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	// Параметр "пароль" метода "авторизовать" - ОТДЕЛЬНАЯ область видимости
	// от переменной "пароль" функции "главная"; "этот.пароль" внутри тела
	// метода - поле сущности, тоже не должно попасть в её вхождения.
	paramLine := lineOf(t, occurrencesTestSrc, "авторизовать(кем пароль: Строка)")
	paramCol := runeCol(t, "    авторизовать(кем пароль: Строка): Логическое", "пароль")
	occ, ok := d.occurrences(paramLine, paramCol)
	if !ok {
		t.Fatalf("ожидались вхождения параметра \"пароль\"")
	}
	// "вернуть этот.пароль == пароль" - "этот.пароль" (после точки, поле
	// сущности) не должно совпасть, только второе, голое "пароль".
	bodyLine := lineOf(t, occurrencesTestSrc, "вернуть этот.пароль == пароль")
	count := 0
	for _, o := range occ {
		if o.Line == bodyLine {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("ожидалось ровно 1 вхождение в строке %q (не \"этот.пароль\"), получено %d: %+v", bodyLine, count, occ)
	}
}

func TestOccurrencesTopLevelFunction(t *testing.T) {
	src := `
функция сложить(что: Целое, кем: Целое): Целое
{
    вернуть что + кем
}
функция главная
{
    вывести сложить(1, 2)
}
`
	d := newDoc("test://a.rech", src)
	if d.irProg == nil {
		t.Fatalf("документ не разобрался")
	}
	line := lineOf(t, src, "функция сложить")
	col := runeCol(t, "функция сложить(что: Целое, кем: Целое): Целое", "сложить")
	occ, ok := d.occurrences(line, col)
	if !ok {
		t.Fatalf("ожидались вхождения функции \"сложить\"")
	}
	if len(occ) != 2 {
		t.Fatalf("ожидалось 2 вхождения (объявление + вызов), получено %d: %+v", len(occ), occ)
	}
}
