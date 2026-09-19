package interp

import (
	"strings"
	"testing"
)

// TestRunPointerNullLiteralOK - "Ничто" как значение "Указатель<T>" (НЕ
// "?Тип" - см. package doc coerceExpr/ir.NullLiteral, internal/semantic/
// nullable.go) - v.Ptr == nil (evalNullLiteral), "== Ничто"/"!= Ничто"
// читают именно v.Ptr, не v.Nullable (evalNullCheckExpr).
func TestRunPointerNullLiteralOK(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная п: Указатель<Целое> = Ничто
    вывести `+"`${"+`п == Ничто}\n`+"`"+`
    переменная а = 5
    п = &а
    вывести `+"`${"+`п != Ничто}\n`+"`"+`
}
`)
	if got != "истина\nистина\n" {
		t.Fatalf("получили %q, хотели \"истина\\nистина\\n\"", got)
	}
}

// TestRunDerefNullPointerRuntimeError - "*п", где п == Ничто, - ошибка
// исполнения (rtmsg.NullAccess через panicRuntime, pointerTarget.get()), не
// паника самого Go-интерпретатора (nil-receiver разыменование без защиты).
func TestRunDerefNullPointerRuntimeError(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `
функция главная
{
    переменная п: Указатель<Целое> = Ничто
    вывести `+"`${"+`*п}\n`+"`"+`
}
`)
	if err == nil || !strings.Contains(err.Error(), "обращение к пустому значению") {
		t.Fatalf("ожидалась ошибка разыменования Ничто-указателя, получено: %v", err)
	}
}
