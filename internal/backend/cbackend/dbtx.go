// Транзакции БД ("начать_транзакцию", callback-стиль - см. package doc
// internal/semantic/dbtx.go для полного описания примитива) - реализованы
// БЕЗ единой строчки изменений в уже существующих rech_db_query/
// rech_db_exec/rech_db_checkout/rech_db_checkin (db.go): "Транзакция"
// несёт СОБСТВЕННЫЙ "мини-пул" - rech_db_t с pool_size == 1, единственный
// слот которого - уже взятое из РЕАЛЬНОГО пула соединение. "тр.запросить"/
// "тр.выполнить" зовут rech_db_query/rech_db_exec НАД ЭТИМ мини-пулом как
// есть (checkout/checkin внутри них видят единственный слот, конкуренции
// нет - весь колбэк исполняется одним потоком синхронно) - та же
// диспетчеризация по СУБД (rech_db_sqlite_query/rech_db_pg_query/
// rech_db_mysql_query), тот же rech_db_substitute, без единого дубля их
// логики.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/dbscheme"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

const transactionEntityName = "Транзакция"

// transactionCallableSig - каноническая сигнатура "Вызываемый(Транзакция):
// Ничто", единственная форма колбэка "начать_транзакцию" - вычисляется тем
// же способом, что и в internal/semantic/dbtx.go (registerTransactionFuncs),
// оба места обязаны сходиться на одной и той же строке (mangleCallableType
// требует ИМЕННО тот canonical-текст, что реально попал в prog.Callables).
func transactionCallableSig() ir.Type {
	return ir.CallableType([]ir.Type{ir.EntityType(transactionEntityName)}, nil)
}

// dbtxTypedefCode - C-структура "Транзакция": mini - собственный "мини-пул"
// (rech_db_t, см. package doc выше) с единственным уже занятым слотом -
// заполнен только у корневой транзакции; conn_db - мини-пул, через который
// идут запросы (&mini у корневой, conn_db родителя у вложенной - одно и то
// же соединение на всю цепочку); real_db/real_idx - откуда соединение взято
// НА САМОМ ДЕЛЕ, чтобы вернуть его в НАСТОЯЩИЙ пул по завершении корневой
// (rech_db_checkin); resolved - транзакция уже зафиксирована/откачена (явным
// "откатить()" или автоматически) - не позволяет откатить/зафиксировать её
// дважды; parent/depth/savepoint - только у вложенной ("тр.вложенная"):
// родитель, глубина вложенности и имя SAVEPOINT.
func dbtxTypedefCode() string {
	return fmt.Sprintf(`typedef struct %[1]s_s {
    rech_db_t mini;
    rech_db_t *conn_db;
    rech_db_t *real_db;
    int real_idx;
    bool resolved;
    struct %[1]s_s *parent;
    int depth;
    char savepoint[32];
} %[1]s;

`, mangleType(transactionEntityName))
}

// dbRunTransactionFunc - C-имя функции, которую эмитит builtin-вызов
// "начать_транзакцию" (см. builtinCallCode, builtins_shared.go).
const dbRunTransactionFunc = "rech_db_run_transaction"

// dbtxPrototypes - прототипы четырёх методов "Транзакция" и самой
// rech_db_run_transaction - эмитятся в общий "protos" builder
// (ir_generator.go), тем же принципом, что и mutexPrototypes/
// semaphorePrototypes (могут понадобиться раньше их определения по тексту
// файла).
func dbtxPrototypes() string {
	resultListC := mangleType(dbResultType)
	paramsListC := reflectAnyListCType()
	txC := mangleType(transactionEntityName)
	callableC := mangleCallableType(transactionCallableSig().Entity)
	return fmt.Sprintf(
		"static %[1]s *%[2]s(%[3]s *this_, const char *sql, %[4]s *params);\nstatic long long %[5]s(%[3]s *this_, const char *sql, %[4]s *params);\nstatic void %[6]s(%[3]s *this_);\nstatic void %[9]s(%[3]s *this_, %[8]s callback);\nstatic void %[7]s(rech_db_t *db, %[8]s callback);\n",
		resultListC,
		mangleMethod(transactionEntityName, "запросить"),
		txC,
		paramsListC,
		mangleMethod(transactionEntityName, "выполнить"),
		mangleMethod(transactionEntityName, "откатить"),
		dbRunTransactionFunc,
		callableC,
		mangleMethod(transactionEntityName, "вложенная"),
	)
}

// dbtxRunSimpleCode - "rech_tx_run_simple" - выполняет ОДИН SQL-стейтмент
// без параметров и без чтения результата (BEGIN/COMMIT/ROLLBACK) на уже
// данном (kind, conn) - НЕ через rech_db_query/rech_db_exec (тем нужен
// db-уровня checkout, здесь соединение уже и так эксклюзивно у нас) и не
// дублирует их логику разбора результата - результат здесь не нужен вовсе.
func dbtxRunSimpleCode(backends dbscheme.Set) string {
	var sb strings.Builder
	sb.WriteString("static void rech_tx_run_simple(rech_db_kind kind, void *conn, const char *sql) {\n    switch (kind) {\n")
	if backends.NeedsSQLite() {
		fmt.Fprintf(&sb, `    case RECH_DB_SQLITE: {
        char *errmsg = NULL;
        if (sqlite3_exec(conn, sql, NULL, NULL, &errmsg) != 0) {
            rech_panic("%s");
        }
        break;
    }
`, escapeCString(rtmsg.DBQueryFailedTemplate))
	}
	if backends.NeedsPostgres() {
		fmt.Fprintf(&sb, `    case RECH_DB_PG: {
        PGresult *res = PQexec(conn, sql);
        int status = PQresultStatus(res);
        PQclear(res);
        if (status != 1 && status != 2) {
            rech_panic("%s");
        }
        break;
    }
`, escapeCString(rtmsg.DBQueryFailedTemplate))
	}
	if backends.NeedsMySQL() {
		fmt.Fprintf(&sb, `    case RECH_DB_MYSQL: {
        if (mysql_real_query(conn, sql, (unsigned long)strlen(sql)) != 0) {
            rech_panic("%s");
        }
        break;
    }
`, escapeCString(rtmsg.DBQueryFailedTemplate))
	}
	fmt.Fprintf(&sb, "    default:\n        rech_panic(\"%s\");\n    }\n}\n\n", escapeCString(rtmsg.DBUnreachableKind))
	return sb.String()
}

// dbtxRuntimeCode - "начать_транзакцию" + все четыре метода "Транзакция" -
// эмитится безусловно, ровно один раз, только если ctx.usesDBTransaction.
//
// rech_tx_run - общее тело корневой ("начать_транзакцию") и вложенной
// ("тр.вложенная") транзакции: исполняет колбэк под кадром перехвата
// (catchFrameType/catchTopVar, exceptions.go), который ловит ЛЮБОЕ
// брошенное значение - исключение или панику, в т.ч. от самих
// "запросить"/"выполнить" внутри колбэка. При срабатывании: откатывает
// (rech_tx_finish -
// ROLLBACK у корневой, ROLLBACK TO SAVEPOINT у вложенной; ничего, если уже
// откачено явно "откатить()"), возвращает соединение в реальный пул (только
// корневая), пробрасывает исходное значение дальше тем же rech_throw. Штатное
// завершение колбэка - фиксирует (COMMIT/RELEASE SAVEPOINT) и тоже
// возвращает соединение в пул.
func dbtxRuntimeCode(ctx *irGenCtx) string {
	resultListC := mangleType(dbResultType)
	paramsListC := reflectAnyListCType()
	callableC := mangleCallableType(transactionCallableSig().Entity)
	txC := mangleType(transactionEntityName)
	zaprosit := mangleMethod(transactionEntityName, "запросить")
	vypolnit := mangleMethod(transactionEntityName, "выполнить")
	otkatit := mangleMethod(transactionEntityName, "откатить")
	vlozhennaya := mangleMethod(transactionEntityName, "вложенная")

	var sb strings.Builder
	fmt.Fprintf(&sb, `static %[1]s *rech_db_begin_transaction(rech_db_t *db) {
    %[1]s *tx = GC_MALLOC(sizeof(%[1]s));
    tx->real_db = db;
    void *conn = rech_db_checkout(db, &tx->real_idx);
    tx->mini.kind = db->kind;
    tx->mini.pool_size = 1;
    tx->mini.conns[0] = conn;
    tx->mini.in_use[0] = 0;
#ifdef _WIN32
    InitializeCriticalSection(&tx->mini.mutex);
    InitializeConditionVariable(&tx->mini.cond);
#else
    pthread_mutex_init(&tx->mini.mutex, NULL);
    pthread_cond_init(&tx->mini.cond, NULL);
#endif
    tx->conn_db = &tx->mini;
    tx->resolved = false;
    tx->parent = NULL;
    tx->depth = 0;
    rech_tx_run_simple(tx->mini.kind, conn, "BEGIN");
    return tx;
}

static bool rech_tx_alive(%[1]s *tx) {
    for (; tx != NULL; tx = tx->parent) {
        if (tx->resolved) {
            return false;
        }
    }
    return true;
}

static void rech_tx_finish(%[1]s *tx, bool commit) {
    bool alive = rech_tx_alive(tx);
    tx->resolved = true;
    if (!alive) {
        return;
    }
    rech_db_kind kind = tx->conn_db->kind;
    void *conn = tx->conn_db->conns[0];
    if (tx->parent == NULL) {
        rech_tx_run_simple(kind, conn, commit ? "COMMIT" : "ROLLBACK");
        return;
    }
    char sql[64];
    if (!commit) {
        snprintf(sql, sizeof(sql), "ROLLBACK TO SAVEPOINT %%s", tx->savepoint);
        rech_tx_run_simple(kind, conn, sql);
    }
    snprintf(sql, sizeof(sql), "RELEASE SAVEPOINT %%s", tx->savepoint);
    rech_tx_run_simple(kind, conn, sql);
}

static void rech_tx_release_conn(%[1]s *tx) {
    if (tx->parent == NULL) {
        rech_db_checkin(tx->real_db, tx->real_idx);
    }
}

static %[2]s *%[3]s(%[1]s *this_, const char *sql, %[4]s *params) {
    return rech_db_query(this_->conn_db, sql, params);
}

static long long %[5]s(%[1]s *this_, const char *sql, %[4]s *params) {
    return rech_db_exec(this_->conn_db, sql, params);
}

static void %[6]s(%[1]s *this_) {
    rech_tx_finish(this_, false);
}

`, txC, resultListC, zaprosit, paramsListC, vypolnit, otkatit)

	fmt.Fprintf(&sb, `static void rech_tx_run(%[3]s *tx, %[2]s callback) {
    %[4]s frame = {0};
    frame.prev = %[5]s;
    %[5]s = &frame;
    if (setjmp(frame.buf) == 0) {
        callback.fn(callback.env, tx);
        %[5]s = frame.prev;
    } else {
        %[5]s = frame.prev;
        void *exc = frame.exc;
        rech_tx_finish(tx, false);
        rech_tx_release_conn(tx);
        %[6]s(exc, %[7]s);
    }
    rech_tx_finish(tx, true);
    rech_tx_release_conn(tx);
}

static void %[1]s(rech_db_t *db, %[2]s callback) {
    rech_tx_run(rech_db_begin_transaction(db), callback);
}

static void %[8]s(%[3]s *this_, %[2]s callback) {
    if (!rech_tx_alive(this_)) {
        rech_panic("%[9]s");
    }
    %[3]s *tx = GC_MALLOC(sizeof(%[3]s));
    tx->conn_db = this_->conn_db;
    tx->real_db = this_->real_db;
    tx->real_idx = this_->real_idx;
    tx->resolved = false;
    tx->parent = this_;
    tx->depth = this_->depth + 1;
    snprintf(tx->savepoint, sizeof(tx->savepoint), "rech_sp_%%d", tx->depth);
    char sql[64];
    snprintf(sql, sizeof(sql), "SAVEPOINT %%s", tx->savepoint);
    rech_tx_run_simple(tx->conn_db->kind, tx->conn_db->conns[0], sql);
    rech_tx_run(tx, callback);
}

`, dbRunTransactionFunc, callableC, txC, catchFrameType, catchTopVar, throwFunc,
		thrownTypeNameExpr("exc"), vlozhennaya, escapeCString(rtmsg.DBTxResolved))

	return sb.String()
}
