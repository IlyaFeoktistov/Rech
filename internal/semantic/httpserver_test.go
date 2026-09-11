package semantic

import "testing"

// TestAnalyzeDefaultsHTTPMaxRequestBytes - built-in умолчание (без
// .rechproj вовсе, internal/compiler в этом случае не переопределяет поле)
// - предел размера входящего HTTP-запроса выставлен сразу, без единой
// правки.rechproj (HTTP highload пункт 2), тем же
// принципом, что TestAnalyzeDefaultsPoolShrinkEnabledTrue
// (concurrency_defaults_test.go).
func TestAnalyzeDefaultsHTTPMaxRequestBytes(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная {
    вывести "привет"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if prog.HTTPMaxRequestBytes != DefaultHTTPMaxRequestBytes {
		t.Fatalf("ожидалось HTTPMaxRequestBytes=%d по умолчанию, получено %d", DefaultHTTPMaxRequestBytes, prog.HTTPMaxRequestBytes)
	}
}
