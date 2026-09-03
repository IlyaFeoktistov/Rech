// "подключиться_к_бд"/"запросить"/"выполнить"/"закрыть_бд" - единый API
// поверх SQLite/PostgreSQL/MySQL (см. package doc internal/semantic/db.go)
// - реализовано через прямую динамическую линковку с настоящими
// клиентскими библиотеками (libsqlite3/libpq/libmysqlclient), не через
// "внешняя функция" (FFI, раздел 12 SPEC.md): та ограничена скалярными
// параметрами/результатом и не может выразить out-параметр
// "sqlite3_open(const char*, sqlite3**)" - здесь это просто ещё один
// hand-written C runtime, как httpHelpers/jsonParserCode, со своими
// extern-объявлениями нужных функций библиотек (компилятор не включает
// настоящие заголовки - их и нет на большинстве систем без отдельного
// установленного "-dev"-пакета, только сами .so).
//
// Структуры MYSQL/PGconn/PGresult/sqlite3/sqlite3_stmt объявлены здесь как
// НЕПОЛНЫЕ типы (forward declaration без тела) - этого достаточно для
// работы только через указатель на них (ни разу не разыменовывается),
// даже там, где настоящий заголовок библиотеки определяет полную
// структуру (MYSQL) - C не требует полного определения типа для указателя
// на него. Единственное исключение - rechMySQLFieldStruct (см. её
// package doc): её реальные поля читаются напрямую (`->name`, `->type`),
// поэтому объявлены здесь byte-в-byte как в настоящем mysql.h.
//
// Параметры запроса подставляются в текст SQL как литералы САМИМ этим
// рантаймом (rech_db_substitute), не через нативное параметризованное
// связывание каждой библиотеки - тем же принципом простоты, что и у
// остального рантайма cbackend (собственный код вместо трёх разных
// протоколов подготовленных выражений), при этом безопасно против
// SQL-инъекций: строковые значения экранируются (одинарная кавычка
// удваивается), числа/логические/NULL печатаются напрямую, не как
// пользовательский текст.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/dbscheme"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// dbRowType/dbResultType - те же канонические имена generic-инстанциаций,
// что строит internal/semantic/db.go (dbResultType) - typedef/методы уже
// эмитированы общим циклом по prog.Generics раньше этого блока (см. место
// вызова dbRuntimeCode в generateSource).
const (
	dbRowType    = "Словарь<Строка, Значение>"
	dbResultType = "Список<Словарь<Строка, Значение>>"
)

// sqliteOpaqueTypedefs/pgOpaqueTypedefs/mysqlOpaqueTypedefs - неполные
// (opaque) объявления типов трёх клиентских библиотек - см. package doc.
// Три отдельные константы, не одна - каждая эмитируется (dbRuntimeCode)
// только когда internal/dbscheme.Set реально требует эту конкретную СУБД,
// иначе программа, использующая только SQLite, не должна нести (и уж тем
// более линковать) ничего, упоминающего PostgreSQL/MySQL - docs/ROADMAP.md,
// "toolchain линкует -lsqlite3 -lpq -lmysqlclient безусловно".
const sqliteOpaqueTypedefs = `typedef struct sqlite3 sqlite3;
typedef struct sqlite3_stmt sqlite3_stmt;

`

const pgOpaqueTypedefs = `typedef struct pg_conn PGconn;
typedef struct pg_result PGresult;

`

const mysqlOpaqueTypedefs = `typedef struct MYSQL MYSQL;
typedef struct MYSQL_RES MYSQL_RES;
typedef char **rech_mysql_row_t;

`

// rechMySQLFieldStruct - MYSQL_FIELD (mysql.h) byte-в-byte, до поля "type"
// включительно (все поля до него нужны настоящей библиотеке для
// вычисления правильного смещения "type" в её собственной памяти - порядок
// и типы должны совпадать с реальным заголовком). Только "name" и "type"
// читаются этим рантаймом (имя колонки, классификация
// Целое/Действительное/Строка). Названо не "MYSQL_FIELD" (настоящее имя в
// mysql.h) - реальный заголовок нигде не подключается, совпадение имён не
// требуется, только совпадение памяти по указателю.
const rechMySQLFieldStruct = `typedef struct {
    char *name;
    char *org_name;
    char *table;
    char *org_table;
    char *db;
    char *catalog;
    char *def;
    unsigned long length;
    unsigned long max_length;
    unsigned int name_length;
    unsigned int org_name_length;
    unsigned int table_length;
    unsigned int org_table_length;
    unsigned int db_length;
    unsigned int catalog_length;
    unsigned int def_length;
    unsigned int flags;
    unsigned int decimals;
    unsigned int charsetnr;
    unsigned int type;
} rech_mysql_field_t;

`

// sqliteExternDecls/pgExternDecls/mysqlExternDecls - прототипы функций
// трёх библиотек, реально используемых этим рантаймом - подмножество
// настоящих заголовков (sqlite3.h, libpq-fe.h, mysql.h), byte-в-byte по
// сигнатуре (типы параметров/результата), достаточное для честной линковки
// с настоящими .so (см. package doc). Числовые константы (SQLITE_ROW и
// т. п.) - часть публичного, стабильного C ABI каждой библиотеки,
// определены прямо в коде-потребителе (sqliteQueryExecCode и т. д.), не
// здесь. Три отдельные константы (не одна) - той же причиной, что и у
// *OpaqueTypedefs выше: каждая эмитируется, только когда её СУБД реально
// нужна (internal/dbscheme.Set).
const sqliteExternDecls = `extern int sqlite3_open(const char *filename, sqlite3 **db);
extern int sqlite3_close(sqlite3 *db);
extern int sqlite3_exec(sqlite3 *db, const char *sql, int (*cb)(void *, int, char **, char **), void *arg, char **errmsg);
extern int sqlite3_prepare_v2(sqlite3 *db, const char *sql, int nbyte, sqlite3_stmt **stmt, const char **tail);
extern int sqlite3_step(sqlite3_stmt *stmt);
extern int sqlite3_finalize(sqlite3_stmt *stmt);
extern int sqlite3_column_count(sqlite3_stmt *stmt);
extern const char *sqlite3_column_name(sqlite3_stmt *stmt, int col);
extern int sqlite3_column_type(sqlite3_stmt *stmt, int col);
extern long long sqlite3_column_int64(sqlite3_stmt *stmt, int col);
extern double sqlite3_column_double(sqlite3_stmt *stmt, int col);
extern const unsigned char *sqlite3_column_text(sqlite3_stmt *stmt, int col);
extern int sqlite3_changes(sqlite3 *db);

`

const pgExternDecls = `extern PGconn *PQconnectdb(const char *conninfo);
extern int PQstatus(const PGconn *conn);
extern void PQfinish(PGconn *conn);
extern PGresult *PQexec(PGconn *conn, const char *query);
extern int PQresultStatus(const PGresult *res);
extern int PQntuples(const PGresult *res);
extern int PQnfields(const PGresult *res);
extern char *PQfname(const PGresult *res, int field_num);
extern char *PQgetvalue(const PGresult *res, int tup_num, int field_num);
extern int PQgetisnull(const PGresult *res, int tup_num, int field_num);
extern void PQclear(PGresult *res);
extern unsigned int PQftype(const PGresult *res, int field_num);
extern char *PQcmdTuples(PGresult *res);

`

const mysqlExternDecls = `extern MYSQL *mysql_init(MYSQL *mysql);
extern MYSQL *mysql_real_connect(MYSQL *mysql, const char *host, const char *user, const char *passwd, const char *db, unsigned int port, const char *unix_socket, unsigned long clientflag);
extern void mysql_close(MYSQL *sock);
extern int mysql_real_query(MYSQL *mysql, const char *q, unsigned long length);
extern MYSQL_RES *mysql_store_result(MYSQL *mysql);
extern void mysql_free_result(MYSQL_RES *result);
extern rech_mysql_row_t mysql_fetch_row(MYSQL_RES *result);
extern unsigned int mysql_num_fields(MYSQL_RES *res);
extern rech_mysql_field_t *mysql_fetch_field(MYSQL_RES *result);
extern unsigned long long mysql_affected_rows(MYSQL *mysql);

`

// rechDBStruct - дескриптор соединения "БД" (ir.KindDB) - один и тот же
// C-тип для всех трёх СУБД, kind решает, какое из трёх полей handle
// валидно (ровно одно) - решается один раз, в "подключиться_к_бд", дальше
// каждая функция просто читает db->kind. Поля - "void *", НЕ типизированные
// "sqlite3 *"/"PGconn *"/"MYSQL *" - этот typedef эмитируется БЕЗУСЛОВНО и
// РАНЬШЕ (dbHandleTypedefCode, до genericsCode/typedefs - см. её package
// doc) для ЛЮБОЙ программы, использующей "БД" (в том числе как поле
// сущности), а opaque-типы трёх библиотек эмитируются условно и ПОЗЖЕ,
// только для реально нужных СУБД (см. dbRuntimeCode/internal/dbscheme) - на
// этом, более раннем месте они бы ещё не существовали. "void *" не требует
// приведения типа ни при записи (rech_db_connect), ни при чтении
// (rech_db_sqlite_query и т. п. - тот код возникает уже ПОСЛЕ нужных
// opaque-типов, там "db->sqlite" неявно конвертируется в "sqlite3 *" тем же
// правилом C, что и для любого другого "void *").
const rechDBStruct = `typedef enum { RECH_DB_SQLITE, RECH_DB_PG, RECH_DB_MYSQL } rech_db_kind;

typedef struct rech_db {
    rech_db_kind kind;
    void *sqlite;
    void *pg;
    void *mysql;
} rech_db_t;

`

// dbCommonHelperFuncs - вспомогательные функции, не завязанные на
// конкретную СУБД: rech_db_dup_str (копия текстового значения в
// GC-память - буфер, который вернула библиотека, живёт только до
// PQclear/mysql_free_result/sqlite3_finalize, а результат "запросить"
// должен пережить их) и растущий буфер rech_db_sb (собственный, не
// переиспользует rech_json_sb - тот эмитится только вместе с JSON, здесь не
// гарантирован) для rech_db_substitute - эмитируются безусловно, для ЛЮБОЙ
// СУБД (в отличие от dbMySQLURLHelperFuncs ниже - тот нужен только MySQL).
var dbCommonHelperFuncs = `static void rech_db_panicf(const char *tmpl, const char *arg) {
    char msg[1024];
    snprintf(msg, sizeof msg, tmpl, arg);
    rech_panic(msg);
}

static const char *rech_db_dup_str(const char *s) {
    if (!s) return "";
    size_t n = strlen(s);
    char *out = GC_MALLOC(n + 1);
    memcpy(out, s, n);
    out[n] = '\0';
    return out;
}

typedef struct { char *buf; long long len; long long cap; } rech_db_sb;

static void rech_db_sb_init(rech_db_sb *sb) {
    sb->cap = 64;
    sb->len = 0;
    sb->buf = malloc((size_t)sb->cap);
}

static void rech_db_sb_ensure(rech_db_sb *sb, long long extra) {
    if (sb->len + extra + 1 > sb->cap) {
        while (sb->cap < sb->len + extra + 1) {
            sb->cap *= 2;
        }
        sb->buf = realloc(sb->buf, (size_t)sb->cap);
    }
}

static void rech_db_sb_putc(rech_db_sb *sb, char c) {
    rech_db_sb_ensure(sb, 1);
    sb->buf[sb->len++] = c;
}

static void rech_db_sb_puts(rech_db_sb *sb, const char *s) {
    long long n = (long long)strlen(s);
    rech_db_sb_ensure(sb, n);
    memcpy(sb->buf + sb->len, s, (size_t)n);
    sb->len += n;
}

`

// dbMySQLURLHelperFuncs - разбор "mysql://" (единственная из трёх схем без
// готового URL-парсера в своей библиотеке - PostgreSQL принимает URL в
// PQconnectdb как есть, SQLite и так просто путь к файлу) - эмитируется
// только когда MySQL реально нужен (internal/dbscheme.Set).
var dbMySQLURLHelperFuncs = fmt.Sprintf(`static char *rech_db_url_part(const char *start, long long len) {
    char *out = GC_MALLOC((size_t)len + 1);
    memcpy(out, start, (size_t)len);
    out[len] = '\0';
    return out;
}

// rech_db_parse_mysql_url разбирает "mysql://[пользователь[:пароль]@]хост[:порт]/база"
// на компоненты (порт 0 - использовать 3306 по умолчанию) - ручной разбор,
// без внешней библиотеки, тем же принципом, что и rech_query_params
// (httpserver.go) для query-строки HTTP.
static void rech_db_parse_mysql_url(const char *conn, char **user, char **pass, char **host, int *port, char **dbname) {
    const char *p = conn + 8; /* strlen("mysql://") */
    const char *slash = strchr(p, '/');
    if (!slash) {
        rech_panic("%[1]s");
    }
    const char *authority = p;
    long long authority_len = (long long)(slash - p);
    const char *at = memchr(authority, '@', (size_t)authority_len);
    const char *hostport = authority;
    long long hostport_len = authority_len;
    *user = "";
    *pass = "";
    if (at) {
        long long userinfo_len = (long long)(at - authority);
        const char *colon = memchr(authority, ':', (size_t)userinfo_len);
        if (colon) {
            *user = rech_db_url_part(authority, (long long)(colon - authority));
            *pass = rech_db_url_part(colon + 1, (long long)(at - colon - 1));
        } else {
            *user = rech_db_url_part(authority, userinfo_len);
        }
        hostport = at + 1;
        hostport_len = authority_len - userinfo_len - 1;
    }
    const char *colon2 = memchr(hostport, ':', (size_t)hostport_len);
    if (colon2) {
        *host = rech_db_url_part(hostport, (long long)(colon2 - hostport));
        *port = atoi(rech_db_url_part(colon2 + 1, hostport_len - (long long)(colon2 - hostport) - 1));
    } else {
        *host = rech_db_url_part(hostport, hostport_len);
        *port = 0;
    }
    const char *db_start = slash + 1;
    const char *q = strchr(db_start, '?');
    long long db_len = q ? (long long)(q - db_start) : (long long)strlen(db_start);
    *dbname = rech_db_url_part(db_start, db_len);
}

`, escapeCString(rtmsg.DBConnectFailed))

// dbConnectCloseCode строит "rech_db_connect"/"rech_db_close" - выбор СУБД
// по префиксу строки подключения ("sqlite:"/"postgres://"/"postgresql://"/
// "mysql://") - только для СУБД, реально нужных backends (internal/
// dbscheme.Set): ветка, которую backends не требует, здесь не эмитируется
// вовсе (не просто "мертва по данным", а физически отсутствует в
// сгенерированном C) - иначе тело "rech_db_connect" безусловно ссылалось бы
// на символ пропущенной библиотеки (PQconnectdb/mysql_init и т. п.), и
// программа не слинковалась бы без неё, даже если эта ветка никогда не
// выполняется (docs/ROADMAP.md, "проблема не в отсутствующем флаге
// линковщика, а в безусловной ссылке на символ в самом сгенерированном
// коде"). "&db->sqlite" - явный "(sqlite3 **)" (единственное место с явным
// приведением из всего рантайма БД) - "void **" не конвертируется в
// "T **" неявно, в отличие от одиночного "void *" (см. package doc
// rechDBStruct).
func dbConnectCloseCode(backends dbscheme.Set) string {
	var sb strings.Builder
	sb.WriteString("static rech_db_t *rech_db_connect(const char *conn) {\n")
	sb.WriteString("    rech_db_t *db = GC_MALLOC(sizeof(rech_db_t));\n")
	if backends.NeedsSQLite() {
		fmt.Fprintf(&sb, `    if (strncmp(conn, "%[1]s", %[2]d) == 0) {
        db->kind = RECH_DB_SQLITE;
        if (sqlite3_open(conn + %[2]d, (sqlite3 **)&db->sqlite) != 0) {
            rech_panic("%[3]s");
        }
        return db;
    }
`, dbscheme.SQLitePrefix, len(dbscheme.SQLitePrefix), escapeCString(rtmsg.DBConnectFailed))
	}
	if backends.NeedsPostgres() {
		fmt.Fprintf(&sb, `    if (strncmp(conn, "%[1]s", %[2]d) == 0 || strncmp(conn, "%[3]s", %[4]d) == 0) {
        db->kind = RECH_DB_PG;
        db->pg = PQconnectdb(conn);
        if (PQstatus(db->pg) != 0) {
            rech_panic("%[5]s");
        }
        return db;
    }
`, dbscheme.PostgresPrefix, len(dbscheme.PostgresPrefix), dbscheme.PostgresAltPrefix, len(dbscheme.PostgresAltPrefix), escapeCString(rtmsg.DBConnectFailed))
	}
	if backends.NeedsMySQL() {
		fmt.Fprintf(&sb, `    if (strncmp(conn, "%[1]s", %[2]d) == 0) {
        db->kind = RECH_DB_MYSQL;
        char *user, *pass, *host, *dbname;
        int port;
        rech_db_parse_mysql_url(conn, &user, &pass, &host, &port, &dbname);
        db->mysql = mysql_init(NULL);
        if (!mysql_real_connect(db->mysql, host, user, pass, dbname, port ? (unsigned int)port : 3306, NULL, 0)) {
            rech_panic("%[3]s");
        }
        return db;
    }
`, dbscheme.MySQLPrefix, len(dbscheme.MySQLPrefix), escapeCString(rtmsg.DBConnectFailed))
	}
	fmt.Fprintf(&sb, `    rech_panic("%s");
    return NULL;
}

`, escapeCString(rtmsg.DBUnsupportedScheme))

	sb.WriteString("static void rech_db_close(rech_db_t *db) {\n    switch (db->kind) {\n")
	if backends.NeedsSQLite() {
		sb.WriteString("    case RECH_DB_SQLITE:\n        sqlite3_close(db->sqlite);\n        break;\n")
	}
	if backends.NeedsPostgres() {
		sb.WriteString("    case RECH_DB_PG:\n        PQfinish(db->pg);\n        break;\n")
	}
	if backends.NeedsMySQL() {
		sb.WriteString("    case RECH_DB_MYSQL:\n        mysql_close(db->mysql);\n        break;\n")
	}
	fmt.Fprintf(&sb, "    default:\n        rech_panic(\"%s\");\n        break;\n    }\n}\n\n", escapeCString(rtmsg.DBUnreachableKind))
	return sb.String()
}

// dbSubstituteCode строит rech_db_substitute(sql, параметры) - переписывает
// "?" (единый плейсхолдер Rech) в литералы SQL прямо по тексту запроса, по
// порядку следования - см. package doc про экранирование. paramsListC -
// C-тип "Список<Значение>" (уже эмитирован genericsCode).
func dbSubstituteCode(paramsListC string) string {
	return fmt.Sprintf(`static char *rech_db_substitute(const char *sql, %[1]s *params) {
    rech_db_sb sb;
    rech_db_sb_init(&sb);
    long long pi = 0;
    for (const char *p = sql; *p; p++) {
        if (*p != '?') {
            rech_db_sb_putc(&sb, *p);
            continue;
        }
        if (pi >= params->len) {
            rech_panic("%[2]s");
        }
        rech_any_t v = params->data[pi++];
        if (v.type == &%[3]s) {
            char buf[32];
            snprintf(buf, sizeof buf, "%%lld", v.v.i);
            rech_db_sb_puts(&sb, buf);
        } else if (v.type == &%[4]s) {
            char buf[64];
            snprintf(buf, sizeof buf, "%%.17g", v.v.f64);
            rech_db_sb_puts(&sb, buf);
        } else if (v.type == &%[5]s) {
            char buf[64];
            snprintf(buf, sizeof buf, "%%.9g", (double)v.v.f32);
            rech_db_sb_puts(&sb, buf);
        } else if (v.type == &%[6]s) {
            rech_db_sb_putc(&sb, '\'');
            for (const char *sp = v.v.s; *sp; sp++) {
                if (*sp == '\'') {
                    rech_db_sb_putc(&sb, '\'');
                }
                rech_db_sb_putc(&sb, *sp);
            }
            rech_db_sb_putc(&sb, '\'');
        } else if (v.type == &%[7]s) {
            rech_db_sb_puts(&sb, v.v.b ? "TRUE" : "FALSE");
        } else if (v.type == &%[8]s) {
            rech_db_sb_puts(&sb, "NULL");
        } else {
            rech_panic("%[9]s");
        }
    }
    rech_db_sb_ensure(&sb, 0);
    sb.buf[sb.len] = '\0';
    return sb.buf;
}

`, paramsListC, escapeCString(rtmsg.DBTooFewParams),
		reflectMetaVarFor(ir.TypeInt), reflectMetaVarFor(ir.TypeFloat), reflectMetaVarFor(ir.TypeFloat32),
		reflectMetaVarFor(ir.TypeString), reflectMetaVarFor(ir.TypeBool), reflectMetaVarFor(ir.TypeNullValue),
		escapeCString(rtmsg.DBInvalidParamType))
}

// sqliteQueryExecCode - разбор через sqlite3_prepare_v2/step (не
// sqlite3_exec - тот отдаёт значения только текстом через callback, здесь
// нужны настоящие типы колонок). Числовые константы - публичный,
// стабильный C ABI SQLite (sqlite3.h): SQLITE_ROW=100, SQLITE_DONE=101,
// SQLITE_INTEGER=1, SQLITE_FLOAT=2, SQLITE_NULL=5.
func sqliteQueryExecCode(resultListC, dictC, dictSet, listAdd, resultCtor, dictCtor string) string {
	return fmt.Sprintf(`static %[1]s *rech_db_sqlite_query(rech_db_t *db, const char *sql) {
    %[1]s *out = %[11]s();
    sqlite3_stmt *stmt;
    if (sqlite3_prepare_v2(db->sqlite, sql, -1, &stmt, NULL) != 0) {
        rech_db_panicf("%[5]s", sql);
    }
    int ncols = sqlite3_column_count(stmt);
    int rc;
    while ((rc = sqlite3_step(stmt)) == 100) {
        %[2]s *row = %[12]s();
        for (int i = 0; i < ncols; i++) {
            const char *colname = rech_db_dup_str(sqlite3_column_name(stmt, i));
            rech_any_t v;
            int ctype = sqlite3_column_type(stmt, i);
            if (ctype == 1) {
                v = (rech_any_t){.type = &%[6]s, .v.i = sqlite3_column_int64(stmt, i)};
            } else if (ctype == 2) {
                v = (rech_any_t){.type = &%[7]s, .v.f64 = sqlite3_column_double(stmt, i)};
            } else if (ctype == 5) {
                v = (rech_any_t){.type = &%[8]s};
            } else {
                v = (rech_any_t){.type = &%[9]s, .v.s = rech_db_dup_str((const char *)sqlite3_column_text(stmt, i))};
            }
            %[3]s(row, colname, v);
        }
        %[4]s(out, row);
    }
    if (rc != 101) {
        sqlite3_finalize(stmt);
        rech_db_panicf("%[5]s", sql);
    }
    sqlite3_finalize(stmt);
    return out;
}

static long long rech_db_sqlite_exec(rech_db_t *db, const char *sql) {
    char *errmsg = NULL;
    if (sqlite3_exec(db->sqlite, sql, NULL, NULL, &errmsg) != 0) {
        rech_db_panicf("%[10]s", sql);
    }
    return (long long)sqlite3_changes(db->sqlite);
}

`, resultListC, dictC, dictSet, listAdd, escapeCString(rtmsg.DBQueryFailedTemplate),
		reflectMetaVarFor(ir.TypeInt), reflectMetaVarFor(ir.TypeFloat), reflectMetaVarFor(ir.TypeNullValue), reflectMetaVarFor(ir.TypeString),
		escapeCString(rtmsg.DBExecFailedTemplate), resultCtor, dictCtor)
}

// pgQueryExecCode - PostgreSQL всегда отдаёт значения текстом в простом
// протоколе запроса (PQexec) - PQftype даёт OID типа колонки для
// классификации (well-known системные OID, стабильны с первых версий
// PostgreSQL: int2/int4/int8=21/23/20, float4/float8/numeric=700/701/1700,
// bool=16). PGRES_COMMAND_OK=1/PGRES_TUPLES_OK=2 - тоже часть публичного,
// стабильного ABI (enum ExecStatusType, libpq-fe.h).
func pgQueryExecCode(resultListC, dictC, dictSet, listAdd, resultCtor, dictCtor string) string {
	return fmt.Sprintf(`static %[1]s *rech_db_pg_query(rech_db_t *db, const char *sql) {
    %[1]s *out = %[12]s();
    PGresult *res = PQexec(db->pg, sql);
    int status = PQresultStatus(res);
    if (status != 1 && status != 2) {
        PQclear(res);
        rech_db_panicf("%[5]s", sql);
    }
    int nrows = PQntuples(res);
    int ncols = PQnfields(res);
    for (int r = 0; r < nrows; r++) {
        %[2]s *row = %[13]s();
        for (int c = 0; c < ncols; c++) {
            const char *colname = rech_db_dup_str(PQfname(res, c));
            rech_any_t v;
            if (PQgetisnull(res, r, c)) {
                v = (rech_any_t){.type = &%[8]s};
            } else {
                unsigned int oid = PQftype(res, c);
                const char *text = PQgetvalue(res, r, c);
                if (oid == 20 || oid == 21 || oid == 23) {
                    v = (rech_any_t){.type = &%[6]s, .v.i = strtoll(text, NULL, 10)};
                } else if (oid == 700 || oid == 701 || oid == 1700) {
                    v = (rech_any_t){.type = &%[7]s, .v.f64 = strtod(text, NULL)};
                } else if (oid == 16) {
                    v = (rech_any_t){.type = &%[10]s, .v.b = (text[0] == 't')};
                } else {
                    v = (rech_any_t){.type = &%[9]s, .v.s = rech_db_dup_str(text)};
                }
            }
            %[3]s(row, colname, v);
        }
        %[4]s(out, row);
    }
    PQclear(res);
    return out;
}

static long long rech_db_pg_exec(rech_db_t *db, const char *sql) {
    PGresult *res = PQexec(db->pg, sql);
    int status = PQresultStatus(res);
    if (status != 1 && status != 2) {
        PQclear(res);
        rech_db_panicf("%[11]s", sql);
    }
    long long affected = atoll(PQcmdTuples(res));
    PQclear(res);
    return affected;
}

`, resultListC, dictC, dictSet, listAdd, escapeCString(rtmsg.DBQueryFailedTemplate),
		reflectMetaVarFor(ir.TypeInt), reflectMetaVarFor(ir.TypeFloat), reflectMetaVarFor(ir.TypeNullValue),
		reflectMetaVarFor(ir.TypeString), reflectMetaVarFor(ir.TypeBool), escapeCString(rtmsg.DBExecFailedTemplate),
		resultCtor, dictCtor)
}

// mysqlQueryExecCode - libmysqlclient без подготовленных выражений
// (mysql_real_query/mysql_store_result/mysql_fetch_row) - тот же принцип
// упрощения, что у PostgreSQL/SQLite: значения читаются как текст
// (MYSQL_ROW - массив char*, NULL - SQL NULL), тип колонки - из
// rech_mysql_field_t.type (публичные, стабильные числовые константы
// enum_field_types, field_types.h: TINY=1/SHORT=2/LONG=3/LONGLONG=8/
// INT24=9/YEAR=13 - целые; DECIMAL=0/FLOAT=4/DOUBLE=5/NEWDECIMAL=246 -
// дробные; всё остальное - текст).
func mysqlQueryExecCode(resultListC, dictC, dictSet, listAdd, resultCtor, dictCtor string) string {
	return fmt.Sprintf(`static %[1]s *rech_db_mysql_query(rech_db_t *db, const char *sql) {
    %[1]s *out = %[11]s();
    if (mysql_real_query(db->mysql, sql, (unsigned long)strlen(sql)) != 0) {
        rech_db_panicf("%[5]s", sql);
    }
    MYSQL_RES *res = mysql_store_result(db->mysql);
    if (!res) {
        return out;
    }
    unsigned int ncols = mysql_num_fields(res);
    const char **colnames = GC_MALLOC(sizeof(char *) * ncols);
    unsigned int *coltypes = GC_MALLOC(sizeof(unsigned int) * ncols);
    for (unsigned int i = 0; i < ncols; i++) {
        rech_mysql_field_t *f = mysql_fetch_field(res);
        colnames[i] = rech_db_dup_str(f->name);
        coltypes[i] = f->type;
    }
    rech_mysql_row_t row_data;
    while ((row_data = mysql_fetch_row(res)) != NULL) {
        %[2]s *row = %[12]s();
        for (unsigned int i = 0; i < ncols; i++) {
            rech_any_t v;
            if (row_data[i] == NULL) {
                v = (rech_any_t){.type = &%[8]s};
            } else {
                unsigned int t = coltypes[i];
                if (t == 1 || t == 2 || t == 3 || t == 8 || t == 9 || t == 13) {
                    v = (rech_any_t){.type = &%[6]s, .v.i = strtoll(row_data[i], NULL, 10)};
                } else if (t == 0 || t == 4 || t == 5 || t == 246) {
                    v = (rech_any_t){.type = &%[7]s, .v.f64 = strtod(row_data[i], NULL)};
                } else {
                    v = (rech_any_t){.type = &%[9]s, .v.s = rech_db_dup_str(row_data[i])};
                }
            }
            %[3]s(row, colnames[i], v);
        }
        %[4]s(out, row);
    }
    mysql_free_result(res);
    return out;
}

static long long rech_db_mysql_exec(rech_db_t *db, const char *sql) {
    if (mysql_real_query(db->mysql, sql, (unsigned long)strlen(sql)) != 0) {
        rech_db_panicf("%[10]s", sql);
    }
    return (long long)mysql_affected_rows(db->mysql);
}

`, resultListC, dictC, dictSet, listAdd, escapeCString(rtmsg.DBQueryFailedTemplate),
		reflectMetaVarFor(ir.TypeInt), reflectMetaVarFor(ir.TypeFloat), reflectMetaVarFor(ir.TypeNullValue), reflectMetaVarFor(ir.TypeString),
		escapeCString(rtmsg.DBExecFailedTemplate), resultCtor, dictCtor)
}

// dbDispatchCode - "запросить"/"выполнить": подставляет параметры один раз
// (rech_db_substitute), дальше выбор конкретной СУБД по db->kind - только
// среди backends (см. package doc dbConnectCloseCode - тот же принцип: не
// ссылаться на rech_db_<опущенная СУБД>_query/exec вообще).
func dbDispatchCode(backends dbscheme.Set, resultListC, paramsListC string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "static %[1]s *rech_db_query(rech_db_t *db, const char *sql, %[2]s *params) {\n    char *final_sql = rech_db_substitute(sql, params);\n    switch (db->kind) {\n",
		resultListC, paramsListC)
	if backends.NeedsSQLite() {
		sb.WriteString("    case RECH_DB_SQLITE:\n        return rech_db_sqlite_query(db, final_sql);\n")
	}
	if backends.NeedsPostgres() {
		sb.WriteString("    case RECH_DB_PG:\n        return rech_db_pg_query(db, final_sql);\n")
	}
	if backends.NeedsMySQL() {
		sb.WriteString("    case RECH_DB_MYSQL:\n        return rech_db_mysql_query(db, final_sql);\n")
	}
	fmt.Fprintf(&sb, "    default:\n        rech_panic(\"%s\");\n        return NULL;\n    }\n}\n\n", escapeCString(rtmsg.DBUnreachableKind))

	fmt.Fprintf(&sb, "static long long rech_db_exec(rech_db_t *db, const char *sql, %[1]s *params) {\n    char *final_sql = rech_db_substitute(sql, params);\n    switch (db->kind) {\n",
		paramsListC)
	if backends.NeedsSQLite() {
		sb.WriteString("    case RECH_DB_SQLITE:\n        return rech_db_sqlite_exec(db, final_sql);\n")
	}
	if backends.NeedsPostgres() {
		sb.WriteString("    case RECH_DB_PG:\n        return rech_db_pg_exec(db, final_sql);\n")
	}
	if backends.NeedsMySQL() {
		sb.WriteString("    case RECH_DB_MYSQL:\n        return rech_db_mysql_exec(db, final_sql);\n")
	}
	fmt.Fprintf(&sb, "    default:\n        rech_panic(\"%s\");\n        return 0;\n    }\n}\n\n", escapeCString(rtmsg.DBUnreachableKind))
	return sb.String()
}

// dbRuntimeCode собирает рантайм БД целиком - вызывается из generateSource,
// только когда prog.UsesDB (см. package doc internal/ir, ir.Program.UsesDB).
// backends (internal/dbscheme.Set, prog.DBBackends) решает, КАКИЕ из трёх
// СУБД реально эмитируются - opaque-типы/прототипы/query-exec код
// пропущенной СУБД не попадают в сгенерированный C вообще (см. package doc
// dbConnectCloseCode - иначе линковка требовала бы её клиентскую
// библиотеку, даже когда она никогда не вызывается). paramsListC/
// resultListC/dictC - C-типы "Список<Значение>"/"Список<Словарь<Строка,
// Значение>>"/"Словарь<Строка, Значение>", уже эмитированные genericsCode
// раньше этого блока (см. место вызова).
func dbRuntimeCode(backends dbscheme.Set) string {
	paramsListC := reflectAnyListCType()
	resultListC := mangleType(dbResultType)
	resultCtor := mangleCtor(dbResultType)
	dictC := mangleType(dbRowType)
	dictCtor := mangleCtor(dbRowType)
	dictSet := mangleMethod(dbRowType, "установить")
	listAdd := mangleMethod(dbResultType, "добавить")

	var sb strings.Builder
	sb.WriteString(dbCommonHelperFuncs)
	if backends.NeedsSQLite() {
		sb.WriteString(sqliteOpaqueTypedefs)
		sb.WriteString(sqliteExternDecls)
	}
	if backends.NeedsPostgres() {
		sb.WriteString(pgOpaqueTypedefs)
		sb.WriteString(pgExternDecls)
	}
	if backends.NeedsMySQL() {
		sb.WriteString(mysqlOpaqueTypedefs)
		sb.WriteString(rechMySQLFieldStruct)
		sb.WriteString(mysqlExternDecls)
		sb.WriteString(dbMySQLURLHelperFuncs)
	}
	sb.WriteString(dbConnectCloseCode(backends))
	sb.WriteString(dbSubstituteCode(paramsListC))
	if backends.NeedsSQLite() {
		sb.WriteString(sqliteQueryExecCode(resultListC, dictC, dictSet, listAdd, resultCtor, dictCtor))
	}
	if backends.NeedsPostgres() {
		sb.WriteString(pgQueryExecCode(resultListC, dictC, dictSet, listAdd, resultCtor, dictCtor))
	}
	if backends.NeedsMySQL() {
		sb.WriteString(mysqlQueryExecCode(resultListC, dictC, dictSet, listAdd, resultCtor, dictCtor))
	}
	sb.WriteString(dbDispatchCode(backends, resultListC, paramsListC))
	return sb.String()
}

// dbHandleTypedefCode - только объявление самого типа-дескриптора "БД"
// (сам "rech_db_t", без единого упоминания SQLite/PostgreSQL/MySQL - поля
// "void *", см. package doc rechDBStruct) - вынесено ИЗ dbRuntimeCode и
// эмитируется РАНЬШЕ (до genericsCode/typedefs, см. место вызова в
// generateSource), потому что сущность теперь может нести поле типа "БД"
// (см. examples/проекты/http_сервер/контейнер.rech) - её C-структуре
// (typedefs) нужен уже видимый "rech_db_t", а раньше он появлялся только
// внутри dbRuntimeCode, эмитированной ПОСЛЕ typedefs (unknown type name
// "rech_db_t" при компиляции сгенерированного out.c). Остальная часть
// рантайма (opaque-типы конкретных СУБД, запросы/подстановка параметров) -
// по-прежнему в dbRuntimeCode, на своём прежнем месте: она зависит от
// generic-типов (Список<Значение> и т. п.), которых на этом, более раннем
// месте ещё нет, и УСЛОВНА по backends - в отличие от этого typedef'а,
// нужного безусловно при UsesDB, независимо от того, какая конкретно СУБД
// используется.
func dbHandleTypedefCode() string {
	return rechDBStruct
}
