// "состояние_бд(БД): СостояниеБД" - живой снимок пула соединений (уже
// трекается обоими backend'ами - rech_db_t.pool_size/in_use[] в cbackend,
// database/sql.DBStats() в interp) - пользователь явно попросил ЧИСТЫЙ
// ГЕТТЕР без сеттера (см. project_functions_to_methods_backlog): typestate
// ("состояние:"/"становится") здесь не подходит - тот механизм для ОДНОГО
// дискретного тега состояния, переключаемого изнутри тела метода
// пользовательской сущности, а не для числового снимка сразу по НЕСКОЛЬКИМ
// физическим соединениям пула, и БД - не ir.Entity вовсе (непрозрачный
// KindDB), у неё нет тел методов, куда можно было бы поместить "становится".
// Значит нет и способа НАПИСАТЬ "бд становится ..." на Rech - геттер
// достаточен и безопасен сам по себе.
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// dbStateEntityName - см. package doc.
const dbStateEntityName = "СостояниеБД"

// DBStateFields - "всего" (размер пула) == "занято" (соединений выдано
// вызовам прямо сейчас) + "простаивает" (свободны) - инвариант, который
// обязаны соблюдать оба backend'а (см. их package doc).
var DBStateFields = []ir.Field{
	{Name: "всего", Type: ir.TypeInt},
	{Name: "занято", Type: ir.TypeInt},
	{Name: "простаивает", Type: ir.TypeInt},
}

// registerDBStateFunc регистрирует "состояние_бд" - отдельно от
// registerDBFuncs (db.go), но вызывается сразу следом (analyzer.go) и
// делит с ней dbFuncNames (см. package doc db.go, markUsesDB). Тем же
// принципом, что "Процесс"/"РезультатКоманды" (registerSubprocessFuncs) -
// entitySig регистрируется здесь же, БЕЗУСЛОВНО (не лениво) - "СостояниеБД"
// плоская структура из трёх Целое, лишняя регистрация не стоит ничего.
func registerDBStateFunc(entities map[string]entitySig, funcs map[string]funcSig) {
	registerHTTPEntity(entities, dbStateEntityName, DBStateFields)
	funcs["состояние_бд"] = funcSig{Params: []ir.Type{ir.TypeDB}, ReturnType: ir.EntityType(dbStateEntityName), HasReturn: true}
}
