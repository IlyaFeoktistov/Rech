package parser

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

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
        Целое x
        Целое y

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

// TestParseDocCommentBeforeStatementInsideBodyIsError - "/** ... */"
// перед инструкцией, которая ничего не объявляет, - ошибка.
func TestParseDocCommentBeforeStatementInsideBodyIsError(t *testing.T) {
	src := "функция главная\n{\n    /** не объявление */\n    вывести `1\\n`\n}\n"
	if _, err := parseSource(t, src); err == nil || !strings.Contains(err.Error(), "RECH2016") {
		t.Fatalf("ожидалась ошибка RECH2016, получено %v", err)
	}
}

// TestParseDocCommentOnLocalsInterfaceEnum - doc-комментарий у локальной
// переменной (в том числе отложенной "переменная X" и типом первым),
// локальной константы, интерфейса, метода интерфейса и перечисления.
func TestParseDocCommentOnLocalsInterfaceEnum(t *testing.T) {
	src := `
/** Фигура. */
интерфейс Фигура
{
    /** Площадь фигуры. */
    площадь(): Действительное
}

/** Цвет. */
перечисление Цвет { Красный, Зелёный }

функция главная
{
    /** Счётчик. */
    переменная счётчик = 1
    /** Имя. */
    Строка имя = "а"
    /** Предел. */
    константа ПРЕДЕЛ = 10
    /** Отложенная. */
    переменная позже
    позже = 2
    вывести счётчик + ПРЕДЕЛ + позже
    вывести имя
}
`
	prog, err := parseSource(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Interfaces[0].Doc != "Фигура." || prog.Interfaces[0].Methods[0].Doc != "Площадь фигуры." {
		t.Errorf("doc интерфейса/метода: %q / %q", prog.Interfaces[0].Doc, prog.Interfaces[0].Methods[0].Doc)
	}
	if prog.Enums[0].Doc != "Цвет." {
		t.Errorf("doc перечисления: %q", prog.Enums[0].Doc)
	}
	want := map[string]string{"счётчик": "Счётчик.", "имя": "Имя.", "ПРЕДЕЛ": "Предел.", "позже": "Отложенная."}
	for _, st := range prog.Functions[0].Body {
		switch s := st.(type) {
		case *ast.VarDecl:
			if s.Doc != want[s.Name] {
				t.Errorf("doc %s: %q", s.Name, s.Doc)
			}
			delete(want, s.Name)
		case *ast.ConstDecl:
			if s.Doc != want[s.Name] {
				t.Errorf("doc %s: %q", s.Name, s.Doc)
			}
			delete(want, s.Name)
		}
	}
	if len(want) != 0 {
		t.Errorf("не найдены объявления: %v", want)
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

// TestParseDocCommentMustBeAttached - doc-комментарий, после которого нет
// вплотную объявления (пустая строка, "//", ещё один doc, "}" или конец
// файла), - ошибка RECH2016, а не молчаливая привязка к следующему
// объявлению.
func TestParseDocCommentMustBeAttached(t *testing.T) {
	cases := map[string]string{
		"два doc подряд":   "/** Файл. */\n/** Функция. */\nфункция ф()\n{\n}\n",
		"пустая строка":    "/** Функция. */\n\nфункция ф()\n{\n}\n",
		"//-комментарий":   "/** Функция. */\n// заметка\nфункция ф()\n{\n}\n",
		"конец файла":      "функция ф()\n{\n}\n/** висячий */\n",
		"перед } сущности": "сущность Т\n{\n    имеет:\n        Целое x\n    /** висячий */\n}\n",
		"перед } функции":  "функция ф()\n{\n    вывести `1\\n`\n    /** висячий */\n}\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseSource(t, src)
			if err == nil || !strings.Contains(err.Error(), "RECH2016") {
				t.Fatalf("ожидалась ошибка RECH2016, получено %v", err)
			}
		})
	}
}

// TestParseDocCommentAttachedAfterLineComment - "//" ПЕРЕД doc-комментарием
// не мешает: важен только разрыв между "*/" и объявлением.
func TestParseDocCommentAttachedAfterLineComment(t *testing.T) {
	prog, err := parseSource(t, "// файл\n\n/** Функция. */\nфункция ф()\n{\n}\nфункция главная { вывести `ок` }\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.Functions[0].Doc != "Функция." {
		t.Fatalf("неожиданный Doc: %q", prog.Functions[0].Doc)
	}
}
