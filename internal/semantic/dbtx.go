// Транзакции БД ("начать_транзакцию", callback-стиль - см. package doc
// internal/backend/cbackend/dbtx.go для полного описания примитива и
// решения дизайна) - "запросить"/"выполнить" (db.go) каждый сам делает
// rech_db_checkout/rech_db_checkin (Шаг 66), поэтому три отдельных вызова
// ("BEGIN"/"INSERT"/"COMMIT") не гарантируют одно и то же физическое
// соединение при "бд.размер_пула" > 1 - транзакция средствами обычных
// "выполнить" небезопасна. "начать_транзакцию(бд, обработчик)" закрепляет
// ОДНО соединение на весь колбэк и сама решает - зафиксировать (колбэк
// вернулся штатно) или откатить (исключение/паника/явный "откатить()").
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

const transactionEntityName = "Транзакция"

// beginTransactionFuncName - "начать_транзакцию" - единственный builtin
// компилятора, принимающий параметром "Вызываемый" (см. package doc выше) -
// checkCallArgs уже умеет проверять лямбду/"&имя" против ожидаемого
// ir.CallableType (тот же путь, что и у обычных пользовательских функций с
// параметром "Вызываемый" - analyzer.go/expr.go этого не различают,
// проверка типа параметра не завязана на то, builtin это или нет).
const beginTransactionFuncName = "начать_транзакцию"

// registerTransactionEntity - лениво регистрирует "Транзакция" в
// entities/methods при первой встрече имени типа (тем же принципом, что
// registerSemaphoreEntity) - методы дублируют сигнатуры свободных функций
// БД "запросить"/"выполнить" (db.go, registerDBFuncs), но это отдельное
// пространство имён (методы сущности, не совпадают с глобальными
// свободными функциями с теми же именами).
func registerTransactionEntity(entities map[string]entitySig, methods map[string]map[string]funcSig) {
	if _, exists := entities[transactionEntityName]; exists {
		return
	}
	entities[transactionEntityName] = entitySig{IsTransaction: true}
	methods[transactionEntityName] = map[string]funcSig{
		"запросить": {
			Params:        []ir.Type{ir.TypeString, listOfAnyType},
			ParamDefaults: []ast.Expr{nil, dbNoParamsDefault},
			ReturnType:    ir.EntityType(dbResultType),
			HasReturn:     true,
		},
		"выполнить": {
			Params:        []ir.Type{ir.TypeString, listOfAnyType},
			ParamDefaults: []ast.Expr{nil, dbNoParamsDefault},
			ReturnType:    ir.TypeInt,
			HasReturn:     true,
		},
		"откатить": {},
		// "вложенная" - SAVEPOINT на том же соединении: штатный возврат
		// колбэка - RELEASE, исключение/паника/"откатить()" - ROLLBACK TO,
		// внешняя транзакция продолжается.
		"вложенная": {
			Params: []ir.Type{ir.CallableType([]ir.Type{ir.EntityType(transactionEntityName)}, nil)},
		},
	}
}

// transactionIRType - собственная сущность "Транзакция" в готовом
// IR-виде, добавляется в ir.Program.Entities только если использована
// (см. internal/semantic/analyzer.go, buildIR) - Fields/Methods пусты, все
// три синтезируются backend'ом напрямую по IsTransaction (см. package doc
// ir.Entity.IsTransaction).
func transactionIRType() *ir.Entity {
	return &ir.Entity{Name: transactionEntityName, IsTransaction: true}
}

// registerTransactionFuncs - "начать_транзакцию(БД, Вызываемый(Транзакция):
// Ничто)" - без результата (ни один из двух backend'ов не возвращает ничего
// осмысленного - смысл вызова целиком в побочном эффекте колбэка).
func registerTransactionFuncs(funcs map[string]funcSig) {
	funcs[beginTransactionFuncName] = funcSig{
		Params: []ir.Type{
			ir.TypeDB,
			ir.CallableType([]ir.Type{ir.EntityType(transactionEntityName)}, nil),
		},
	}
}
