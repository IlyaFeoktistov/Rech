package diagnostics

import (
	"regexp"
	"testing"
)

var codeFormatRe = regexp.MustCompile(`^RECH\d{4}$`)

// TestAllCodesWellFormed - каждый зарегистрированный код (internal/lexer,
// internal/parser, internal/semantic, плюс CodeInternal этого пакета - все
// они регистрируются через init()/пакетные var при импорте) соответствует
// формату "RECHxxxx" и несёт непустое описание категории - опечатка вроде
// "RECH20O1" (буква вместо нуля) или пустое Register(code, "") иначе
// осталась бы незамеченной до первого живого прогона с этим кодом.
func TestAllCodesWellFormed(t *testing.T) {
	for code, desc := range Codes() {
		if !codeFormatRe.MatchString(code) {
			t.Errorf("код %q не соответствует формату RECHxxxx", code)
		}
		if desc == "" {
			t.Errorf("код %q зарегистрирован с пустым описанием", code)
		}
	}
}
