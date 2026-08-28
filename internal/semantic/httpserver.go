package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// queryParamsDictType - тип результата "параметры_пути" (см. ниже). В
// отличие от Запрос/Ответ (registerHTTPEntity - безусловно, для КАЖДОЙ
// программы), эта generic-инстанциация регистрируется в entities/methods
// ЛЕНИВО - только когда "параметры_пути" реально вызвана хоть раз где-то
// в программе (registerQueryParamsDictTypeOnCall, вызывается из
// analyzeCallExpr) - иначе КАЖДАЯ Rech-программа получала бы
// неиспользуемый typedef и пять методов Словарь<Строка, Строка> в
// сгенерированном C просто из факта существования этой встроенной функции
// в языке (тот же принцип, что и у "ОшибкаРазбора", см. parse.go).
const queryParamsDictType = "Словарь<Строка, Строка>"

// registerHTTPServerFuncs - минимальный HTTP-СЕРВЕР поверх уже
// существующих сетевых примитивов (слушать_tcp/принять) и уже
// существующих сущностей "Запрос"/"Ответ" (registerHTTPEntity, вызывается
// до этой функции - см. collectFuncSigs) - симметрично HTTP-клиенту
// ("запрос"), не отдельный набор типов:
//   - "принять_http(Сокет): Запрос" - принимает одно соединение на уже
//     слушающем сокете и разбирает сырые байты запроса в "Запрос" (метод/
//     путь/заголовки/тело); "адрес"/"порт" (поля клиентского назначения)
//     остаются пустыми - у входящего запроса нет смысла "куда стучаться",
//     он уже пришёл.
//   - "отправить_http_ответ(Сокет, Ответ)" - сериализует "Ответ" в сырой
//     HTTP-ответ и отправляет его на том же соединении, без результата.
//     Закрытие соединения - на стороне вызывающего кода (закрыть_сокет),
//     тем же принципом, что и у остальных сетевых примитивов - не скрыто
//     внутри builtin'а.
//   - "параметры_пути(Строка): Словарь<Строка, Строка>" - разбирает
//     query-строку ("?a=1&b=2", часть после "?" - до неё, если есть,
//     игнорируется, значит применимо и к "запрос.путь" целиком, и к уже
//     отдельно взятой query-строке) с процент-декодированием ключей/
//     значений (%XX, "+" как пробел). ReturnType ссылается на
//     queryParamsDictType по имени - сам тип регистрируется лениво (см. её
//     package doc), не здесь.
func registerHTTPServerFuncs(funcs map[string]funcSig) {
	funcs["принять_http"] = funcSig{Params: []ir.Type{ir.TypeSocket}, ReturnType: ir.EntityType("Запрос"), HasReturn: true}
	funcs["отправить_http_ответ"] = funcSig{Params: []ir.Type{ir.TypeSocket, ir.EntityType("Ответ")}, HasReturn: false}
	funcs["параметры_пути"] = funcSig{Params: []ir.Type{ir.TypeString}, ReturnType: ir.EntityType(queryParamsDictType), HasReturn: true}
}

// registerQueryParamsDictTypeOnCall - вызывается из analyzeCallExpr при
// каждом вызове "параметры_пути" - см. package doc queryParamsDictType.
// resolveTypeName сам идемпотентен (не пересоздаёт запись, если она уже
// есть - resolveDictTypeName), поэтому безопасно звать на каждый вызов, не
// только на первый.
func registerQueryParamsDictTypeOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, queryParamsDictType)
	return err
}
