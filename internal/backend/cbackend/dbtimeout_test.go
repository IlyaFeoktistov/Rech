package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// generateFromSrcWithDBPoolTimeoutMs - как generateFromSrc (db_test.go), но
// с irProg.DBPoolTimeoutMs, выставленным ПОСЛЕ semantic.Analyze, ДО
// GenerateFromIR - тем же порядком, что и internal/compiler.defaultBackend.
// Generate (единственное реальное место, которое это делает вне тестов).
func generateFromSrcWithDBPoolTimeoutMs(t *testing.T, src string, timeoutMs int) string {
	t.Helper()
	return generateFromSrcWithDBTimeouts(t, src, timeoutMs, 0)
}

// generateFromSrcWithDBTimeouts - как generateFromSrcWithDBPoolTimeoutMs,
// но позволяет задать ещё и DBSQLiteBusyTimeoutMs (0 - не переопределять,
// оставить встроенное умолчание semantic.Analyze, тем же принципом, что и
// у internal/compiler.defaultBackend.Generate).
func generateFromSrcWithDBTimeouts(t *testing.T, src string, poolTimeoutMs, sqliteBusyTimeoutMs int) string {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		t.Fatalf("семантический анализ: %v", err)
	}
	irProg.DBPoolTimeoutMs = poolTimeoutMs
	if sqliteBusyTimeoutMs > 0 {
		irProg.DBSQLiteBusyTimeoutMs = sqliteBusyTimeoutMs
	}
	out, err := GenerateFromIR(irProg)
	if err != nil {
		t.Fatalf("GenerateFromIR: %v", err)
	}
	return out
}

const dbTimeoutTestSrc = `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    закрыть_бд(бд)
}
`

// TestGenerateFromIRDBPoolTimeoutMsDefaultIsZero - без "бд.таймаут_пула_мс"
// в .rechproj (internal/project.File.DBPoolTimeoutMs возвращает 0 - см. её
// package doc) макрос эмитируется как 0 - "ждать бесконечно", то же
// поведение, что и до появления этой настройки.
func TestGenerateFromIRDBPoolTimeoutMsDefaultIsZero(t *testing.T) {
	out := generateFromSrcWithDBPoolTimeoutMs(t, dbTimeoutTestSrc, 0)
	if !strings.Contains(out, "#define RECH_DB_POOL_TIMEOUT_MS 0") {
		t.Fatalf("ожидался RECH_DB_POOL_TIMEOUT_MS 0 (умолчание), получено:\n%s", out)
	}
}

// TestGenerateFromIRDBPoolTimeoutMsEmitsBoundedCheckout - "бд.
// таймаут_пула_мс" задан - макрос несёт указанное значение, а
// rech_db_checkout несёт ограниченное ожидание (pthread_cond_timedwait/
// SleepConditionVariableCS) с паникой rtmsg.DBCheckoutTimeout на таймаут -
// и на POSIX, и на Windows ветке (обе присутствуют в тексте одновременно,
// выбор - через #ifdef _WIN32 при компиляции самим cc, не Go).
func TestGenerateFromIRDBPoolTimeoutMsEmitsBoundedCheckout(t *testing.T) {
	out := generateFromSrcWithDBPoolTimeoutMs(t, dbTimeoutTestSrc, 500)
	if !strings.Contains(out, "#define RECH_DB_POOL_TIMEOUT_MS 500") {
		t.Fatalf("ожидался RECH_DB_POOL_TIMEOUT_MS 500, получено:\n%s", out)
	}
	for _, want := range []string{
		"pthread_cond_timedwait(&db->cond, &db->mutex, &rech_db_deadline)",
		"SleepConditionVariableCS(&db->cond, &db->mutex, RECH_DB_POOL_TIMEOUT_MS)",
		"clock_gettime(CLOCK_REALTIME, &rech_db_deadline);",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	wantPanic := `rech_error_as(RECH_ERR_DB, "` + escapeCString(rtmsg.DBCheckoutTimeout) + `");`
	if strings.Count(out, wantPanic) != 2 {
		t.Fatalf("ожидались РОВНО два вызова %q (Windows-ветка + POSIX-ветка), получено %d, текст:\n%s",
			wantPanic, strings.Count(out, wantPanic), out)
	}
}

// TestGenerateFromIRDBPoolTimeoutMsPanicUsesConfiguredMsgBuffer -
// rech_db_errorf использует RECH_DB_PANIC_MSG_MAX, не голое число.
func TestGenerateFromIRDBPoolTimeoutMsPanicUsesConfiguredMsgBuffer(t *testing.T) {
	out := generateFromSrcWithDBPoolTimeoutMs(t, dbTimeoutTestSrc, 0)
	if !strings.Contains(out, "#define RECH_DB_PANIC_MSG_MAX 1024") {
		t.Fatalf("ожидался #define RECH_DB_PANIC_MSG_MAX 1024, получено:\n%s", out)
	}
	if !strings.Contains(out, "char msg[RECH_DB_PANIC_MSG_MAX];") {
		t.Fatalf("ожидался char msg[RECH_DB_PANIC_MSG_MAX];, получено:\n%s", out)
	}
}

// TestGenerateFromIRDBSQLiteBusyTimeoutMsDefault - без "бд.
// sqlite_таймаут_блокировки_мс" в .rechproj (irProg.DBSQLiteBusyTimeoutMs
// остаётся тем, что поставил semantic.Analyze безусловно - см. package doc
// DefaultDBSQLiteBusyTimeoutMs) - макрос несёт 5000, sqlite3_busy_timeout
// зовётся через него, не голым числом.
func TestGenerateFromIRDBSQLiteBusyTimeoutMsDefault(t *testing.T) {
	out := generateFromSrcWithDBTimeouts(t, dbTimeoutTestSrc, 0, 0)
	if !strings.Contains(out, "#define RECH_DB_SQLITE_BUSY_TIMEOUT_MS 5000") {
		t.Fatalf("ожидался #define RECH_DB_SQLITE_BUSY_TIMEOUT_MS 5000 (умолчание), получено:\n%s", out)
	}
	if !strings.Contains(out, "sqlite3_busy_timeout(h, RECH_DB_SQLITE_BUSY_TIMEOUT_MS);") {
		t.Fatalf("ожидался sqlite3_busy_timeout(h, RECH_DB_SQLITE_BUSY_TIMEOUT_MS);, получено:\n%s", out)
	}
	if strings.Contains(out, "sqlite3_busy_timeout(h, 5000)") {
		t.Fatalf("не ожидалось голое число 5000 в вызове sqlite3_busy_timeout, получено:\n%s", out)
	}
}

// TestGenerateFromIRDBSQLiteBusyTimeoutMsConfigured - значение задано - несёт
// его, не умолчание.
func TestGenerateFromIRDBSQLiteBusyTimeoutMsConfigured(t *testing.T) {
	out := generateFromSrcWithDBTimeouts(t, dbTimeoutTestSrc, 0, 2000)
	if !strings.Contains(out, "#define RECH_DB_SQLITE_BUSY_TIMEOUT_MS 2000") {
		t.Fatalf("ожидался #define RECH_DB_SQLITE_BUSY_TIMEOUT_MS 2000, получено:\n%s", out)
	}
}
