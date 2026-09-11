package semantic

import "testing"

// TestAnalyzeDefaultsPoolShrinkEnabledTrue - built-in умолчание (без
// .rechproj вовсе, internal/compiler в этом случае не переопределяет поля)
// - пул воркер-потоков сжимается по умолчанию (Шаг 81).
func TestAnalyzeDefaultsPoolShrinkEnabledTrue(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная {
    вывести "привет"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !prog.PoolShrinkEnabled {
		t.Fatalf("ожидалось PoolShrinkEnabled=true по умолчанию, получено false")
	}
	if prog.PoolIdleTimeoutMs != DefaultPoolIdleTimeoutMs {
		t.Fatalf("ожидалось PoolIdleTimeoutMs=%d по умолчанию, получено %d", DefaultPoolIdleTimeoutMs, prog.PoolIdleTimeoutMs)
	}
}
