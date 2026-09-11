package semantic

import (
	"slices"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// DatagramFields - фиксированные поля встроенной сущности "Дейтаграмма" -
// результат "получить_от" (recvfrom на udp-узле, полученном "сетевой_узел"
// БЕЗ хоста, см. registerBuiltinFuncs/internal/builtins.All) - одно
// входящее UDP-сообщение вместе с адресом ЕГО ОТПРАВИТЕЛЯ (в отличие от
// "получить" на подключённом сокете, где собеседник уже зафиксирован
// самим соединением) - "адрес"/"порт" здесь и есть тот отправитель,
// нужный, чтобы ответить именно ему через "отправить_в".
var DatagramFields = []ir.Field{
	{Name: "данные", Type: ir.TypeString},
	{Name: "адрес", Type: ir.TypeString},
	{Name: "порт", Type: ir.TypeInt},
}

// registerNetworkFuncs регистрирует сущность "Дейтаграмма" и функции
// "получить_от"/"отправить_в"/"принять" - не входят в internal/
// builtins.All (там только Kind примитивов и не-nullable возврат, а
// "получить_от" возвращает сущность, "принять" - nullable "?Сокет"), тот
// же принцип, что и у "запрос"/registerDateTimeFuncs/
// "получить_переменную_окружения" (registerEnvFuncs). "?Сокет" НЕ
// регистрируется здесь (в отличие от прежней версии) - см. package doc
// registerSocketNullableOnCall.
func registerNetworkFuncs(entities map[string]entitySig, funcs map[string]funcSig) {
	registerHTTPEntity(entities, "Дейтаграмма", DatagramFields)
	funcs["получить_от"] = funcSig{Params: []ir.Type{ir.TypeSocket, ir.TypeInt}, ReturnType: ir.EntityType("Дейтаграмма"), HasReturn: true}
	funcs["отправить_в"] = funcSig{Params: []ir.Type{ir.TypeSocket, ir.TypeString, ir.TypeString, ir.TypeInt}, HasReturn: false}

	// "принять" возвращает Ничто, если на слушающем Сокете установлен
	// таймаут ("установить_таймаут") и он истёк раньше, чем подключился
	// клиент - иначе (нет таймаута, или клиент подключился раньше) ведёт
	// себя как раньше, просто обёрнуто в "?Сокет".
	funcs["принять"] = funcSig{
		Params:     []ir.Type{ir.TypeSocket},
		ReturnType: ir.NullableType(ir.TypeSocket), HasReturn: true,
	}
}

// funcSigTouchesSocket - true, если сигнатура builtin-функции упоминает
// "Сокет" где-либо (параметром или результатом, включая "?Сокет") -
// условие, при котором cbackend эмитирует "socketHelpers"
// (`internal/backend/cbackend/builtins_shared.go`, `rech_accept`/
// `rech_send`/`rech_recv`/... - ЛЮБАЯ из них требует "?Сокет", т.к.
// `rech_accept` возвращает именно эту обёртку независимо от того, какая
// ИМЕННО сетевая функция вызвана - см. package doc
// registerSocketNullableOnCall). Signature-based проверка вместо
// перечисления имён функций по одной - автоматически верна для ЛЮБОГО
// будущего builtin'а, где-либо принимающего/возвращающего "Сокет", без
// необходимости вручную дублировать список имён, уже существующий в
// cbackend (`usesSockets = true` там же). Единственное ИЗВЕСТНОЕ
// исключение - "запрос" (HTTP-клиент): его сигнатура ("Запрос" -> "Ответ")
// вообще не упоминает "Сокет" (сокет заведён и закрыт ВНУТРИ
// rech_http_request, невидимо на уровне типов Rech) - обрабатывается
// отдельной проверкой имени в analyzeCallExpr, не этой функцией.
func funcSigTouchesSocket(sig funcSig) bool {
	if sig.ReturnType == ir.TypeSocket || sig.ReturnType == ir.NullableType(ir.TypeSocket) {
		return true
	}
	return slices.Contains(sig.Params, ir.TypeSocket)
}

// registerSocketNullableOnCall - регистрирует entitySig "?Сокет" ЛЕНИВО, при
// первом РЕАЛЬНОМ вызове функции, действительно затрагивающей Сокет
// (analyzeCallExpr, funcSigTouchesSocket выше, либо явно "запрос"), а не
// безусловно при настройке анализатора (как было раньше, тем же путём, что
// и до сих пор "?Целое"/"?Строка" у registerCookieFuncs/registerEnvFuncs/
// registerHTTPHeaderFunc) - тот же принцип ленивой регистрации по факту
// использования, что уже применяется к generic-коллекциям/DB-типам/regex/
// "разбить" (registerDBTypesOnCall и соседи, db.go/regex.go/
// stringsplit.go). НАЙДЕННЫЙ структурный риск ("Архитектура
// компилятора - структурные риски"): `Program.Nullables` строился сканом
// ВСЕХ когда-либо ЗАРЕГИСТРИРОВАННЫХ nullable-сущностей, не только реально
// используемых - "?Сокет" регистрировался безусловно (для КАЖДОЙ программы,
// даже без единого сетевого вызова), полагаясь на то, что
// `rech_socket_t` (тип, на который ссылается обёртка) ТОЖЕ эмитируется
// безусловно (иначе - "unknown type name", как уже случалось в Шаге 59) -
// вместо того чтобы полагаться на этот побочный факт впредь у КАЖДОГО
// будущего builtin с nullable-возвратом над условно эмитируемым типом,
// исправлено в корне: "?Сокет" появляется в `entities` (и, значит, в
// итоговом `Program.Nullables`, `internal/semantic/analyzer.go`) только
// тогда, когда сеть действительно затронута где-то в программе. НАЙДЕНО ПРИ
// ПРОВЕРКЕ `check_examples.sh` (не гипотетически): первая версия этой
// правки регистрировала "?Сокет" только на вызове ИМЕННО "принять" - три
// примера-клиента (`examples/сеть/http_запрос.rech`/`udp_сервер.rech`/
// `эхо_клиент.rech`), ни разу не вызывающие "принять" (только
// "сетевой_узел"/"отправить"/"получить"/"запрос"), перестали собираться
// настоящим `cc` ("unknown type name rech_T_n_soket") - `rech_accept`
// эмитируется вместе с ОСТАЛЬНЫМ сетевым рантаймом при usesSockets, не
// только когда программа вызывает именно "принять".
func registerSocketNullableOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, "?Сокет")
	return err
}
