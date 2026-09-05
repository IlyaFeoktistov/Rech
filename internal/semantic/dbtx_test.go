package semantic

import (
	"strings"
	"testing"
)

const dbtxTestSrc = `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    начать_транзакцию(бд, функция(тр: Транзакция)
    {
        переменная параметры = новый Список<Значение>()
        параметры.добавить((Значение) 1)
        тр.выполнить("INSERT INTO t (id) VALUES (?)", параметры)
        переменная строки = тр.запросить("SELECT id FROM t")
        вывести строки.длина()
        тр.откатить()
    })
    закрыть_бд(бд)
}
`

// TestAnalyzeDBTransactionBuildsIRAndRegistersCallable - "начать_транзакцию"
// проходит проверку типов (лямбда с параметром "Транзакция" против
// ожидаемого ir.CallableType), "Транзакция" появляется в IR с IsTransaction,
// и её каноническая сигнатура "Вызываемый(Транзакция)" реально
// зарегистрирована в Program.Callables (без этого cbackend не эмитировал
// бы C-typedef колбэка вовсе).
func TestAnalyzeDBTransactionBuildsIRAndRegistersCallable(t *testing.T) {
	irProg := parseProgram(t, dbtxTestSrc)
	found := false
	for _, ent := range irProg.Entities {
		if ent.Name == "Транзакция" && ent.IsTransaction {
			found = true
		}
	}
	if !found {
		t.Fatalf("ожидалась сущность \"Транзакция\" (IsTransaction) в IR, получено %+v", irProg.Entities)
	}
	if len(irProg.Callables) == 0 {
		t.Fatalf("ожидалась зарегистрированная сигнатура Вызываемый(Транзакция) в Program.Callables")
	}
}

// TestAnalyzeNewTransactionRejected - "Транзакция" получить можно ТОЛЬКО
// параметром колбэка "начать_транзакцию" - "новая Транзакция()" (форма
// согласована по роду) должна быть явной ошибкой компиляции, не молчаливо
// разрешённым, но бессмысленным конструктором.
func TestAnalyzeNewTransactionRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная
{
    переменная х = новая Транзакция()
}
`)
	if err == nil || !strings.Contains(err.Error(), "новый") {
		t.Fatalf("ожидалась ошибка про запрет \"новый\" для Транзакция, получено: %v", err)
	}
}

// TestAnalyzeUserEntityCannotBeNamedTransaction - "Транзакция"
// зарезервировано (internal/builtins.ReservedEntityNames), как и
// "Блокировка"/"Семафор" - нельзя объявить свою сущность с этим именем.
func TestAnalyzeUserEntityCannotBeNamedTransaction(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Транзакция
{
    имеет:
        х: Целое
}
функция главная { вывести 1 }
`)
	if err == nil || !strings.Contains(err.Error(), "зарезервированное имя") {
		t.Fatalf("ожидалась ошибка - \"Транзакция\" зарезервированное имя, получено: %v", err)
	}
}
