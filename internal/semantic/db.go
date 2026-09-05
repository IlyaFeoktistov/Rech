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

// DefaultDBPoolSize - число соединений с БД в пуле, если файл проекта
// (.rechproj) не задаёт "бд.размер_пула" вовсе (internal/project.File.
// DBPoolSize возвращает 0 в этом случае - internal/compiler тогда не
// переопределяет ir.Program.DBPoolSize, оставляя именно это значение,
// подставленное здесь безусловно для ЛЮБОЙ программы - включая те, что
// анализируются в обход internal/compiler, например internal/lsp/
// internal/testrunner). См. package doc ir.Program.DBPoolSize.
const DefaultDBPoolSize = 4

// DefaultDBSQLiteBusyTimeoutMs - сколько миллисекунд SQLite сама ждёт
// снятия блокировки файла ДРУГИМ соединением/процессом (sqlite3_busy_timeout/
// "?_busy_timeout=" в DSN), если файл проекта не задаёт "бд.
// sqlite_таймаут_блокировки_мс" - тем же принципом, что DefaultDBPoolSize
// выше (подставляется здесь безусловно, internal/compiler переопределяет
// только при явном значении > 0). Не путать с ir.Program.DBPoolTimeoutMs -
// то ожидание СОБСТВЕННОГО пула этого рантайма (rech_db_checkout), это -
// ожидание, встроенное в саму SQLite, для файловой блокировки между
// НЕЗАВИСИМЫМИ процессами/соединениями, разные понятия.
const DefaultDBSQLiteBusyTimeoutMs = 5000

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
	// "начать_транзакцию" (internal/semantic/dbtx.go) - тоже нуждается в
	// dbResultType/listOfAnyType (её методы "запросить"/"выполнить" несут
	// те же типы) - тем же принципом идемпотентной лишней регистрации.
	beginTransactionFuncName: true,
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
	// dbResultType ("запросить") И listOfAnyType.Entity ("Список<Значение>",
	// параметры "запросить"/"выполнить") - ОБА принудительно, для ЛЮБОЙ из
	// четырёх функций БД (не только "запросить"/"выполнить") - cbackend
	// эмитирует rech_db_substitute/rech_db_query/rech_db_exec (все три
	// ссылаются на C-typedef ОБОИХ generic-типов) безусловно, как только
	// UsesDB, независимо от того, какая конкретно из четырёх функций
	// реально вызвана в программе (internal/backend/cbackend/db.go,
	// dbRuntimeCode) - программа, вызвавшая только "подключиться_к_бд"/
	// "закрыть_бд" (без единого "запросить"/"выполнить" и без буквального
	// "новый Список<Значение>()" где-либо ещё), иначе не компилировалась бы
	// ("unknown type name" на typedef, которого genericsCode не эмитировала
	// вообще).
	if _, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, dbResultType); err != nil {
		return err
	}
	_, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, listOfAnyType.Entity)
	return err
}
