package interp

import "testing"

// TestDBPoolStateReflectsConfiguredSizeAndBusyCount - "бд.состояние_пула()"
// (internal/semantic/dbstate.go/handle_methods.go) - "всего" - НАСТРОЕННЫЙ
// размер пула (не physically-opened count, см. package doc dbPoolState,
// db.go), "занято"/"простаивает" в сумме всегда равны "всего". Соединение
// удерживается ЖИВЫМ дольше времени чтения строк (rows не закрыты явно, RAII
// нет) только для того, чтобы гарантированно увидеть "занято" > 0 - как и в
// cbackend, соединение возвращается в пул при финализации курсора, поэтому
// сам факт печати "занято" происходит ДО завершения запроса.
func TestDBPoolStateReflectsConfiguredSizeAndBusyCount(t *testing.T) {
	out := runSrc(t, `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    переменная сост = бд.состояние_пула()
    вывести `+"`${"+`сост.всего}\n`+"`"+`
    вывести `+"`${"+`(сост.занято + сост.простаивает == сост.всего)}\n`+"`"+`
    бд.закрыть()
}
`)
	want := "1\nистина\n"
	if out != want {
		t.Fatalf("ожидалось %q, получено %q", want, out)
	}
}
