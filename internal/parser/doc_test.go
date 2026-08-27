package parser

import "testing"

func TestParseDocCommentOnFunction(t *testing.T) {
	src := `
/** Складывает два числа. */
функция сложить(кого: Целое, кому: Целое): Целое
{
    вернуть кого + кому
}
функция главная { вывести "ок" }
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Functions[0].Doc != "Складывает два числа." {
		t.Fatalf("неожиданный Doc: %q", prog.Functions[0].Doc)
	}
	if prog.Functions[1].Doc != "" {
		t.Fatalf("у функции без doc-комментария Doc должен быть пуст, получено %q", prog.Functions[1].Doc)
	}
}

func TestParseDocCommentOnEntityFieldMethodConst(t *testing.T) {
	src := `
/** Точка на плоскости. */
сущность Точка
{
    /** Максимально допустимая координата. */
    константа ПРЕДЕЛ = 100

    имеет:
        /** Горизонтальная координата. */
        x: Целое
        y: Целое

    /** Расстояние от начала координат. */
    длина(): Целое
    {
        вернуть этот.x
    }
}
функция главная { вывести "ок" }
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	ent := prog.Entities[0]
	if ent.Doc != "Точка на плоскости." {
		t.Fatalf("неожиданный Doc сущности: %q", ent.Doc)
	}
	if ent.Constants[0].Doc != "Максимально допустимая координата." {
		t.Fatalf("неожиданный Doc константы: %q", ent.Constants[0].Doc)
	}
	if ent.Fields[0].Doc != "Горизонтальная координата." {
		t.Fatalf("неожиданный Doc поля x: %q", ent.Fields[0].Doc)
	}
	if ent.Fields[1].Doc != "" {
		t.Fatalf("у поля y без doc-комментария Doc должен быть пуст, получено %q", ent.Fields[1].Doc)
	}
	if ent.Methods[0].Doc != "Расстояние от начала координат." {
		t.Fatalf("неожиданный Doc метода: %q", ent.Methods[0].Doc)
	}
}

func TestParseDocCommentBeforeStatementInsideBodyIsError(t *testing.T) {
	src := `
функция главная
{
    /** это не объявление верхнего уровня */
    переменная х = 1
}
`
	if _, err := parseSource(t, src); err == nil {
		t.Fatalf("ожидалась ошибка - doc-комментарий перед обычной инструкцией внутри тела не поддержан")
	}
}

// TestParseDocCommentJSDocStyleStripsLeadingStars - "/**\n * строка1\n *
// строка2\n */" (JSDoc/Javadoc) - ведущие "*" каждой строки не должны
// попасть в итоговый текст.
func TestParseDocCommentJSDocStyleStripsLeadingStars(t *testing.T) {
	src := `
/**
 * Складывает два числа.
 * Работает только с Целое.
 */
функция сложить(кого: Целое, кому: Целое): Целое
{
    вернуть кого + кому
}
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	want := "Складывает два числа.\nРаботает только с Целое."
	if prog.Functions[0].Doc != want {
		t.Fatalf("Doc = %q, ожидалось %q", prog.Functions[0].Doc, want)
	}
}

func TestParseOrdinaryBlockCommentIsInvisibleEverywhere(t *testing.T) {
	src := `
/* обычный блочный комментарий перед функцией */
функция главная
{
    /* и внутри тела тоже */
    переменная х = 1 /* и посреди строки */
    вывести х
}
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Functions[0].Doc != "" {
		t.Fatalf("обычный /* */ не должен становиться Doc, получено %q", prog.Functions[0].Doc)
	}
}
