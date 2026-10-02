package parser

import "testing"

// TestParseLeadingDotAndCommaContinueStatement - цепочка вызовов с "." на
// следующей строке и запятая в начале строки в параметрах, аргументах,
// литерале списка и "новый Т(...)".
func TestParseLeadingDotAndCommaContinueStatement(t *testing.T) {
	_, err := parseSource(t, `
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

функция ф(
    а: Целое
    , б: Целое
): Целое {
    вернуть а + б
}

функция главная
{
    Список<Целое> сп = [
        1
        , 2
    ]
    сп
        .добавить(ф(1
            , 2))
    Точка т = новая Точка(
        1
        , 2
    )
    вывести "${сп.длина()} ${т.x}"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
