// Транзакции БД ("начать_транзакцию", callback-стиль - см. package doc
// internal/semantic/dbtx.go для полного описания примитива). В отличие от
// cbackend (internal/backend/cbackend/dbtx.go, два вложенных кадра
// setjmp/longjmp) - здесь НАМНОГО проще: "Исключение" (thrownException) -
// обычная Go error, "Паника" (panicRuntime) - настоящий Go panic/recover,
// два и так уже разных канала Go, не два вида ОДНОГО канала (C longjmp).
package interp

import (
	"database/sql"
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

const beginTransactionFuncName = "начать_транзакцию"
const transactionEntityName = "Транзакция"

// dbTxState - состояние "Транзакция" (entityObj.dbTx) - db нужен ТОЛЬКО
// для dbRewritePlaceholders (определяет драйвер PostgreSQL по
// db.Driver()) - *sql.Tx такого метода не несёт, конкретное соединение
// внутри транзакции то же самое, что и у db, драйвер не меняется.
type dbTxState struct {
	tx       *sql.Tx
	db       *sql.DB
	resolved bool
}

// dbBeginTransaction - "начать_транзакцию(БД, Вызываемый(Транзакция):
// Ничто)". Колбэк выполняется синхронно; откат - если он вернул error
// (thrownException - обычная error, docs internal/interp/exception.go) ИЛИ
// запаниковал (panicRuntime - настоящий Go panic, перехватывается defer/
// recover, откатывается, паника поднимается заново) ИЛИ явно позвал
// "откатить()" (state.resolved уже true к моменту штатного возврата -
// коммит тогда пропускается).
func (in *Interp) dbBeginTransaction(db *sql.DB, callback Value, dbg *dbgThread) (Value, error) {
	tx, err := db.Begin()
	if err != nil {
		panicRuntime("%s", rtmsg.DBConnectFailed)
	}
	state := &dbTxState{tx: tx, db: db}
	txValue := Value{Type: ir.EntityType(transactionEntityName), Obj: &entityObj{typeName: transactionEntityName, dbTx: state}}

	defer func() {
		if r := recover(); r != nil {
			if !state.resolved {
				tx.Rollback()
				state.resolved = true
			}
			panic(r)
		}
	}()

	if callback.Callable == nil {
		return Value{}, nil
	}
	_, callErr := callback.Callable.call([]Value{txValue}, dbg)
	if callErr != nil {
		if !state.resolved {
			tx.Rollback()
			state.resolved = true
		}
		return Value{}, callErr
	}
	if !state.resolved {
		if err := tx.Commit(); err != nil {
			panicRuntime("%s", rtmsg.DBExecFailedFormat("COMMIT"))
		}
		state.resolved = true
	}
	return Value{}, nil
}

// dbTxQuery/dbTxExec - "тр.запросить"/"тр.выполнить" - тот же смысл и та
// же логика разбора результата, что и dbQuery/dbExec (db.go), но НА УЖЕ
// ОТКРЫТОЙ транзакции (*sql.Tx), без in.dbCheckoutContext() - соединение
// уже эксклюзивно удерживается этой транзакцией целиком, ждать в пуле
// больше нечего (та же причина, что у cbackend - Транзакция.mini,
// rech_db_checkout мгновенно отдаёт единственный уже занятый слот).
func (in *Interp) dbTxQuery(state *dbTxState, sqlText string, params []Value) Value {
	args := make([]any, len(params))
	for i, p := range params {
		args[i] = dbParamValue(p.Boxed)
	}
	rows, err := state.tx.Query(dbRewritePlaceholders(state.db, sqlText), args...)
	if err != nil {
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

func (in *Interp) dbTxExec(state *dbTxState, sqlText string, params []Value) Value {
	args := make([]any, len(params))
	for i, p := range params {
		args[i] = dbParamValue(p.Boxed)
	}
	result, err := state.tx.Exec(dbRewritePlaceholders(state.db, sqlText), args...)
	if err != nil {
		panicRuntime("%s", rtmsg.DBExecFailedFormat(sqlText))
	}
	affected, err := result.RowsAffected()
	if err != nil {
		panicRuntime("%s", rtmsg.DBExecFailedFormat(sqlText))
	}
	return intValue(affected)
}

// evalTransactionMethod - "запросить"/"выполнить"/"откатить" на значении
// "Транзакция" - recv.Obj.dbTx != nil отличает эту ветку от обычного
// метода/generic-коллекции/Блокировки/Семафора (см. evalMethodCallExpr).
func (in *Interp) evalTransactionMethod(obj *entityObj, e *ir.MethodCallExpr, env *Env, cc *callCtx) (Value, error) {
	args := make([]Value, len(e.Args))
	for i, a := range e.Args {
		v, err := in.evalExpr(a, env, cc)
		if err != nil {
			return Value{}, err
		}
		args[i] = v
	}
	state := obj.dbTx
	switch e.Method {
	case "запросить":
		return in.dbTxQuery(state, args[0].S, reflectListElems(args[1])), nil
	case "выполнить":
		return in.dbTxExec(state, args[0].S, reflectListElems(args[1])), nil
	case "откатить":
		if !state.resolved {
			state.tx.Rollback()
			state.resolved = true
		}
		return Value{}, nil
	default:
		return Value{}, fmt.Errorf("у сущности %q нет метода %q", transactionEntityName, e.Method)
	}
}
