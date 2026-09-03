package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// dbResultType - тип результата "запросить" (см. registerDBFuncs) - как и
// queryParamsDictType (httpserver.go), регистрируется в entities/methods
// ЛЕНИВО, только когда "запросить" реально вызвана хоть раз в программе
// (registerDBTypesOnCall, вызывается из analyzeCallExpr) - иначе КАЖДАЯ
// Rech-программа получала бы неиспользуемый Список<Словарь<Строка,
// Значение>> (и вложенный Словарь<Строка, Значение>) в сгенерированном C
// просто из факта существования БД в языке.
const dbResultType = "Список<Словарь<Строка, Значение>>"

// registerDBFuncs - единый русский API поверх SQLite/PostgreSQL/MySQL
// (docs/SPEC.md, "БД") - один и тот же "БД" (ir.TypeDB) для всех трёх,
// схема строки подключения ("sqlite:"/"postgres://"/"mysql://") решает,
// какой драйвер использовать, определяется в рантайме, не на этапе
// компиляции:
//   - "подключиться_к_бд(Строка): БД" - строка подключения целиком.
//   - "запросить(БД, Строка, Список<Значение>): Список<Словарь<Строка,
//     Значение>>" - SQL с "?" как единым плейсхолдером во всех трёх (см.
//     package doc internal/backend/cbackend/db.go про переписывание в
//     конкретный диалект), параметры - отдельным списком, каждый элемент
//     заранее боксирован в "Значение" ("(Значение) x") тем же принципом,
//     что и аргументы ".вызвать"/"тип.создать" (рефлексия, reflect.go).
//     Третий аргумент необязателен (значение по умолчанию - пустой "новый
//     Список<Значение>()", см. dbNoParamsDefault) для запросов без
//     плейсхолдеров.
//   - "выполнить(БД, Строка, Список<Значение>): Целое" - то же самое для
//     запросов без результата-таблицы (INSERT/UPDATE/DELETE/DDL),
//     результат - число затронутых строк, третий аргумент так же
//     необязателен.
//   - "закрыть_бд(БД)" - закрывает соединение, как и "закрыть_сокет".
//
// NULL в результате запроса боксируется в "Значение" с типом "Ничто" (то
// же зарезервированное имя, что и литерал nullable-типов "?Тип") - не
// отдельный синглтон вроде "Пусто" (тот означает "у метода нет
// результата", другое понятие) - "результат.получить(колонка).
// тип_значения().имя() == \"Ничто\"" проверяет отсутствие значения тем же
// словом языка, что и везде.
// dbNoParamsDefault - значение по умолчанию третьего аргумента
// "запросить"/"выполнить" (см. package doc) - "новый Список<Значение>()",
// один и тот же узел ast.Expr переиспользуется для обеих сигнатур
// (resolveParamDefault кеширует по указателю на срез ParamDefaults, не на
// сам узел - переиспользование безопасно).
var dbNoParamsDefault = &ast.NewExpr{Type: "Список<Значение>"}

func registerDBFuncs(funcs map[string]funcSig) {
	funcs["подключиться_к_бд"] = funcSig{Params: []ir.Type{ir.TypeString}, ReturnType: ir.TypeDB, HasReturn: true}
	funcs["запросить"] = funcSig{
		Params:        []ir.Type{ir.TypeDB, ir.TypeString, listOfAnyType},
		ParamDefaults: []ast.Expr{nil, nil, dbNoParamsDefault},
		ReturnType:    ir.EntityType(dbResultType),
		HasReturn:     true,
	}
	funcs["выполнить"] = funcSig{
		Params:        []ir.Type{ir.TypeDB, ir.TypeString, listOfAnyType},
		ParamDefaults: []ast.Expr{nil, nil, dbNoParamsDefault},
		ReturnType:    ir.TypeInt,
		HasReturn:     true,
	}
	funcs["закрыть_бд"] = funcSig{Params: []ir.Type{ir.TypeDB}, HasReturn: false}
}

// dbFuncNames - имена всех четырёх функций БД - analyzeCallExpr вызывает
// registerDBTypesOnCall для любой из них (не только "запросить", у
// которой реально есть generic-возврат) - маркировка usesDB/usesReflection
// одним и тем же простым правилом проще, чем различать четыре случая по
// отдельности, а лишняя (идемпотентная) регистрация dbResultType для
// "подключиться_к_бд"/"закрыть_бд" ничего не стоит.
var dbFuncNames = map[string]bool{
	"подключиться_к_бд": true,
	"запросить":         true,
	"выполнить":         true,
	"закрыть_бд":        true,
}

// registerDBTypesOnCall - вызывается из analyzeCallExpr при каждом вызове
// одной из четырёх функций БД - см. package doc dbResultType.
// resolveTypeName сам идемпотентен (resolveDictTypeName/
// resolveGenericTypeName не пересоздают запись, если она уже есть),
// поэтому безопасно звать на каждый вызов, не только на первый.
// markUsesReflection взводится безусловно (не только когда результат
// реально прочитан через "как"/".тип_значения()") - представление
// Список<Значение>/Список<Словарь<Строка, Значение>> в cbackend несёт
// "rech_any_t" (Значение), которого без этого флага просто не было бы в
// сгенерированном C, независимо от того, что вызывающий Rech-код сделает
// с результатом.
func registerDBTypesOnCall(sc *ctx) error {
	sc.a.markUsesReflection()
	sc.a.markUsesDB()
	_, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, dbResultType)
	return err
}
