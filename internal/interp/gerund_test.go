package interp

import "testing"

func TestRunGerundStmtAutoDetectedForm(t *testing.T) {
	out := runSrc(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция главная
{
    сложив(2, 3)
        вывести результат
        вывести результат * 2
    вывести "после"
}
`)
	if out != "5\n10\nпосле\n" {
		t.Fatalf("получено %q", out)
	}
}

func TestRunGerundStmtExplicitTag(t *testing.T) {
	out := runSrc(t, `
функция странная_функция(а: Целое): Целое `+"`деепричастие:чудив`"+`
{
    вернуть а * 100
}
функция главная
{
    чудив(3)
        вывести результат
}
`)
	if out != "300\n" {
		t.Fatalf("получено %q", out)
	}
}

// TestRunNestedGerundStmtShadowsResult - вложенный деепричастный вызов
// временно затеняет "результат" внешнего, снаружи вложенного блока (но
// внутри внешнего) видно снова внешнее значение - см. package doc
// internal/semantic/gerund.go.
func TestRunNestedGerundStmtShadowsResult(t *testing.T) {
	out := runSrc(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция умножить(а: Целое, б: Целое): Целое
{
    вернуть а * б
}
функция главная
{
    сложив(2, 3)
        вывести результат
        умножив(результат, 10)
            вывести результат
        вывести результат
}
`)
	if out != "5\n50\n5\n" {
		t.Fatalf("получено %q", out)
	}
}

// TestRunGerundStmtReturnPropagatesOutOfEnclosingFunction - "вернуть"
// внутри блока деепричастия завершает саму функцию, не только блок.
func TestRunGerundStmtReturnPropagatesOutOfEnclosingFunction(t *testing.T) {
	out := runSrc(t, `
функция сложить(а: Целое, б: Целое): Целое
{
    вернуть а + б
}
функция проверить(а: Целое): Целое
{
    сложив(а, 100)
        вернуть результат
    вернуть -1
}
функция главная
{
    вывести проверить(5)
}
`)
	if out != "105\n" {
		t.Fatalf("получено %q", out)
	}
}
