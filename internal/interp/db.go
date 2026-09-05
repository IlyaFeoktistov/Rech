// "подключиться_к_бд"/"запросить"/"выполнить"/"закрыть_бд" - единый API
// поверх SQLite/PostgreSQL/MySQL (см. package doc internal/semantic/db.go)
// - реализовано через стандартный database/sql и три pure-Go драйвера
// (modernc.org/sqlite, github.com/lib/pq, github.com/go-sql-driver/mysql),
// зарегистрированные анонимным импортом ниже - ни один C-клиент не
// подключается (в отличие от internal/backend/cbackend/db.go, у
// интерпретатора и так нет доступа к произвольной динамической линковке).
// Схема строки подключения ("sqlite:"/"postgres://"/"mysql://") решает,
// какой драйвер database/sql использовать - см. dbDriverAndDSN.
package interp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	pqdrv "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"github.com/IlyaFeoktistov/Rech/internal/dbscheme"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// dbRowType/dbResultType - те же имена, что строит internal/semantic/db.go
// (dbResultType) через resolveDictTypeName/resolveGenericTypeName -
// Список<T>/Словарь<К, В> распознаются интерпретатором по каноническому
// имени типа (см. entityObj.typeName), не по отдельному флагу.
const (
	dbRowType    = "Словарь<Строка, Значение>"
	dbResultType = "Список<Словарь<Строка, Значение>>"
)

// dbConnect - "подключиться_к_бд(Строка): БД" - настраивает пул database/sql
// (SetMaxOpenConns) на in.dbPoolSize, см. package doc Interp.dbPoolSize.
// Исключение - SQLite "в памяти" (":memory:"): КАЖДОЕ физическое соединение
// пула открывает СВОЮ, независимую от остальных, пустую in-memory базу -
// пул больше одного соединения означал бы, что запись через одно соединение
// не видна через другое (тихая потеря данных, не ошибка) - тот же принцип,
// что и у cbackend (internal/backend/cbackend/db.go, rech_db_connect) -
// для ":memory:" пул принудительно 1, независимо от dbPoolSize.
//
// "?_busy_timeout=5000" - добавлен к DSN файлового SQLite (modernc.org/
// sqlite, dsnPick/"pragma busy_timeout", sqlite.go) безусловно: НЕСКОЛЬКО
// реальных соединений пула теперь могут одновременно писать в один и тот же
// файл - без этого второй писатель падает с SQLITE_BUSY немедленно вместо
// того, чтобы подождать освобождения блокировки (см. тот же принцип на
// стороне cbackend, sqlite3_busy_timeout).
func (in *Interp) dbConnect(connStr string) Value {
	driverName, dsn, ok := dbDriverAndDSN(connStr)
	if !ok {
		panicRuntime("%s", rtmsg.DBUnsupportedScheme)
	}
	poolSize := in.dbPoolSize
	isSQLiteMemory := driverName == "sqlite" && dsn == ":memory:"
	if driverName == "sqlite" {
		dsn += fmt.Sprintf("?_busy_timeout=%d", in.dbSQLiteBusyTimeoutMs)
	}
	db, err := sql.Open(driverName, dsn)
	if err == nil {
		err = db.Ping()
	}
	if err != nil {
		panicRuntime("%s", rtmsg.DBConnectFailed)
	}
	if isSQLiteMemory {
		poolSize = 1
	}
	db.SetMaxOpenConns(poolSize)
	return Value{Type: ir.TypeDB, DB: db}
}

// dbDriverAndDSN разбирает строку подключения на имя зарегистрированного
// драйвера database/sql и DSN, ожидаемый этим драйвером: "sqlite:" -
// остаток строки как есть (путь к файлу или ":memory:"); "postgres://"/
// "postgresql://" - строка целиком (lib/pq сам разбирает URL); "mysql://" -
// перестраивается в собственный DSN go-sql-driver/mysql
// ("пользователь:пароль@tcp(хост:порт)/база"), потому что этот драйвер (в
// отличие от lib/pq) URL-формат не принимает.
func dbDriverAndDSN(connStr string) (driver, dsn string, ok bool) {
	switch dbscheme.Detect(connStr) {
	case dbscheme.SQLite:
		return "sqlite", strings.TrimPrefix(connStr, dbscheme.SQLitePrefix), true
	case dbscheme.Postgres:
		return "postgres", connStr, true
	case dbscheme.MySQL:
		dsn, ok := mysqlDSNFromURL(connStr)
		return "mysql", dsn, ok
	default:
		return "", "", false
	}
}

// mysqlDSNFromURL переводит "mysql://пользователь:пароль@хост:порт/база" в
// формат DSN go-sql-driver/mysql - "пользователь:пароль@tcp(хост:порт)/база
// [?параметры]" (RawQuery переносится как есть - у обоих формат
// "ключ=значение&...").
func mysqlDSNFromURL(connStr string) (string, bool) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", false
	}
	userinfo := u.User.Username()
	if pass, has := u.User.Password(); has {
		userinfo += ":" + pass
	}
	dsn := fmt.Sprintf("%s@tcp(%s)%s", userinfo, u.Host, u.Path)
	if u.RawQuery != "" {
		dsn += "?" + u.RawQuery
	}
	return dsn, true
}

// dbRewritePlaceholders переписывает "?" (единый плейсхолдер Rech, см.
// package doc internal/semantic/db.go) в то, что реально ожидает
// database/sql-драйвер выбранного соединения - SQLite/MySQL принимают "?"
// как есть, PostgreSQL - только позиционные "$1", "$2", ... (lib/pq не
// умеет "?" вообще). Определяется по фактическому типу db.Driver()
// (*pqdrv.Driver), не по хранению отдельного поля "какой это диалект" -
// сам *sql.DB уже знает свой драйвер.
func dbRewritePlaceholders(db *sql.DB, sqlText string) string {
	if _, isPostgres := db.Driver().(*pqdrv.Driver); !isPostgres {
		return sqlText
	}
	var sb strings.Builder
	n := 0
	for _, r := range sqlText {
		if r == '?' {
			n++
			sb.WriteByte('$')
			sb.WriteString(strconv.Itoa(n))
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// dbCheckoutContext - context.Background() без ограничения (in.dbPoolTimeoutMs
// <= 0 - "бд.таймаут_пула_мс" не задан в .rechproj, ждать бесконечно, тем же
// поведением, что и до появления этой настройки), либо контекст с дедлайном
// в in.dbPoolTimeoutMs миллисекунд. Вызывающий код ОБЯЗАН звать cancel()
// (defer) сразу после получения - иначе context.WithTimeout течёт таймер
// до истечения самого дедлайна, даже если запрос уже давно завершился.
//
// Известная асимметрия с cbackend (internal/backend/cbackend/db.go,
// rech_db_checkout): у database/sql нет отдельного примитива "подождать
// свободное соединение из пула" - QueryContext/ExecContext кладут под ОДИН
// контекст и ожидание слота пула, и время выполнения самого запроса, тогда
// как cbackend таймаутит РОВНО ожидание слота, отдельно от запроса. Здесь
// таймаут строже задуманного - может сработать и на честно медленном
// запросе, не только на занятости пула. Не блокирует фичу (тот же класс
// принятого различия бэкендов, что и SIGPIPE/ленивое TLS-рукопожатие), но
// не должен быть спрятан.
func (in *Interp) dbCheckoutContext() (context.Context, context.CancelFunc) {
	if in.dbPoolTimeoutMs <= 0 {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), time.Duration(in.dbPoolTimeoutMs)*time.Millisecond)
}

// dbQuery - "запросить(БД, Строка, Список<Значение>): Список<Словарь<Строка,
// Значение>>".
func (in *Interp) dbQuery(db *sql.DB, sqlText string, params []Value) Value {
	args := make([]any, len(params))
	for i, p := range params {
		args[i] = dbParamValue(p.Boxed)
	}
	ctx, cancel := in.dbCheckoutContext()
	defer cancel()
	rows, err := db.QueryContext(ctx, dbRewritePlaceholders(db, sqlText), args...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			panicRuntime("%s", rtmsg.DBCheckoutTimeout)
		}
		panicRuntime("%s", rtmsg.DBQueryFailedFormat(sqlText))
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		panicRuntime("%s", rtmsg.DBQueryFailedFormat(sqlText))
	}
	scanTargets := make([]any, len(cols))
	scanValues := make([]any, len(cols))
	for i := range scanValues {
		scanTargets[i] = &scanValues[i]
	}
	rowElems := make([]Value, 0)
	for rows.Next() {
		if err := rows.Scan(scanTargets...); err != nil {
			panicRuntime("%s", rtmsg.DBQueryFailedFormat(sqlText))
		}
		pairs := make(map[Value]Value, len(cols))
		for i, col := range cols {
			pairs[stringValue(col)] = dbBoxColumnValue(scanValues[i])
		}
		rowElems = append(rowElems, Value{
			Type: ir.EntityType(dbRowType),
			Obj:  &entityObj{typeName: dbRowType, coll: &collection{template: "Словарь", pairs: pairs}},
		})
	}
	if err := rows.Err(); err != nil {
		panicRuntime("%s", rtmsg.DBQueryFailedFormat(sqlText))
	}
	return Value{
		Type: ir.EntityType(dbResultType),
		Obj:  &entityObj{typeName: dbResultType, coll: &collection{template: "Список", elems: rowElems}},
	}
}

// dbExec - "выполнить(БД, Строка, Список<Значение>): Целое" - число
// затронутых строк.
func (in *Interp) dbExec(db *sql.DB, sqlText string, params []Value) Value {
	args := make([]any, len(params))
	for i, p := range params {
		args[i] = dbParamValue(p.Boxed)
	}
	ctx, cancel := in.dbCheckoutContext()
	defer cancel()
	result, err := db.ExecContext(ctx, dbRewritePlaceholders(db, sqlText), args...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			panicRuntime("%s", rtmsg.DBCheckoutTimeout)
		}
		panicRuntime("%s", rtmsg.DBExecFailedFormat(sqlText))
	}
	affected, err := result.RowsAffected()
	if err != nil {
		panicRuntime("%s", rtmsg.DBExecFailedFormat(sqlText))
	}
	return intValue(affected)
}

// dbParamValue распаковывает боксированный параметр ("Значение", см.
// internal/semantic/db.go) в Go-значение, которое database/sql передаст
// драйверу как есть - только Целое/Действительное/Дробное/Строка/
// Логическое допустимы, всё остальное - недопустимый тип параметра БД.
func dbParamValue(v *Value) any {
	switch v.Type.Kind {
	case ir.KindInt:
		return v.I
	case ir.KindFloat, ir.KindFloat32:
		return v.F
	case ir.KindString:
		return v.S
	case ir.KindBool:
		return v.B
	default:
		panicRuntime("%s", rtmsg.DBInvalidParamType)
		return nil
	}
}

// dbBoxColumnValue боксирует значение одной колонки одной строки
// результата (уже приведённое database/sql/драйвером к одному из
// стандартных Go-типов) в "Значение" - int64 -> Целое, float64 ->
// Действительное, bool -> Логическое, string/[]byte -> Строка (текстовый
// протокол части драйверов - в частности, go-sql-driver/mysql - отдаёт
// текстовые/decimal-колонки как []byte, не string), nil (SQL NULL) ->
// ir.TypeNullValue (см. её package doc - тот же принцип на стороне
// cbackend, internal/backend/cbackend/db.go). Любой другой Go-тип
// (time.Time и т. п.) - как есть в текстовом виде через fmt.Sprint,
// запасной путь на случай нестандартной колонки, которую драйвер не привёл
// ни к одному из четырёх основных типов.
func dbBoxColumnValue(raw any) Value {
	var inner Value
	switch v := raw.(type) {
	case nil:
		inner = Value{Type: ir.TypeNullValue}
	case int64:
		inner = intValue(v)
	case float64:
		inner = floatValue(v)
	case bool:
		inner = boolValue(v)
	case string:
		inner = stringValue(v)
	case []byte:
		inner = stringValue(string(v))
	default:
		inner = stringValue(fmt.Sprint(v))
	}
	return Value{Type: ir.TypeAny, Boxed: &inner}
}
