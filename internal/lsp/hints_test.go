package lsp

import "testing"

// findSuggestion - первая Suggestion, чей Title содержит want, "" - не
// найдена.
func findSuggestion(sugs []Suggestion, want string) (Suggestion, bool) {
	for _, s := range sugs {
		if s.Title == want {
			return s, true
		}
	}
	return Suggestion{}, false
}

func TestSuggestionsSimplifyIncrement(t *testing.T) {
	d := newDoc("test://a.rech", `
функция главная
{
    переменная i = 0
    i = i + 1
    вывести i
}
`)
	sugs := d.suggestions()
	s, ok := findSuggestion(sugs, `Сократить до "i++"`)
	if !ok {
		t.Fatalf("ожидалась подсказка \"Сократить до i++\", получено %+v", sugs)
	}
	if s.NewText != "i++" {
		t.Fatalf("ожидался NewText=%q, получено %q", "i++", s.NewText)
	}
	if s.StartLine != 5 || s.EndLine != 5 {
		t.Fatalf("ожидалась строка 5, получено %+v", s)
	}
}

func TestSuggestionsSimplifyDecrement(t *testing.T) {
	d := newDoc("test://a.rech", `
функция главная
{
    переменная i = 10
    i = i - 1
    вывести i
}
`)
	if _, ok := findSuggestion(d.suggestions(), `Сократить до "i--"`); !ok {
		t.Fatalf("ожидалась подсказка \"Сократить до i--\"")
	}
}

func TestSuggestionsSimplifyCompoundAssign(t *testing.T) {
	d := newDoc("test://a.rech", `
функция главная
{
    переменная сумма = 0
    переменная шаг = 3
    сумма = сумма + шаг
    вывести сумма
}
`)
	s, ok := findSuggestion(d.suggestions(), `Сократить до составного присваивания ("+=")`)
	if !ok {
		t.Fatalf("ожидалась подсказка составного присваивания, получено %+v", d.suggestions())
	}
	if s.NewText != "сумма += шаг" {
		t.Fatalf("ожидался NewText=%q, получено %q", "сумма += шаг", s.NewText)
	}
}

// TestSuggestionsSkipsAlreadyCompound - "i += 1" уже сокращена (парсер
// десахарирует её в ТОТ ЖЕ AST, что и "i = i + 1" - см. package doc
// hints.go) - не должно предлагаться ничего.
func TestSuggestionsSkipsAlreadyCompound(t *testing.T) {
	d := newDoc("test://a.rech", `
функция главная
{
    переменная i = 0
    i += 1
    вывести i
}
`)
	for _, s := range d.suggestions() {
		if s.Line == 5 {
			t.Fatalf("не ожидалась подсказка на уже сокращённое присваивание, получено %+v", s)
		}
	}
}

// TestSuggestionsSkipsUnrelatedAssign - "i = j + 1" (Left операнда бинарного
// выражения - НЕ та же переменная, что слева от "=") - не составное
// присваивание вообще, ничего не предлагается.
func TestSuggestionsSkipsUnrelatedAssign(t *testing.T) {
	d := newDoc("test://a.rech", `
функция главная
{
    переменная i = 0
    переменная j = 5
    i = j + 1
    вывести i
}
`)
	for _, s := range d.suggestions() {
		if s.Line == 6 {
			t.Fatalf("не ожидалась подсказка на \"i = j + 1\", получено %+v", s)
		}
	}
}

// TestSuggestionsFindsCompoundAssignInsideLambdaCallback - "j = j + 1"
// внутри тела лямбды-колбэка (аргумент вызова) - репортнуто пользователем
// (examples/бд/чанкование.rech): раньше walkStmts не спускался внутрь
// выражений вообще, значит never видел тело лямбды, спрятанное там.
func TestSuggestionsFindsCompoundAssignInsideLambdaCallback(t *testing.T) {
	d := newDoc("test://a.rech", `
функция чанковать(обработчик: Вызываемый(Целое))
{
    обработчик(1)
}
функция главная
{
    чанковать(функция(страница: Целое)
    {
        переменная j = 0
        пока j < страница
        {
            j = j + 1
        }
    })
}
`)
	if _, ok := findSuggestion(d.suggestions(), `Сократить до "j++"`); !ok {
		t.Fatalf("ожидалась подсказка \"Сократить до j++\" внутри тела лямбды, получено %+v", d.suggestions())
	}
}

func TestSuggestionsStaticMethodNotUsingThis(t *testing.T) {
	d := newDoc("test://a.rech", `
структура Калькулятор
{
    имеет:
        значение: Целое

    сложить(а: Целое, б: Целое): Целое
    {
        вернуть а + б
    }
}
`)
	s, ok := findSuggestion(d.suggestions(), `Сделать "сложить" статическим`)
	if !ok {
		t.Fatalf("ожидалась подсказка про статический метод, получено %+v", d.suggestions())
	}
	if s.NewText != "статический " {
		t.Fatalf("ожидался NewText=%q, получено %q", "статический ", s.NewText)
	}
}

func TestSuggestionsStaticMethodSkippedWhenUsesThis(t *testing.T) {
	d := newDoc("test://a.rech", `
структура Счётчик
{
    имеет:
        значение: Целое

    увеличить()
    {
        этот.значение = этот.значение + 1
    }
}
`)
	if _, ok := findSuggestion(d.suggestions(), `Сделать "увеличить" статическим`); ok {
		t.Fatalf("не ожидалась подсказка про статический метод - тело использует \"этот\"")
	}
}

// TestSuggestionsStaticMethodSkippedForInterfaceImplementer - метод не
// использует "этот", но сущность реализует интерфейс - осознанно не
// предлагается (см. package doc staticMethodSuggestion про полиморфизм).
func TestSuggestionsStaticMethodSkippedForInterfaceImplementer(t *testing.T) {
	d := newDoc("test://a.rech", `
интерфейс Фигура
{
    площадь(): Целое
}

сущность Точка реализует Фигура
{
    площадь(): Целое
    {
        вернуть 0
    }
}
`)
	if _, ok := findSuggestion(d.suggestions(), `Сделать "площадь" статическим`); ok {
		t.Fatalf("не ожидалась подсказка про статический метод - сущность реализует интерфейс")
	}
}
