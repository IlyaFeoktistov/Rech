// Бинарно-безопасный HTTP-запрос/ответ - "принять_http_байты"/
// "отправить_http_ответ_байты" - те же операции, что и "принять_http"/
// "отправить_http_ответ" (httpserver.go), но с "тело: Список<Байт>" вместо
// "тело: Строка": "Строка" (null-terminated в cbackend) молча обрезает
// содержимое на первом нулевом байте - непригодна для тела произвольного
// бинарного формата (docs/ROADMAP.md, "загрузка файлов через HTTP-
// сервер").
//
// НЕ поле в уже существующих "Запрос"/"Ответ" - те регистрируются
// БЕЗУСЛОВНО, для КАЖДОЙ Rech-программы (registerHTTPEntity в
// collectFuncSigs, ниже в этом же файле), даже не использующей HTTP вовсе -
// поле generic-типа заставило бы ЛЮБУЮ программу тянуть полную машинерию
// Список<Байт> (typedef + 5+ методов) просто из факта существования этого
// поля в схеме, а не по факту использования - обсуждено и отклонено
// пользователем в пользу отдельных сущностей, регистрируемых лениво (тем
// же принципом, что и "Совпадение", regex.go, - см. её package doc про
// generic-поле "группы").
//
// "ЗапросБайты"/"ОтветБайты" - отдельные, параллельные "Запрос"/"Ответ"
// сущности (те же остальные поля: метод/адрес/порт/путь/заголовки -
// Строка, как обычно - бинарной безопасности требует только тело) -
// регистрируются лениво, только когда программа реально вызвала
// "принять_http_байты"/"отправить_http_ответ_байты" хоть раз
// (registerHTTPBytesTypesOnCall, analyzeCallExpr).
package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

const (
	httpBytesRequestType  = "ЗапросБайты"
	httpBytesResponseType = "ОтветБайты"
)

// httpBytesRequestFields/httpBytesResponseFields - те же поля, что
// httpRequestFields/httpResponseFields (types.go), кроме "тело".
var (
	httpBytesRequestFields = []ir.Field{
		{Name: "метод", Type: ir.TypeString},
		{Name: "адрес", Type: ir.TypeString},
		{Name: "порт", Type: ir.TypeInt},
		{Name: "путь", Type: ir.TypeString},
		{Name: "заголовки", Type: ir.TypeString},
		{Name: "тело", Type: ir.EntityType(byteListType)},
	}
	httpBytesResponseFields = []ir.Field{
		{Name: "код", Type: ir.TypeInt},
		{Name: "заголовки", Type: ir.TypeString},
		{Name: "тело", Type: ir.EntityType(byteListType)},
	}
)

// registerHTTPBytesFuncs - только сигнатуры функций, без регистрации самих
// сущностей/Список<Байт> - те лениво регистрируются при первом реальном
// вызове (registerHTTPBytesTypesOnCall), тем же принципом, что и
// registerRegexFunc.
func registerHTTPBytesFuncs(funcs map[string]funcSig) {
	funcs["принять_http_байты"] = funcSig{Params: []ir.Type{ir.TypeSocket}, ReturnType: ir.EntityType(httpBytesRequestType), HasReturn: true}
	funcs["отправить_http_ответ_байты"] = funcSig{Params: []ir.Type{ir.TypeSocket, ir.EntityType(httpBytesResponseType)}}
}

// registerHTTPBytesTypesOnCall - вызывается из analyzeCallExpr при вызове
// "принять_http_байты"/"отправить_http_ответ_байты" - резолвит ОБЕ
// сущности разом (симметрично registerHTTPEntity для обычных Запрос/Ответ,
// которые тоже регистрируются парой, не по отдельности) - каждая сама
// резолвит byteListType первой (см. их case в resolveTypeName, types.go),
// тем же порядком, что "Совпадение" резолвит regexGroupsType раньше себя.
func registerHTTPBytesTypesOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	if _, err := resolveTypeName(entities, methods, aliases, interfaces, enums, httpBytesRequestType); err != nil {
		return err
	}
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, httpBytesResponseType)
	return err
}
