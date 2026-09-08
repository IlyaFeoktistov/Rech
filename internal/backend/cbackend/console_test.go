package cbackend

import (
	"strings"
	"testing"
)

// TestGenerateFromIRRawModeDisablesStdioBuffering - "установить_сырой_режим"
// обязан отключать буферизацию stdin (setvbuf(..., _IONBF, ...)) - без
// этого "можно_прочитать_байт" (poll на самом fd) не видит байты, которые уже
// забрал в СВОЙ внутренний буфер первый же fread на "прочитать" (см. package
// doc consoleRuntimeCode) - живая проверка через pty (не только этот тест)
// подтвердила реальную порчу разбора ANSI-последовательностей без этой
// строки.
func TestGenerateFromIRRawModeDisablesStdioBuffering(t *testing.T) {
	src := `функция главная
{
    установить_сырой_режим(истина)
}
`
	out := generateFromSrc(t, src)
	if !strings.Contains(out, "setvbuf(stdin, NULL, _IONBF, 0)") {
		t.Fatalf("ожидался setvbuf(stdin, NULL, _IONBF, 0) внутри rech_set_raw_mode, получено:\n%s", out)
	}
}
