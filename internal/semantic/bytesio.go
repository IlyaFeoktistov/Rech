// Бинарно-безопасный файловый ввод-вывод - "прочитать_байты"/"записать_
// байты" (на уже открытом "Файл")/"прочитать_файл_байты"/"записать_в_
// файл_байты" (весь файл целиком, без ручного открыть/закрыть) - те же
// операции, что и обычные "прочитать"/"записать"/"прочитать_файл"/
// "записать_в_файл" (шаг 8), но результат/аргумент - "Список<Байт>", не
// "Строка": "Строка" в cbackend - null-terminated C-строка (rech_read:
// "buf[n] = '\0';"), любой байт со значением 0 внутри содержимого молча
// ОБРЕЗАЕТ её - не проблема для текстовых файлов, но делает "Строка"
// непригодной для содержимого произвольного бинарного формата (картинки,
// PDF, zip - см. docs/ROADMAP.md, "загрузка файлов через HTTP-сервер").
// "Список<Байт>" уже существует как generic-коллекция (её C-представление
// - `{unsigned char *data; long long len; long long cap;}`, internal/
// backend/cbackend/collections_ccode.go) - решение переиспользовать её, а
// не заводить отдельный новый тип, было предложено пользователем прямо
// (то же самое рассуждение, что уже привело к переиспользованию "Файл"
// вместо нового файлового дескриптора).
//
// Тип "Список<Байт>" регистрируется лениво - только когда программа
// РЕАЛЬНО вызвала одну из этих функций хоть раз (registerByteListTypeOnCall,
// вызывается из analyzeCallExpr) - тем же принципом, что и
// registerQueryParamsDictTypeOnCall/registerRegexTypesOnCall - иначе
// КАЖДАЯ Rech-программа получала бы неиспользуемую машинерию generic-
// коллекции просто из факта существования этих функций в языке.
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// byteListType - "Список<Байт>", каноническое имя generic-инстанциации,
// используемое как тип результата/параметра функций этого файла и
// entity-полей "тело"/"данные" у ЗапросБайты/ОтветБайты/ЧастьФормы
// (httpbytes.go/multipart.go).
const byteListType = "Список<Байт>"

// bytesIOFuncNames - имена функций этого файла - используется
// analyzeCallExpr (expr.go) для решения, нужно ли звать
// registerByteListTypeOnCall при разборе конкретного вызова.
var bytesIOFuncNames = map[string]bool{
	"прочитать_байты":       true,
	"записать_байты":        true,
	"прочитать_файл_байты":  true,
	"записать_в_файл_байты": true,
	"строка_из_байт":        true,
	"байты_из_строки":       true,
}

// registerBytesIOFuncs регистрирует сигнатуры всех функций этого файла -
// вызывается безусловно (как и registerHTTPServerFuncs) при сборе
// сигнатур программы; ReturnType/Params ссылаются на byteListType по
// имени - сам тип резолвится лениво, при первом реальном вызове (см.
// package doc выше), не здесь.
//
// "строка_из_байт"/"байты_из_строки" - мост между "Список<Байт>" и
// "Строка" (docs/ROADMAP.md - без него сервер, которому нужны И
// JSON/текстовые маршруты, И загрузка файлов, был бы вынужден выбирать
// ОДНУ функцию приёма запроса заранее для ВСЕХ маршрутов сразу - либо
// "принять_http" (Строка), либо "принять_http_байты" (Список<Байт>), не
// смешивая; с этой парой - "принять_http_байты" безусловно для всех, а
// "строка_из_байт(запрос.тело)" там, где нужен именно текст). "байты_из_
// строки" не теряет ничего (Строка уже несёт ровно те байты, что были при
// её построении). "строка_из_байт" физически копирует ВСЕ байты в новую
// Строку без потери здесь же, в этой функции - потеря (если есть)
// происходит не в момент конвертации, а ПОЗЖЕ, у любой операции над
// результатом, которая читает Строку до первого нулевого байта (strlen) -
// врождённое ограничение null-terminated представления в cbackend, не
// новое, вносимое этой парой функций.
func registerBytesIOFuncs(funcs map[string]funcSig) {
	funcs["прочитать_байты"] = funcSig{Params: []ir.Type{ir.TypeFile, ir.TypeInt}, ReturnType: ir.EntityType(byteListType), HasReturn: true}
	funcs["записать_байты"] = funcSig{Params: []ir.Type{ir.TypeFile, ir.EntityType(byteListType)}}
	funcs["прочитать_файл_байты"] = funcSig{Params: []ir.Type{ir.TypeString}, ReturnType: ir.EntityType(byteListType), HasReturn: true}
	funcs["записать_в_файл_байты"] = funcSig{Params: []ir.Type{ir.TypeString, ir.EntityType(byteListType)}}
	funcs["строка_из_байт"] = funcSig{Params: []ir.Type{ir.EntityType(byteListType)}, ReturnType: ir.TypeString, HasReturn: true}
	funcs["байты_из_строки"] = funcSig{Params: []ir.Type{ir.TypeString}, ReturnType: ir.EntityType(byteListType), HasReturn: true}
}

// registerByteListTypeOnCall - вызывается из analyzeCallExpr при каждом
// вызове одной из bytesIOFuncNames (а также принять_http_байты/
// отправить_http_ответ_байты/разобрать_multipart - см. их package doc,
// httpbytes.go/multipart.go, все они несут поле/аргумент этого же типа) -
// resolveTypeName сам идемпотентен, безопасно звать на каждый вызов, не
// только на первый (тот же принцип, что и у registerQueryParamsDictTypeOnCall).
func registerByteListTypeOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, byteListType)
	return err
}
