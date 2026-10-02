package lsp

import (
	"fmt"
	"testing"
)

func TestAgreementFixesFindsAllMismatches(t *testing.T) {
	src := `сущность Точка
{
    имеет:
        приватный Строка имя
        статический публичный Целое счёт = 0
        приватная Список<Строка> метки
        публичный Вызываемый(Целое): Целое ф
        публичное Целое верно

    публичный показать()
    {
    }
}

приватный Строка адрес = ""

функция главная
{
    Точка т = новый Точка()
}
`
	d := newDoc("test://a.rech", src)
	got := map[string]string{}
	for _, f := range d.agreementFixes() {
		got[fmt.Sprintf("%s@%d", f.tok.Value, f.tok.Line)] = f.want
	}
	want := map[string]string{
		"приватный@4":   "приватная",
		"статический@5": "статическое",
		"публичный@5":   "публичное",
		"приватная@6":   "приватный",
		"приватный@15":  "приватная",
		"новый@19":      "новая",
	}
	if len(got) != len(want) {
		t.Fatalf("получено %v, ожидалось %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: получено %q, ожидалось %q (все: %v)", k, got[k], v, got)
		}
	}
}
