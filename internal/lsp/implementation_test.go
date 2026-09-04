package lsp

import "testing"

const implTestSrc = `
интерфейс Логгер
{
    сообщить(текст: Строка)
}

сущность КонсольныйЛоггер реализует Логгер
{
    сообщить(текст: Строка)
    {
        вывести текст
    }
}

сущность ФайловыйЛоггер реализует Логгер
{
    сообщить(текст: Строка)
    {
        вывести текст
    }
}

функция применить(логгер: Логгер)
{
    логгер.сообщить("привет")
}

функция главная
{
    применить(новый КонсольныйЛоггер())
}
`

func TestImplementationOnInterfaceNameJumpsToEachEntity(t *testing.T) {
	d := newDoc("test://impl.rech", implTestSrc)
	line := lineOf(t, implTestSrc, "интерфейс Логгер")
	col := runeCol(t, "интерфейс Логгер", "Логгер")
	positions, ok := d.implementations(line, col)
	if !ok {
		t.Fatalf("ожидались реализации интерфейса \"Логгер\"")
	}
	if len(positions) != 2 {
		t.Fatalf("ожидалось 2 реализации, получено %d: %+v", len(positions), positions)
	}
	wantConsole := lineOf(t, implTestSrc, "сущность КонсольныйЛоггер")
	wantFile := lineOf(t, implTestSrc, "сущность ФайловыйЛоггер")
	got := map[int]bool{positions[0].Line: true, positions[1].Line: true}
	if !got[wantConsole] || !got[wantFile] {
		t.Fatalf("ожидались строки %d и %d, получено %+v", wantConsole, wantFile, positions)
	}
}

func TestImplementationOnMethodCallThroughInterfaceJumpsToEachMethod(t *testing.T) {
	d := newDoc("test://impl.rech", implTestSrc)
	line := lineOf(t, implTestSrc, "логгер.сообщить(\"привет\")")
	col := runeCol(t, "    логгер.сообщить(\"привет\")", "сообщить")
	positions, ok := d.implementations(line, col)
	if !ok {
		t.Fatalf("ожидались реализации метода \"сообщить\"")
	}
	if len(positions) != 2 {
		t.Fatalf("ожидалось 2 реализации метода, получено %d: %+v", len(positions), positions)
	}
	lines := map[int]bool{}
	for _, l := range []string{"сущность КонсольныйЛоггер", "сущность ФайловыйЛоггер"} {
		entLine := lineOf(t, implTestSrc, l)
		// метод объявлен через 2 строки после сущности в обоих случаях фикстуры
		lines[entLine+2] = true
	}
	for _, p := range positions {
		if !lines[p.Line] {
			t.Fatalf("неожиданная строка реализации %d, получено %+v", p.Line, positions)
		}
	}
}

const implCallChainTestSrc = `
интерфейс Логгер
{
    сообщить(текст: Строка)
}

сущность КонсольныйЛоггер реализует Логгер
{
    сообщить(текст: Строка)
    {
        вывести текст
    }
}

сущность ФайловыйЛоггер реализует Логгер
{
    сообщить(текст: Строка)
    {
        вывести текст
    }
}

сущность Контейнер
{
    публичный логгер(): Логгер
    {
        вернуть новый КонсольныйЛоггер()
    }
}

функция главная
{
    переменная контейнер = новый Контейнер()
    контейнер.логгер().сообщить("привет")
}
`

// TestImplementationOnMethodCallOnCallResultJumpsToEachMethod - получатель
// "контейнер.логгер()" сам результат вызова (не простое имя/поле) -
// chainBefore не строит цепочку сквозь "(...)" (см. implementations,
// ветка implementationsForCallReceiver) - нужен IR-путь
// (implementation_call.go), не resolveChain.
func TestImplementationOnMethodCallOnCallResultJumpsToEachMethod(t *testing.T) {
	d := newDoc("test://implchain.rech", implCallChainTestSrc)
	line := lineOf(t, implCallChainTestSrc, "контейнер.логгер().сообщить")
	col := runeCol(t, "    контейнер.логгер().сообщить(\"привет\")", "сообщить")
	positions, ok := d.implementations(line, col)
	if !ok {
		t.Fatalf("ожидались реализации метода \"сообщить\" через результат вызова")
	}
	if len(positions) != 2 {
		t.Fatalf("ожидалось 2 реализации метода, получено %d: %+v", len(positions), positions)
	}
	lines := map[int]bool{}
	for _, l := range []string{"сущность КонсольныйЛоггер", "сущность ФайловыйЛоггер"} {
		entLine := lineOf(t, implCallChainTestSrc, l)
		lines[entLine+2] = true
	}
	for _, p := range positions {
		if !lines[p.Line] {
			t.Fatalf("неожиданная строка реализации %d, получено %+v", p.Line, positions)
		}
	}
}

func TestImplementationOnNonInterfaceNameFindsNothing(t *testing.T) {
	d := newDoc("test://impl.rech", implTestSrc)
	line := lineOf(t, implTestSrc, "функция главная")
	col := runeCol(t, "функция главная", "главная")
	if _, ok := d.implementations(line, col); ok {
		t.Fatalf("не ожидалось реализаций у имени функции \"главная\"")
	}
}
