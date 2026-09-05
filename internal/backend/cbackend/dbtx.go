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
// (rech_db_t, см. package doc выше) с единственным уже занятым слотом;
// real_db/real_idx - откуда это соединение взято НА САМОМ ДЕЛЕ, чтобы
// вернуть его в НАСТОЯЩИЙ пул по завершении (rech_db_checkin); resolved -
// транзакция уже зафиксирована/откачена (явным "откатить()" или
// автоматически) - не позволяет откатить/зафиксировать её дважды.
func dbtxTypedefCode() string {
	return fmt.Sprintf(`typedef struct {
    rech_db_t mini;
    rech_db_t *real_db;
    int real_idx;
    bool resolved;
} %s;

`, mangleType(transactionEntityName))
}

// dbRunTransactionFunc - C-имя функции, которую эмитит builtin-вызов
// "начать_транзакцию" (см. builtinCallCode, builtins_shared.go).
const dbRunTransactionFunc = "rech_db_run_transaction"

// dbtxPrototypes - прототипы трёх методов "Транзакция" и самой
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
		"static %[1]s *%[2]s(%[3]s *this_, const char *sql, %[4]s *params);\nstatic long long %[5]s(%[3]s *this_, const char *sql, %[4]s *params);\nstatic void %[6]s(%[3]s *this_);\nstatic void %[7]s(rech_db_t *db, %[8]s callback);\n",
		resultListC,
		mangleMethod(transactionEntityName, "запросить"),
		txC,
		paramsListC,
		mangleMethod(transactionEntityName, "выполнить"),
		mangleMethod(transactionEntityName, "откатить"),
		dbRunTransactionFunc,
		callableC,
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

// dbtxRuntimeCode - "начать_транзакцию" + все три метода "Транзакция" -
// эмитится безусловно, ровно один раз, только если ctx.usesDBTransaction.
//
// rech_db_run_transaction - открывает транзакцию (checkout РЕАЛЬНОГО
// соединения + "BEGIN"), исполняет колбэк ПОД ДВУМЯ вложенными кадрами
// перехвата (тот же catchFrameType/catchTopVar, что использует и
// rech_throw, и rech_panic - каждый ищет в нём свой маркер, пропуская чужие
// кадры, exceptions.go): внешний - catch_panics (ловит панику, в т.ч. от
// самих "запросить"/"выполнить" внутри колбэка), внутренний - обычный кадр
// без диспетчеризации по типу (тип не проверяется - единственная цель
// кадра поймать ЛЮБОЕ "выбросить", раз всё наследует "Исключение", см. её
// package doc). Оба при срабатывании: откатывают (если ещё не откачено
// явно "откатить()"), возвращают соединение в реальный пул, пробрасывают
// исходную ошибку тем же способом дальше. Штатное завершение колбэка (ни
// один из двух кадров не сработал) - фиксирует (если ещё не откачено
// явно) и тоже возвращает соединение в пул.
func dbtxRuntimeCode(ctx *irGenCtx) string {
	resultListC := mangleType(dbResultType)
	paramsListC := reflectAnyListCType()
	callableC := mangleCallableType(transactionCallableSig().Entity)
	txC := mangleType(transactionEntityName)
	zaprosit := mangleMethod(transactionEntityName, "запросить")
	vypolnit := mangleMethod(transactionEntityName, "выполнить")
	otkatit := mangleMethod(transactionEntityName, "откатить")

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
    tx->resolved = false;
    rech_tx_run_simple(tx->mini.kind, conn, "BEGIN");
    return tx;
}

static %[2]s *%[3]s(%[1]s *this_, const char *sql, %[4]s *params) {
    return rech_db_query(&this_->mini, sql, params);
}

static long long %[5]s(%[1]s *this_, const char *sql, %[4]s *params) {
    return rech_db_exec(&this_->mini, sql, params);
}

static void %[6]s(%[1]s *this_) {
    if (!this_->resolved) {
        rech_tx_run_simple(this_->mini.kind, this_->mini.conns[0], "ROLLBACK");
        this_->resolved = true;
    }
}

`, txC, resultListC, zaprosit, paramsListC, vypolnit, otkatit)

	fmt.Fprintf(&sb, `static void %[1]s(rech_db_t *db, %[2]s callback) {
    %[3]s *tx = rech_db_begin_transaction(db);
    %[4]s frame_panic = {0};
    frame_panic.prev = %[5]s;
    frame_panic.catch_panics = true;
    %[5]s = &frame_panic;
    if (setjmp(frame_panic.buf) == 0) {
        %[4]s frame_exc = {0};
        frame_exc.prev = %[5]s;
        %[5]s = &frame_exc;
        if (setjmp(frame_exc.buf) == 0) {
            callback.fn(callback.env, tx);
            %[5]s = frame_exc.prev;
        } else {
            %[5]s = frame_exc.prev;
            void *exc = frame_exc.exc;
            if (!tx->resolved) {
                rech_tx_run_simple(tx->mini.kind, tx->mini.conns[0], "ROLLBACK");
                tx->resolved = true;
            }
            rech_db_checkin(db, tx->real_idx);
            %[6]s(exc, %[7]s);
        }
        %[5]s = frame_panic.prev;
    } else {
        %[5]s = frame_panic.prev;
        const char *msg = frame_panic.panic_msg;
        if (!tx->resolved) {
            rech_tx_run_simple(tx->mini.kind, tx->mini.conns[0], "ROLLBACK");
            tx->resolved = true;
        }
        rech_db_checkin(db, tx->real_idx);
        rech_panic(msg);
    }
    if (!tx->resolved) {
        rech_tx_run_simple(tx->mini.kind, tx->mini.conns[0], "COMMIT");
        tx->resolved = true;
    }
    rech_db_checkin(db, tx->real_idx);
}

`, dbRunTransactionFunc, callableC, txC, catchFrameType, catchTopVar, throwFunc,
		throwTypeNameExpr(ctx, "exc", systemExceptionEntity))

	return sb.String()
}
