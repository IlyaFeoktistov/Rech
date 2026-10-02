package semantic

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

func TestAnalyzePointerIndexReadWriteAddr(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная
{
    Целое а = 5
    *Целое п = &а
    п[0] = 6
    Целое б = п[0] + п.получить(0)
    *Целое сдвиг = &п[0]
    вывести б
    вывести *сдвиг
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	body := prog.Functions[0].Body
	store, ok := body[2].(*ir.DerefAssignStmt)
	if !ok {
		t.Fatalf("п[0] = 6: ожидался DerefAssignStmt, получен %T", body[2])
	}
	if _, ok := store.Pointer.(*ir.PointerOffsetExpr); !ok {
		t.Fatalf("п[0] = 6: ожидался PointerOffsetExpr, получен %T", store.Pointer)
	}
}

func TestAnalyzePointerIndexErrors(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"индекс не Целое": {`
функция главная
{
    Целое а = 5
    *Целое п = &а
    вывести п["0"]
}
`, "индекс указателя должен быть типа Целое"},
		"установить как выражение": {`
функция главная
{
    Целое а = 5
    *Целое п = &а
    вывести п.установить(0, 1)
}
`, "инструкция"},
		"чужой метод": {`
функция главная
{
    Целое а = 5
    *Целое п = &а
    вывести п.длина()
}
`, "нет метода \"длина\""},
		"тип записи": {`
функция главная
{
    Целое а = 5
    *Целое п = &а
    п[0] = "x"
}
`, "Строка"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := AnalyzeSource(t, c.src)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ожидалась ошибка с %q, получено %v", c.want, err)
			}
		})
	}
}
