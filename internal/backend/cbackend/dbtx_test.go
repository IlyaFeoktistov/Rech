package cbackend

import (
	"strings"
	"testing"
)

const dbtxCbackendTestSrc = `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    начать_транзакцию(бд, функция(тр: Транзакция)
    {
        переменная параметры = новый Список<Значение>()
        параметры.добавить((Значение) 1)
        тр.выполнить("INSERT INTO t (id) VALUES (?)", параметры)
        тр.откатить()
    })
    закрыть_бд(бд)
}
`

// TestGenerateFromIRDBTransactionEmitsRuntimeAndTwoCatchFrames -
// "начать_транзакцию" эмитит rech_db_run_transaction с ДВУМЯ вложенными
// кадрами перехвата (catch_panics - для "Паника", обычный - для
// "Исключение", см. package doc dbtx.go) вокруг вызова колбэка, и все три
// метода "Транзакция" (запросить/выполнить/откатить), работающие через
// её собственный "мини-пул" (this_->mini), не напрямую rech_db_checkout.
func TestGenerateFromIRDBTransactionEmitsRuntimeAndTwoCatchFrames(t *testing.T) {
	out := generateFromSrc(t, dbtxCbackendTestSrc)
	for _, want := range []string{
		"typedef struct {\n    rech_db_t mini;",
		"static void rech_db_run_transaction(rech_db_t *db,",
		"frame_panic.catch_panics = true;",
		"callback.fn(callback.env, tx);",
		"rech_tx_run_simple(tx->mini.kind, conn, \"BEGIN\");",
		"rech_tx_run_simple(tx->mini.kind, tx->mini.conns[0], \"COMMIT\");",
		"rech_tx_run_simple(tx->mini.kind, tx->mini.conns[0], \"ROLLBACK\");",
		"return rech_db_query(&this_->mini, sql, params);",
		"return rech_db_exec(&this_->mini, sql, params);",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался фрагмент %q, получено:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "rech_M_tranzakciya_otkatit") {
		t.Fatalf("ожидался метод \"откатить\" (rech_M_tranzakciya_otkatit), получено:\n%s", out)
	}
}

// TestGenerateFromIRWithoutDBTransactionOmitsItsRuntime - программа с
// обычной "БД" (без единого "начать_транзакцию") не должна тянуть за собой
// rech_db_run_transaction/"Транзакция" вовсе.
func TestGenerateFromIRWithoutDBTransactionOmitsItsRuntime(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    закрыть_бд(бд)
}
`)
	for _, unwanted := range []string{"rech_db_run_transaction", "Транзакция", "rech_T_tranzakciya", "rech_tx_run_simple"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("программа без \"начать_транзакцию\" не должна содержать %q, получено:\n%s", unwanted, out)
		}
	}
}
