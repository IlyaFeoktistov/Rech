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
//
// parent/savepoint - только у вложенной транзакции ("тр.вложенная"): она
// работает на том же *sql.Tx, что и родитель, и завершается
// RELEASE/ROLLBACK TO SAVEPOINT вместо COMMIT/ROLLBACK.
type dbTxState struct {
	tx        *sql.Tx
	db        *sql.DB
	resolved  bool
	parent    *dbTxState
	savepoint string
	depth     int
}

// alive - транзакция и все её предки ещё не завершены: откат родителя
// изнутри колбэка вложенной снимает и её SAVEPOINT, завершать её после
// этого нечего.
func (s *dbTxState) alive() bool {
	for t := s; t != nil; t = t.parent {
		if t.resolved {
			return false
		}
	}
	return true
}

// finish - фиксирует (commit) или откатывает транзакцию, если она ещё
// жива; повторный вызов - no-op.
func (s *dbTxState) finish(commit bool) {
	if !s.alive() {
		s.resolved = true
		return
	}
	s.resolved = true
	if s.parent == nil {
		if commit {
			if err := s.tx.Commit(); err != nil {
				panicRuntime("%s", rtmsg.DBExecFailedFormat("COMMIT"))
			}
		} else {
			s.tx.Rollback()
		}
		return
	}
	stmts := []string{"RELEASE SAVEPOINT " + s.savepoint}
	if !commit {
		stmts = []string{"ROLLBACK TO SAVEPOINT " + s.savepoint, "RELEASE SAVEPOINT " + s.savepoint}
	}
	for _, stmt := range stmts {
		if _, err := s.tx.Exec(stmt); err != nil {
			panicRuntime("%s", rtmsg.DBExecFailedFormat(stmt))
		}
	}
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
	return in.dbRunTransaction(&dbTxState{tx: tx, db: db}, callback, dbg)
}

// dbBeginNestedTransaction - "тр.вложенная(обработчик)" - SAVEPOINT на
// соединении родителя, тот же протокол завершения, что и у
// dbBeginTransaction.
func (in *Interp) dbBeginNestedTransaction(parent *dbTxState, callback Value, dbg *dbgThread) (Value, error) {
	if !parent.alive() {
		panicRuntime("%s", rtmsg.DBTxResolved)
	}
	depth := parent.depth + 1
	state := &dbTxState{tx: parent.tx, db: parent.db, parent: parent, depth: depth, savepoint: fmt.Sprintf("rech_sp_%d", depth)}
	if _, err := parent.tx.Exec("SAVEPOINT " + state.savepoint); err != nil {
		panicRuntime("%s", rtmsg.DBExecFailedFormat("SAVEPOINT "+state.savepoint))
	}
	return in.dbRunTransaction(state, callback, dbg)
}

// dbRunTransaction исполняет колбэк над уже открытой транзакцией state и
// завершает её (см. dbBeginTransaction).
func (in *Interp) dbRunTransaction(state *dbTxState, callback Value, dbg *dbgThread) (Value, error) {
	txValue := Value{Type: ir.EntityType(transactionEntityName), Obj: &entityObj{typeName: transactionEntityName, dbTx: state}}

	defer func() {
		if r := recover(); r != nil {
			state.finish(false)
			panic(r)
		}
	}()

	if callback.Callable == nil {
		state.finish(true)
		return Value{}, nil
	}
	_, callErr := callback.Callable.call([]Value{txValue}, dbg)
	if callErr != nil {
		state.finish(false)
		return Value{}, callErr
	}
	state.finish(true)
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
		pairs := newDictPairs()
		for i, col := range cols {
			pairs.set(stringValue(col), dbBoxColumnValue(scanValues[i]))
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

// evalTransactionMethod - "запросить"/"выполнить"/"откатить"/"вложенная" на значении
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
		state.finish(false)
		return Value{}, nil
	case "вложенная":
		return in.dbBeginNestedTransaction(state, args[0], cc.dbg)
	default:
		return Value{}, fmt.Errorf("у сущности %q нет метода %q", transactionEntityName, e.Method)
	}
}
