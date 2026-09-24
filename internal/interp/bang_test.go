package interp

import "testing"

// TestRunBangNegatesBoolAndChecksNullable - "!" у Логическое - отрицание,
// как "не"; у nullable - "== Ничто" со всем сужением guard clause.
func TestRunBangNegatesBoolAndChecksNullable(t *testing.T) {
	got := runSrc(t, `функция проверить(флаг: Логическое, x: ?Целое)
{
    если !флаг {
        вывести `+"`"+`нет флага\n`+"`"+`
    }
    вывести `+"`"+`${!флаг} ${!(1 > 2)} ${!!флаг}\n`+"`"+`
    если !x {
        вывести `+"`"+`x пусто\n`+"`"+`
        вернуть
    }
    вывести `+"`"+`x=${x + 1}\n`+"`"+`
}

функция главная
{
    проверить(ложь, Ничто)
    проверить(истина, 41)
}
`)
	want := "нет флага\nистина истина ложь\nx пусто\nложь истина истина\nx=42\n"
	if got != want {
		t.Fatalf("получили %q, ожидали %q", got, want)
	}
}
