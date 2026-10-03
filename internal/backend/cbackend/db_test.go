package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// generateFromSrc - полный конвейер лексер -> парсер -> семантика ->
// cbackend, тем же принципом, что и internal/interp.runSrc - нужен здесь,
// потому что internal/dbscheme.Set (prog.DBBackends) заполняется ТОЛЬКО
// семантическим анализом literal-аргумента "подключиться_к_бд(...)"
// (internal/semantic/expr.go, markDBConnString) - ir.Program, построенный
// вручную (как в остальных тестах этого пакета), не годится для этого
// сценария.
func generateFromSrc(t *testing.T, src string) string {
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
	out, err := GenerateFromIR(irProg)
	if err != nil {
		t.Fatalf("GenerateFromIR: %v", err)
	}
	return out
}

// TestGenerateFromIRDBOnlyEmitsUsedBackend - internal/dbscheme.Set
// ("toolchain линкует -lsqlite3 -lpq -lmysqlclient
// безусловно") - программа, подключающаяся ТОЛЬКО по literal-строке
// "sqlite:..." не должна нести в сгенерированном C ни единого упоминания
// PostgreSQL/MySQL (иначе линковка требовала бы их клиентские библиотеки
// тоже, даже когда они не могут понадобиться - см. internal/toolchain,
// commonLinkFlags).
func TestGenerateFromIRDBOnlyEmitsUsedBackend(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    закрыть_бд(бд)
}
`)
	if !strings.Contains(out, "sqlite3_open(") {
		t.Fatalf("ожидался sqlite3_open( (SQLite используется), получено:\n%s", out)
	}
	// "RECH_DB_MYSQL"/"RECH_DB_PG" (значения enum "rech_db_kind") эмитируются
	// БЕЗУСЛОВНО для любой программы с БД (db->kind сравнивается с ними
	// независимо от того, какие СУБД реально скомпилированы, см. package
	// doc dbConnectCloseCode) - не входят в проверку ниже, только настоящие
	// функции/типы клиентских библиотек PostgreSQL/MySQL.
	for _, absent := range []string{"PQconnectdb(", "mysql_init(", "PGconn", "typedef struct MYSQL "} {
		if strings.Contains(out, absent) {
			t.Fatalf("не ожидалось упоминание %q (только SQLite используется), получено:\n%s", absent, out)
		}
	}
}

// TestGenerateFromIRDBConnectCloseOnlyStillEmitsParamsListType - найденный
// (не мой) баг: "rech_db_substitute"/"rech_db_query"/"rech_db_exec"
// (db.go) безусловно эмитируются, как только UsesDB, и все ссылаются на
// C-typedef "Список<Значение>" (параметры запроса) - но раньше он
// принудительно регистрировался (registerDBTypesOnCall, internal/
// semantic/db.go) только КАК ЧАСТЬ обработки любого из четырёх вызовов, но
// без самого typedef'а: только dbResultType ("Список<Словарь<Строка,
// Значение>>"), не listOfAnyType ("Список<Значение>" - параметры) - у
// программы, вызвавшей только "подключиться_к_бд"/"закрыть_бд" (без
// единого "запросить"/"выполнить" и без буквального "новый
// Список<Значение>()"), генерировался код с необъявленным
// "rech_T_spisok_of_znachenie" ("unknown type name" у настоящего cc).
func TestGenerateFromIRDBConnectCloseOnlyStillEmitsParamsListType(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    закрыть_бд(бд)
}
`)
	if !strings.Contains(out, "rech_T_spisok_of_znachenie") {
		t.Fatalf("ожидался typedef параметров запроса (Список<Значение>) даже без \"запросить\"/\"выполнить\", получено:\n%s", out)
	}
}

// TestGenerateFromIRDBDynamicConnStringEmitsAllBackends - строка
// подключения НЕ литерал (параметр функции) - какая именно СУБД будет
// выбрана, решится только в рантайме, поэтому internal/dbscheme.Set.Dynamic
// взводится, и cbackend обязан эмитировать все три (безопасный откат к
// прежнему безусловному поведению - см. package doc dbscheme.Set).
func TestGenerateFromIRDBDynamicConnStringEmitsAllBackends(t *testing.T) {
	out := generateFromSrc(t, `
функция подключиться(строка_подключения: Строка): БД
{
    вернуть подключиться_к_бд(строка_подключения)
}

функция главная
{
    переменная бд = подключиться("sqlite::memory:")
    закрыть_бд(бд)
}
`)
	for _, want := range []string{"sqlite3_open(", "PQconnectdb(", "mysql_init("} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось упоминание %q (динамическая строка подключения - безопасный откат ко всем трём), получено:\n%s", want, out)
		}
	}
}

// TestGenerateFromIRWithoutDBOmitsAllBackends - программа без единого
// использования БД не должна нести ничего из её рантайма вообще (тот же
// принцип, что и у остальных builtin-флагов пакета).
func TestGenerateFromIRWithoutDBOmitsAllBackends(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    вывести 1
}
`)
	for _, absent := range []string{"sqlite3_open(", "PQconnectdb(", "mysql_init(", "rech_db_connect(", "rech_db_pool_state("} {
		if strings.Contains(out, absent) {
			t.Fatalf("не ожидалось упоминание %q (БД не используется), получено:\n%s", absent, out)
		}
	}
}

// TestGenerateFromIRDBHandleMethodSugarGeneratesSameCallsAsFreeFunctions -
// "бд.запросить(...)"/"бд.выполнить(...)"/"бд.состояние_пула()"/"бд.закрыть()"
// (internal/semantic/handle_methods.go) - ЧИСТЫЙ семантический сахар, ноль
// нового кода в cbackend - сгенерированный C должен звать РОВНО те же
// функции рантайма, что и запись через свободные функции.
func TestGenerateFromIRDBHandleMethodSugarGeneratesSameCallsAsFreeFunctions(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    бд.выполнить("create table t(id integer)")
    переменная строки = бд.запросить("select id from t")
    вывести строки.длина()
    переменная сост = бд.состояние_пула()
    вывести сост.всего
    бд.закрыть()
}
`)
	for _, want := range []string{"rech_db_exec(", "rech_db_query(", "rech_db_pool_state(", "rech_db_close("} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался вызов %q через метод-сахар, получено:\n%s", want, out)
		}
	}
}

// numeric (PostgreSQL) / DECIMAL (MySQL) - Десятичное из текстовой записи
// (rech_db_numeric), а не double через strtod; у программы только с SQLite
// этого хелпера нет.
func TestGenerateFromIRDBNumericColumnsAreDecimal(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная бд = подключиться_к_бд("postgres://localhost/db")
    закрыть_бд(бд)
}
`)
	for _, want := range []string{"static rech_any_t rech_db_numeric(", "} else if (oid == 1700) {\n                    v = rech_db_numeric(text);"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидалось %q, получено:\n%s", want, out)
		}
	}
	sqliteOnly := generateFromSrc(t, `
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    закрыть_бд(бд)
}
`)
	if strings.Contains(sqliteOnly, "rech_db_numeric") {
		t.Fatal("у программы только с SQLite rech_db_numeric не нужен")
	}
}
